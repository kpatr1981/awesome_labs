#!/usr/bin/env python3
"""The on-call runbooks the Retrieval labs search (AI engineering track).

  corpus.py write DIR STATE [--long] [--stale] [--dups] [--only ID,ID]
      write the runbooks to DIR/<id>.md and this run's facts to STATE (JSON)
  corpus.py change DIR STATE
      edit db-failover (a new standby), delete cost-spike, add search-red; update STATE
  corpus.py questions STATE [KIND,...]
      hidden test questions as JSON lines: {"q", "want": [ids], "kind", "fact"?}
      kinds: semantic, para, code, fact, long, stale, hard, none, acl, changed

Every run picks new facts (error codes, host names, queue names), so the right answer
exists only in this run's files. Stdlib only: runs on the VM's python3 and in kl-rag.
"""
import json, os, random, sys

# ---------- the runbooks ----------
# Each: id, title, team, sections (heading -> text). {code} is the doc's error code;
# other {names} are facts drawn per run in FACTS below.
DOCS = [
    dict(id="db-failover", title="Fail over the primary PostgreSQL database", team="platform", sections={
        "When to use this": "The primary database for the orders service is down or unreachable and will not come back within five minutes. Writes are failing across the shop.",
        "Symptoms": "Alert PostgresPrimaryDown fires. The orders API returns 500s and its logs say `{code}: could not connect to primary`. Replication lag on the standby stops increasing because nothing is replicating.",
        "Steps": "1. Confirm the primary is really gone: `pg_isready -h pg-primary` fails from two different hosts.\n2. Fence the old primary so it can never accept writes again: stop its VM in the console.\n3. Promote the standby **{standby}**: `ssh {standby} sudo -u postgres pg_ctl promote -D /var/lib/postgresql/16/main`.\n4. Point the `pg-primary` DNS record at {standby} (TTL is 30 seconds).\n5. Restart the orders API pods so they reconnect.",
        "Verify": "`psql -h pg-primary -c 'select pg_is_in_recovery()'` returns `f`. Orders are being written again.",
        "Escalation": "If promotion fails, page the database on-call. Do not try to repair the old primary during the incident.",
    }),
    dict(id="db-restore", title="Restore a database from backup", team="platform", sections={
        "When to use this": "Data was deleted or corrupted and you need to bring back a table or a whole database as it was at an earlier point in time.",
        "Symptoms": "Someone ran a bad migration or a `DELETE` without a `WHERE`. The application logs `{code}: relation is empty` or users report missing records.",
        "Steps": "1. Find the last good moment from the audit log.\n2. Restore into a scratch server, never over production: `pgbackrest --stanza=main --type=time --target='<timestamp>' restore` on the restore host **{restorehost}**.\n3. Dump only the damaged table from the scratch server with `pg_dump -t <table>`.\n4. Load it into production inside a transaction and compare row counts before committing.",
        "Verify": "Row counts match the audit log; the application stops logging errors.",
        "Escalation": "Point-in-time recovery older than 14 days is not possible; tell the incident lead straight away.",
    }),
    dict(id="backup-policy", title="Backup schedule and retention policy", team="platform", sections={
        "Scope": "Which databases are backed up, how often, and how long the backups are kept. This page is policy, not a procedure.",
        "Schedule": "Full backups of every PostgreSQL cluster run nightly at 01:00 UTC with pgBackRest. WAL is archived continuously to object storage, so any moment in the last 14 days can be recovered.",
        "Retention": "Nightly full backups are kept for 14 days, monthly backups for one year. Backup failures raise the alert `{code}` in the platform channel.",
        "Ownership": "The platform team owns the backup jobs. Changes to the retention need approval from the data protection officer.",
    }),
    dict(id="cert-rotation", title="Rotate the TLS certificate on the edge load balancer", team="security", sections={
        "When to use this": "The public certificate on the edge load balancers expires within 14 days, or it was revoked.",
        "Symptoms": "Browsers show a certificate warning, monitoring reports `{code}: x509: certificate has expired or is not yet valid`, or the expiry alert fires.",
        "Steps": "1. Request a new certificate from the internal ACME issuer.\n2. Store the certificate and key in Vault at **{vaultpath}**.\n3. Run the `edge-certs` pipeline; it pushes the new pair to every load balancer and reloads HAProxy without dropping connections.\n4. Keep the old certificate until the new one is confirmed everywhere.",
        "Verify": "`openssl s_client -connect shop.example.com:443 | openssl x509 -noout -enddate` shows the new expiry date on every load balancer.",
        "Escalation": "If the pipeline fails half-way, page security on-call: some load balancers may serve the old certificate.",
    }),
    dict(id="cert-inventory", title="Certificate inventory and expiry dates", team="security", sections={
        "Scope": "A list of the certificates the company owns, where each one is used and when it expires. Update it whenever a certificate is issued.",
        "Inventory": "Edge load balancer: public wildcard, renewed every 90 days. Internal mTLS: issued by the service mesh, renewed daily. VPN gateway: renewed yearly. Code signing: held by the release team.",
        "Expiry monitoring": "A daily job compares the inventory with what servers actually present and raises `{code}` when a certificate expires within 21 days.",
    }),
    dict(id="disk-full", title="A server's disk is full", team="platform", sections={
        "When to use this": "A host's root or data filesystem is above 95% and services on it are failing to write.",
        "Symptoms": "Alert NodeFilesystemAlmostFull fires. Applications log `{code}: no space left on device`.",
        "Steps": "1. Find what grew: `du -xh / --max-depth=2 | sort -h | tail`.\n2. Usually it is logs: `journalctl --vacuum-size=500M` and remove rotated logs older than seven days in /var/log.\n3. Docker hosts: `docker system prune` removes stopped containers and dangling images.\n4. If a data volume is full, grow the volume in the cloud console and run `resize2fs`.",
        "Verify": "`df -h` shows usage under 80% and the alert resolves.",
        "Escalation": "Never delete database files to free space. Page the platform on-call instead.",
    }),
    dict(id="dns-outage", title="Internal DNS stops resolving", team="network", sections={
        "When to use this": "Services cannot resolve each other's internal names, so calls between them fail even though every service is up.",
        "Symptoms": "Logs are full of `{code}: lookup orders.internal: no such host` or timeouts on port 53. External names may still work.",
        "Steps": "1. Test from an affected host: `dig @10.0.0.2 orders.internal`.\n2. Check the resolver pods: `kubectl -n kube-system get pods -l k8s-app=kube-dns`.\n3. Restart the resolvers one at a time; if the upstream forwarder **{forwarder}** is down, switch the forwarder in the CoreDNS config map.\n4. Flush the node-local caches.",
        "Verify": "`dig orders.internal` answers in under 10 ms from three different hosts.",
        "Escalation": "Page the network on-call if the forwarders themselves are unreachable.",
    }),
    dict(id="dns-change", title="Request a DNS record change", team="network", sections={
        "Scope": "How to add, change or remove a DNS record in one of our zones. This is a normal change, not an incident procedure.",
        "Process": "Open a change request in the network queue with the zone, record name, type, value and TTL. Records in the public zone need a second approver. Changes go out with the next zone deploy, every hour on the hour.",
        "Limits": "TTLs below 60 seconds are refused. Wildcard records need approval from the network lead. A rejected change shows the reason `{code}` in the ticket.",
    }),
    dict(id="kafka-lag", title="Kafka consumer lag keeps growing", team="data", sections={
        "When to use this": "A consumer group falls further and further behind its topic, so downstream data is stale.",
        "Symptoms": "Alert KafkaConsumerLagHigh fires for the group **{group}**. The consumer logs `{code}: rebalance in progress` over and over.",
        "Steps": "1. See where it is stuck: `kafka-consumer-groups --describe --group {group}`.\n2. A consumer that keeps rebalancing is usually too slow: raise `max.poll.interval.ms` or lower `max.poll.records`.\n3. Scale the consumer deployment up to the number of partitions, never beyond it.\n4. If one message makes it crash every time, move that offset past it and send the message to the dead-letter queue.",
        "Verify": "Lag goes down steadily for 15 minutes.",
        "Escalation": "Data team on-call, during working hours only.",
    }),
    dict(id="deploy-rollback", title="Roll back a bad deploy", team="platform", sections={
        "When to use this": "A release made things worse (errors, latency, broken pages) and a fix-forward will take longer than ten minutes.",
        "Symptoms": "Error rate climbs right after a deploy. The deploy pipeline marks the release with `{code}` when its smoke tests fail.",
        "Steps": "1. Find the previous good release in the pipeline history.\n2. Run `deployctl rollback <service> --to <release>`; it redeploys the previous image with the previous config.\n3. Database migrations are not rolled back automatically: check whether the release ran one.\n4. Freeze deploys for the service until the cause is understood.",
        "Verify": "The error rate returns to its level before the deploy.",
        "Escalation": "If a migration is involved, page the database on-call before rolling back.",
    }),
    dict(id="redis-evictions", title="Redis is evicting keys", team="platform", sections={
        "When to use this": "The session cache is out of memory and evicting keys, so users are logged out or carts empty.",
        "Symptoms": "`evicted_keys` rises in `INFO stats` and the app logs `{code}: OOM command not allowed when used memory > maxmemory`.",
        "Steps": "1. Check memory: `redis-cli INFO memory`.\n2. Find big keys: `redis-cli --bigkeys`.\n3. Make sure session keys have a TTL; a deploy that forgot `EXPIRE` is the usual cause.\n4. Raise `maxmemory` only if the node has room, and set the policy to `allkeys-lru` for the cache.",
        "Verify": "`evicted_keys` stops rising and users stay logged in.",
        "Escalation": "Platform on-call.",
    }),
    dict(id="vpn-access", title="Give a new engineer VPN access", team="it", sections={
        "When to use this": "A new starter or contractor needs to reach internal systems from outside the office.",
        "Steps": "1. Check the access request is approved by their manager in the IT queue.\n2. Add the person to the directory group **{vpngroup}**.\n3. They install the VPN client and sign in with SSO; the profile downloads itself.\n4. Contractors get an account that expires after 90 days.",
        "Troubleshooting": "A client that shows `{code}: no matching profile` means the group membership has not synced yet; it syncs every 15 minutes.",
        "Verify": "They can open the internal wiki while off the office network.",
    }),
    dict(id="oncall-handover", title="Hand over the on-call shift", team="sre", sections={
        "When to use this": "At the end of every on-call week, Monday at 10:00.",
        "Steps": "1. Write the handover note: open incidents, silenced alerts and why, anything flapping.\n2. Walk the next person through it on a call, don't just post it.\n3. Swap the rotation in the paging tool and send a test page to the new person.\n4. Hand over the incident commander role explicitly if an incident is open.",
        "Paging": "Pages that nobody acknowledges within 5 minutes escalate to the secondary, then to the engineering manager. The paging tool marks an unacknowledged page with `{code}`.",
    }),
    dict(id="node-notready", title="A Kubernetes node is NotReady", team="platform", sections={
        "When to use this": "A node in the production cluster reports NotReady and its pods are being evicted.",
        "Symptoms": "`kubectl get nodes` shows NotReady. The kubelet logs `{code}: PLEG is not healthy` or the node stopped sending heartbeats.",
        "Steps": "1. `kubectl describe node <node>` and read the conditions: memory pressure, disk pressure, network unavailable.\n2. SSH to the node and check `systemctl status kubelet containerd`.\n3. Restart containerd, then the kubelet.\n4. If the node does not recover in ten minutes, cordon and drain it and let the autoscaler replace it.",
        "Verify": "The node is Ready, or its replacement is, and no pods are Pending.",
        "Escalation": "Platform on-call; if more than one node is NotReady at once, open an incident.",
    }),
    dict(id="leaked-secret", title="A credential was leaked", team="security", sections={
        "When to use this": "A password, API key or token was pushed to a public repository, pasted in a chat, or shows up in logs.",
        "Steps": "1. Revoke it first, investigate later: disable the key at the provider.\n2. Issue a new credential and store it only in Vault.\n3. Search the audit logs for any use of the old credential since it leaked.\n4. Remove it from the repository history, but assume it is already copied.\n5. Report it in the security channel with the code `{code}` so it is tracked.",
        "Verify": "The old credential is refused by the provider; services work with the new one.",
        "Escalation": "Always page security on-call, even outside working hours.",
    }),
    dict(id="cdn-purge", title="Purge the CDN cache", team="web", sections={
        "When to use this": "The website shows an old page, a broken asset or something that must be taken down after it was changed at the origin.",
        "Steps": "1. Purge by URL when you can; a full purge sends all traffic to the origin at once.\n2. Use the CDN API with the token from Vault at **{cdnpath}**: `cdnctl purge --url <url>`.\n3. For a full purge, warn the platform on-call first and scale the origin up.",
        "Symptoms": "A purge that is refused returns `{code}: purge quota exceeded`; the quota is 1000 URLs per hour.",
        "Verify": "`curl -I <url>` shows `x-cache: MISS` and then the new content.",
    }),
    dict(id="dlq-replay", title="Replay messages from the dead-letter queue", team="data", sections={
        "When to use this": "Messages that failed processing were parked in the dead-letter queue and the bug that broke them is fixed.",
        "Steps": "1. Check the fix is deployed to every consumer.\n2. Inspect a sample: `queuectl peek {dlq} --count 5`.\n3. Replay in batches: `queuectl replay {dlq} --batch 100 --rate 50/s`.\n4. Watch the consumer error rate; stop the replay if errors climb.",
        "Symptoms": "Messages that fail again come back to the DLQ with the reason `{code}`.",
        "Verify": "The dead-letter queue is empty and the replayed orders show up downstream.",
    }),
    dict(id="sso-outage", title="Nobody can log in through SSO", team="it", sections={
        "When to use this": "Staff cannot sign in to internal tools because single sign-on is failing.",
        "Symptoms": "The login page loops, or the identity provider shows `{code}: SAML response signature invalid`.",
        "Steps": "1. Check the identity provider's status page.\n2. A signature error after a certificate change means the tools still trust the old signing certificate: upload the new one in each app's SSO settings.\n3. Use the break-glass admin account from the safe only to fix SSO, and log every use.",
        "Verify": "A normal user can log in to the wiki and the ticketing system.",
        "Escalation": "IT on-call; security must be told about every break-glass login.",
    }),
    dict(id="payment-webhooks", title="Payment provider webhooks are failing", team="payments", sections={
        "When to use this": "The payment provider cannot deliver its webhooks to us, so paid orders stay in pending.",
        "Symptoms": "The provider dashboard shows failed deliveries and our webhook endpoint logs `{code}: signature verification failed` or returns 5xx.",
        "Steps": "1. Check the webhook signing secret in Vault matches the one in the provider dashboard; it changes when someone rotates it there.\n2. Check the endpoint is reachable from the provider's IP ranges.\n3. Once fixed, ask the provider dashboard to resend failed events from the last 24 hours.",
        "Verify": "Pending orders move to paid within a few minutes.",
        "Escalation": "Payments on-call; orders pending for over an hour need a message to customer support.",
    }),
    dict(id="cost-spike", title="The cloud bill jumped overnight", team="finops", sections={
        "When to use this": "Daily cloud spend is more than 30% above the usual for the day of the week.",
        "Steps": "1. Open the cost explorer grouped by service and by tag `team`.\n2. The usual causes: an autoscaling group that never scaled down, a forgotten load test, data transfer between regions, logs at debug level.\n3. Tag owners are asked in the finops channel; untagged resources older than 7 days are stopped.",
        "Alerts": "The budget alert carries the code `{code}` and the name of the account.",
        "Verify": "The next day's spend is back to normal.",
    }),
]

# Long versions (--long): background and reference sections around the procedure, so the
# whole page is far beyond the embedding model's 512 tokens.
LONG = ["db-failover", "cert-rotation", "kafka-lag", "dlq-replay", "sso-outage", "redis-evictions"]
BACKGROUND = (
    "This page belongs to the {team} team and is reviewed every quarter. It was first written "
    "after an incident in which the people on call spent a long time looking for the right "
    "commands, so it tries to collect everything in one place: how the system is put together, "
    "who depends on it, what normally goes wrong and how we found out in the past. Read the "
    "whole page once when you join the rotation, not during an incident. "
    "The system is used by several other teams. Most of them do not know the details of how it "
    "works and they will ask in the incident channel what is going on; point them at the status "
    "page instead of answering one by one. Customers are told through the status page as well, "
    "never through social media. "
)
HISTORY = (
    "The setup has changed several times. The first version ran on a single server in "
    "the old data centre, the second one moved to the cloud with a manual failover, and the "
    "current one is managed with infrastructure as code. Older pages in the wiki may still "
    "describe the earlier versions, so check the date at the top of any page you find. "
    "Monitoring: dashboards for the system are in the observability tool under the team's "
    "folder. Each dashboard has a short description of what normal looks like. Alerts go to "
    "the team's paging rotation during the day and to the platform rotation at night. "
)
REFERENCE = (
    "Reference. Useful general commands when working on any host: `uptime`, `free -m`, "
    "`df -h`, `ss -tlpn`, `journalctl -u <unit> --since -1h`, `systemctl status <unit>`. "
    "On Kubernetes: `kubectl get pods -o wide`, `kubectl describe pod`, `kubectl logs --previous`, "
    "`kubectl top pods`. Change management: any change made during an incident must be written "
    "into the incident timeline with the time, the person and the exact command. After the "
    "incident a ticket is opened to make the change permanent through the normal pipeline, or "
    "to revert it. Post-incident reviews are blameless and happen within five working days; "
    "the incident lead books the meeting and invites everyone who was involved, including "
    "people from other teams. Contacts: the team's channel, the team's manager, and the "
    "engineering manager on duty, listed in the paging tool."
)

# Old versions (--stale). Each is superseded by the current page with the same topic; two are
# marked archived, one was never marked (only the new page's `supersedes` says so).
STALE = [
    dict(id="db-failover-2023", supersededby="db-failover", archived=True,
         title="Database failover procedure (primary down)", team="platform", sections={
        "Failover": "When the primary PostgreSQL database is down, fail over to the standby. Stop the application, then on the standby **{oldstandby}** run `repmgr standby promote`. Edit the application config by hand so the database host points at {oldstandby}, and restart the application. The failover of the primary database is then complete.",
        "Notes": "Primary database failover takes about 30 minutes. Ask in the DBA channel before failing over the primary database.",
    }),
    dict(id="cert-rotation-2022", supersededby="cert-rotation", archived=True,
         title="How to rotate the load balancer TLS certificate", team="security", sections={
        "Rotation": "To rotate the TLS certificate on the load balancer, generate a key and CSR with openssl, send the CSR to the certificate vendor by e-mail, then copy the new certificate to each load balancer with scp into /etc/haproxy/certs and restart HAProxy on each one. Certificate rotation on the load balancer is done once a year.",
    }),
    dict(id="vpn-howto", supersededby="vpn-access", archived=False,
         title="VPN access for new engineers", team="it", sections={
        "Access": "New engineers who need VPN access: e-mail the IT helpdesk with the engineer's name and laptop serial number. IT creates a VPN certificate by hand and sends the engineer a .ovpn file. VPN access for a new engineer takes about two days. Add them to the group **{oldvpngroup}**.",
    }),
]

# Near-duplicate copies (--dups): the same page exported from other wiki spaces.
DUPS = {
    "disk-full": ["disk-full-copy", "sre-space-disk-full", "disk-full-export-2"],
    "node-notready": ["node-notready-copy", "k8s-space-node-notready"],
}

# ---------- hidden questions ----------
SEMANTIC = {
    "db-failover": ["The main Postgres server died and orders can't be written. How do I switch to the replica?",
                    "How do I promote the standby when the primary database is gone?"],
    "db-restore": ["Somebody deleted rows from a table by accident. How do I get them back from last night?",
                   "How do I recover a single table as it was yesterday?"],
    "backup-policy": ["How long do we keep database backups?", "What time do the nightly backups run?"],
    "cert-rotation": ["The HTTPS certificate on the edge is about to expire. How do I replace it?",
                      "Browsers show a certificate warning for the shop. What do I do?"],
    "cert-inventory": ["Where is the list of all our certificates and when each one expires?"],
    "disk-full": ["A server has run out of disk space and apps can't write. How do I free space?"],
    "dns-outage": ["Services can't find each other's hostnames anymore. How do I fix name resolution?"],
    "dns-change": ["How do I ask for a new DNS record to be added?"],
    "kafka-lag": ["A Kafka consumer is falling further and further behind. What should I check?"],
    "deploy-rollback": ["Errors went up right after today's release. How do I go back to the previous version?"],
    "redis-evictions": ["Users keep getting logged out because the session cache is full. What now?"],
    "vpn-access": ["A contractor starts Monday and needs remote access to internal systems. How?"],
    "oncall-handover": ["What do I need to do when my on-call week ends?"],
    "node-notready": ["One of the cluster nodes shows NotReady and pods are being evicted. Steps?"],
    "leaked-secret": ["Someone pushed an API key to a public GitHub repo. What do we do first?"],
    "cdn-purge": ["The website still shows the old page after we changed it. How do I clear the CDN?"],
    "dlq-replay": ["The bug is fixed; how do I reprocess the failed messages that were parked?"],
    "sso-outage": ["Nobody can sign in to internal tools, the login page keeps looping. What do I check?"],
    "payment-webhooks": ["Paid orders are stuck in pending because the payment provider can't reach us. Fix?"],
    "cost-spike": ["Why did our cloud spend jump overnight and how do I find the cause?"],
}
# Paraphrases that share few words with their runbook: meaning matches, keywords don't.
PARA = {
    "db-failover": "Our main SQL box is dead; how does the replica take over writes?",
    "cert-rotation": "Shoppers get a padlock error in their browser on our site.",
    "sso-outage": "Staff get bounced back to the sign-in screen over and over.",
    "cost-spike": "We were billed far more than usual yesterday; who caused it?",
    "redis-evictions": "Logged-in sessions vanish because the in-memory store is maxed out.",
    "dns-outage": "Machines can't look each other up by name anymore.",
    "deploy-rollback": "Undo this morning's release, it broke things.",
    "leaked-secret": "A password ended up on the internet in a commit.",
    "disk-full": "The storage on one box is at 100 percent.",
    "vpn-access": "A new hire needs to work from home and reach the intranet.",
    "kafka-lag": "The event stream readers can't keep up with incoming events.",
    "oncall-handover": "My pager week is over; what do I pass to the next person?",
}
# Questions that land on the deep sections of the long pages.
LONGQ = {
    "db-failover": "Which command promotes the standby Postgres server to primary?",
    "cert-rotation": "Where in Vault do we keep the new edge TLS certificate and key?",
    "kafka-lag": "The consumer keeps rebalancing; which poll settings should I change?",
    "dlq-replay": "How fast should I replay the parked messages, and in what batch size?",
    "sso-outage": "SAML response signature invalid after a certificate change: what do I upload?",
    "redis-evictions": "OOM command not allowed when used memory > maxmemory: what should I fix?",
}
# Facts asked about (citations lab): (doc, fact name, question).
FACTQ = [
    ("db-failover", "standby", "Which server do I promote when the primary database is down?"),
    ("cert-rotation", "vaultpath", "Where is the edge load balancer certificate stored in Vault?"),
    ("kafka-lag", "group", "Which consumer group triggers the Kafka lag alert?"),
    ("vpn-access", "vpngroup", "Which directory group gives a new engineer VPN access?"),
    ("dlq-replay", "dlq", "What is the name of the dead-letter queue to replay?"),
    ("cdn-purge", "cdnpath", "Where is the CDN API token kept in Vault?"),
    ("db-restore", "restorehost", "On which host do I run the pgbackrest restore?"),
    ("dns-outage", "forwarder", "What is the upstream DNS forwarder?"),
]
# Look-alike pairs the first stage tends to confuse (re-ranker lab), tuned against bge-small.
HARDQ = [
    ("db-restore", "What are the steps to bring back data that was lost, using the backups?"),
    ("dns-outage", "Internal hostnames have stopped resolving, which record or forwarder do I fix?"),
    ("cert-rotation", "Which steps put a renewed certificate on the edge load balancers?"),
    ("deploy-rollback", "The release broke checkout; which command takes the service back to the last version?"),
    ("payment-webhooks", "The provider says it cannot deliver events to our endpoint. What's the fix?"),
]
NONE = ["What is the wifi password in the Athens office?",
        "Who won the company football tournament last year?",
        "How many days of parental leave do employees get?"]


def rnd_facts(r):
    words = ["amber", "birch", "cobalt", "delta", "ember", "fjord", "garnet", "harbor", "indigo", "juniper",
             "kestrel", "lumen", "maple", "nimbus", "onyx", "pebble", "quartz", "raven", "sable", "tundra"]
    w = r.sample(words, 12)
    prefixes = r.sample([chr(c) + chr(d) for c in range(65, 91) for d in range(65, 91)], len(DOCS) + 8)
    codes = {}
    for i, d in enumerate(DOCS + STALE + [EXTRA]):
        codes[d["id"]] = f"{prefixes[i]}-{r.randint(1000, 9999)}"
    return {
        "codes": codes,
        "standby": f"pg-{w[0]}-{r.randint(10, 99)}",
        "oldstandby": f"db-{w[1]}-{r.randint(10, 99)}",
        "restorehost": f"restore-{w[2]}-{r.randint(1, 9)}",
        "vaultpath": f"secret/edge/{w[3]}-{r.randint(100, 999)}",
        "cdnpath": f"secret/cdn/{w[4]}-{r.randint(100, 999)}",
        "group": f"orders-{w[5]}-consumer",
        "vpngroup": f"vpn-{w[6]}-users",
        "oldvpngroup": f"vpn-{w[7]}-legacy",
        "dlq": f"orders.{w[8]}.dlq",
        "forwarder": f"10.{r.randint(10, 250)}.{r.randint(0, 250)}.{r.randint(2, 250)}",
    }


PRIVATE = {"leaked-secret", "cert-inventory", "payment-webhooks", "cost-spike"}
EXTRA = dict(id="search-red", title="The search cluster is red", team="platform", sections={
    "When to use this": "The product search cluster reports status red: some shards have no copy at all, so searches return partial results.",
    "Symptoms": "Alert SearchClusterRed fires and the search API logs `{code}: all shards failed`.",
    "Steps": "1. `curl -s search-1:9200/_cluster/health?pretty` and note the unassigned shards.\n2. `GET _cluster/allocation/explain` says why a shard can't be placed: usually a full disk on a data node.\n3. Free the disk or add a data node; the shards then allocate by themselves.\n4. Never delete an index to make the cluster green.",
    "Verify": "Cluster health is green and search results are complete again.",
})


def render(d, facts, long=False, supersedes=None, archived=False, footer=None, acl=False):
    vals = dict(facts)
    vals["code"] = facts["codes"][d["id"]]
    vals["team"] = d["team"]
    fm = [f"id: {d['id']}", f"title: {d['title']}", f"team: {d['team']}"]
    if archived:
        fm.append("status: archived")
    else:
        fm.append("status: current")
    if supersedes:
        fm.append(f"supersedes: {supersedes}")
    if acl:
        fm.append("visibility: " + ("private" if d["id"] in PRIVATE else "public"))
    out = ["---", *fm, "---", "", f"# {d['title']}", ""]
    secs = list(d["sections"].items())
    if long:
        secs = [("Background", BACKGROUND), ("History and monitoring", HISTORY), *secs, ("Reference", REFERENCE)]
    for h, t in secs:
        out += [f"## {h}", "", t.format(**vals), ""]
    if footer:
        out += ["---", footer, ""]
    return "\n".join(out)


def write(dirpath, state, flags):
    r = random.Random()
    facts = rnd_facts(r)
    os.makedirs(dirpath, exist_ok=True)
    only = None
    if "--only" in flags:
        only = set(flags[flags.index("--only") + 1].split(","))
    long = "--long" in flags
    stale = "--stale" in flags
    dups = "--dups" in flags
    acl = "--acl" in flags
    sup = {s["supersededby"]: s["id"] for s in STALE} if stale else {}
    written = []
    for d in DOCS:
        if only and d["id"] not in only:
            continue
        text = render(d, facts, long=long and d["id"] in LONG, supersedes=sup.get(d["id"]), acl=acl)
        with open(os.path.join(dirpath, d["id"] + ".md"), "w") as f:
            f.write(text)
        written.append(d["id"])
        if dups and d["id"] in DUPS:
            for i, cid in enumerate(DUPS[d["id"]]):
                space = cid.split("-space")[0] if "-space" in cid else "ops"
                foot = f"Exported from the {space.upper()} wiki space on 2026-0{i + 3}-1{i}. Copy kept for reference."
                with open(os.path.join(dirpath, cid + ".md"), "w") as f:
                    f.write(render(d, facts, footer=foot).replace(f"id: {d['id']}", f"id: {cid}", 1))
                written.append(cid)
    if stale:
        for s in STALE:
            with open(os.path.join(dirpath, s["id"] + ".md"), "w") as f:
                f.write(render(s, facts, archived=s["archived"]))
            written.append(s["id"])
    facts["flags"] = [f for f in flags if f.startswith("--")]
    facts["docs"] = written
    with open(state, "w") as f:
        json.dump(facts, f, indent=1)


def change(dirpath, state):
    """A day of wiki edits: db-failover gets a new standby, cost-spike is deleted and
    search-red is added (or edited, when it is already there). Each call is a new edit."""
    facts = json.load(open(state))
    r = random.Random()
    p = os.path.join(dirpath, "db-failover.md")
    new = f"pg-{r.choice(['north', 'south', 'east', 'west'])}{r.randint(100, 999)}"
    text = open(p).read().replace(facts["standby"], new)
    with open(p, "w") as f:
        f.write(text)
    facts["standby_old"], facts["standby"] = facts["standby"], new
    gone = os.path.join(dirpath, "cost-spike.md")
    if os.path.exists(gone):
        os.remove(gone)
    p = os.path.join(dirpath, "search-red.md")
    if os.path.exists(p):
        with open(p, "a") as f:
            f.write(f"Reviewed by the platform team, revision {r.randint(1000, 9999)}.\n")
    else:
        with open(p, "w") as f:
            f.write(render(EXTRA, facts))
    facts["docs"] = [d for d in facts["docs"] if d not in ("cost-spike", "search-red")] + ["search-red"]
    json.dump(facts, open(state, "w"), indent=1)


def questions(state, kinds):
    facts = json.load(open(state))
    docs = set(facts["docs"])
    dupgroups = {k: [k, *v] for k, v in DUPS.items()}
    def want(i):
        return dupgroups.get(i, [i]) if "--dups" in facts["flags"] else [i]
    out = []
    if "semantic" in kinds:
        for i, qs in SEMANTIC.items():
            if i in docs:
                out += [dict(q=q, want=want(i), kind="semantic") for q in qs]
    if "para" in kinds:
        out += [dict(q=q, want=want(i), kind="para") for i, q in PARA.items() if i in docs]
    if "code" in kinds:
        for d in DOCS:
            if d["id"] in docs:
                c = facts["codes"][d["id"]]
                out.append(dict(q=f"The logs keep showing {c}. Which runbook covers it?", want=want(d["id"]), kind="code", code=c))
                out.append(dict(q=f"Since this morning we see {c} on the orders service. What is the procedure?", want=want(d["id"]), kind="code", code=c))
    if "long" in kinds:
        out += [dict(q=q, want=[i], kind="long") for i, q in LONGQ.items() if i in docs]
    if "fact" in kinds:
        out += [dict(q=q, want=[i], kind="fact", fact=facts[k]) for i, k, q in FACTQ if i in docs]
    if "stale" in kinds:
        for s in STALE:
            i = s["supersededby"]
            out += [dict(q=q, want=[i], kind="stale", old=s["id"]) for q in SEMANTIC[i]]
        out.append(dict(q="How do I fail over the primary database?", want=["db-failover"], kind="stale", old="db-failover-2023"))
        out.append(dict(q="How do I get VPN access for a new engineer?", want=["vpn-access"], kind="stale", old="vpn-howto"))
    if "hard" in kinds:
        out += [dict(q=q, want=[i], kind="hard") for i, q in HARDQ if i in docs]
    if "acl" in kinds:
        # (asking team, question doc, question): a private page only for its own team
        for team, i in [("platform", "leaked-secret"), ("platform", "payment-webhooks"), ("payments", "cost-spike"),
                        ("security", "cert-inventory"), ("payments", "payment-webhooks"), ("finops", "cost-spike"),
                        ("security", "leaked-secret"), ("platform", "disk-full"), ("payments", "db-failover"),
                        ("network", "cert-inventory"), ("ops' OR 'a'='a", "leaked-secret")]:
            d = next(x for x in DOCS if x["id"] == i)
            ok = i not in PRIVATE or d["team"] == team
            for q in SEMANTIC[i][:1]:
                out.append(dict(q=q, args=["--team", team], want=[i] if ok else [], kind="acl", team=team,
                                hidden=[] if ok else [i]))
    if "changed" in kinds:
        out.append(dict(q="Which server do I promote when the primary database is down?", want=["db-failover"], kind="changed", fact=facts["standby"], old=facts.get("standby_old")))
        out.append(dict(q="The product search cluster is red and shards are unassigned. What do I do?", want=["search-red"], kind="changed"))
        out.append(dict(q="Why did our cloud spend jump overnight and how do I find the cause?", want=[], kind="changed", gone="cost-spike"))
    if "none" in kinds:
        out += [dict(q=q, want=[], kind="none") for q in NONE]
    for o in out:
        print(json.dumps(o))


if __name__ == "__main__":
    a = sys.argv[1:]
    if a and a[0] == "write" and len(a) >= 3:
        write(a[1], a[2], a[3:])
    elif a and a[0] == "change" and len(a) == 3:
        change(a[1], a[2])
    elif a and a[0] == "questions" and len(a) >= 2:
        questions(a[1], (a[2] if len(a) > 2 else "semantic").split(","))
    else:
        sys.exit(__doc__)
