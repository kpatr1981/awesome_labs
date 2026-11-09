# Pending ideas

Ideas for future labs. Each topic has one home; where tracks touch, the row says which
track it relies on.

**Target: 250 labs in each of the ten tracks, 2,500 in total.** Once tracks reach it, review
what to consolidate. The target doesn't lower the bar: a lab should carry a real multi-step
scenario and teach a concept. The parts of this list that the existing exercises don't cover
are not started.

Exercise counts as of v0.11.0:

| Topic | Exercises | Target | Track | Module files |
|---|---|---|---|---|
| Docker | 149 | 250 | `docker` | `exercises/docker/` |
| Kubernetes | 234 | 250 | `kubernetes` | `exercises/kubernetes/` |
| Offensive security | 45 | 250 | `offsec` | `exercises/offsec/` |
| Linux & systems | 34 | 250 | `linux` | `exercises/linux/` |
| Networking | 42 | 250 | `networking` | `exercises/networking/` |
| Cloud & platform | 47 | 250 | `cloud` | `exercises/cloud/` |
| Data | 27 | 250 | `data` | `exercises/data/` |
| Defensive security | 30 | 250 | `defensive` | `exercises/defensive/` |
| Programming | 45 | 250 | `programming` | `exercises/programming/` |
| Data science | 43 | 250 | `datascience` | `exercises/datascience/` |

- [Suggested order](#suggested-order)
- [Shared lab foundations](#shared-lab-foundations)
- [Linux & systems](#linux--systems)
- [Networking](#networking)
- [Cloud & platform](#cloud--platform)
- [Data](#data)
- [Security](#security)
- [Programming](#programming)
- [Practice modes](#practice-modes)
- [awesome_labs features](#awesome_labs-features)
- [More lab ideas](#more-lab-ideas)
- [Left out for now](#left-out-for-now)

## Suggested order

1. **Quick wins, no new infrastructure:** Linux admin first, then Git and Shell. They run
   on the existing VMs and are easy to check.
2. **Networking & IP routing**, which the rest of the networking group builds on.
3. **TLS & PKI**, before OpenVPN and the mail TLS exercises.
4. **Degraded networks and VPN creation.**
5. **DNS & email**, with DNS first, because MX records, SPF, DKIM and DMARC are all DNS.
6. **Everything else**, in any order. Container internals is the most valuable of these.

## Shared lab foundations

These apply to several tracks, so they're listed once here.

### Test before building

| Check | Needed by | Status |
|---|---|---|
| `/dev/kvm` inside the lab VMs | QEMU | Works in `kl-docker`: the 64MB Alpine virt 3.22 ISO reaches a login prompt on the serial console in 5s, under KVM in a container with 256MiB. Avoid CirrOS, which stalls on metadata requests |
| containerlab on the 3GiB Docker VM | Networking and tracks built on it | Works: six FRR 9.1 routers in a ring deployed and linked with containerlab 0.59, using about 150MiB in total |
| `sch_netem` kernel module | Degraded networks | Works in `kl-docker`, and `net-005` uses it |
| WireGuard kernel support | VPN creation | Works in `kl-docker`, and `net-015` uses it |
| Moto's optional IAM enforcement | AWS CLI | Works on 5.2.4, the version the labs pin, with `INITIAL_NO_AUTH_ACTION_COUNT`: a user's policy allowed one S3 call and the next was `AccessDenied` |

### Multi-network labs

For Networking, Degraded networks, VPN creation and DNS & email.

- **Layout:** each router, site or organisation is a container or network namespace on the
  Docker VM, so several networks need no extra VMs.
- **Topologies:** [containerlab](https://containerlab.dev) describes a network of
  containers in YAML.
- **Docker firewall:** Docker's rules block traffic between its bridge networks. Connect
  routers with direct veth links, as containerlab does.
- **Docker resolver:** containers use Docker's built-in resolver by default, so point each
  one at the lab's own resolver.
- **Lockout risk:** a bad route, a full tunnel or netem on a VM's main interface can cut
  the SSH connection awesome_labs depends on. Keep labs inside containers or namespaces. Netem
  rules on k8s nodes should target only cluster traffic, or remove themselves after a
  timeout.
- **Permissions:** containers changing their own networking need `NET_ADMIN`, and OpenVPN
  also needs `/dev/net/tun`. Applying netem from the host side avoids giving learners
  `NET_ADMIN`.
- **Reset:** tearing down a topology clears nearly everything. Reset hooks must also remove
  leftover netem rules, namespaces, interfaces and routes.

### Checks and setup

- **Timing:** poll with a timeout for anything that converges, such as OSPF and BGP.
- **Randomness:** netem loss is random, so use generous thresholds and several samples, or
  inspect `tc qdisc show` directly.
- **Downloads:** pull toolchains, guest images and service images during bootstrap, so
  exercises don't need the network.
- **Editing:** learners only have `vim` or `nano` inside the VM. Larger exercises, above
  all programming ones, need `/lab` shared with the host.

## Linux & systems

| Track | What you'd practise | Runs on |
|---|---|---|
| **Linux admin & troubleshooting** | systemd units, full disks and inodes, LVM, permissions and ACLs, runaway processes | Docker VM; cheapest to build |
| **Shell & text processing** | Bash scripting, `awk`, `sed`, `jq`, log parsing, fixing buggy scripts | Any VM; very easy to check |
| **Git** | Done in `exercises/linux/06-git.yaml`: merge and rebase conflicts, recovering lost work, hooks, cleaning secrets out of history; `bisect` in `linux-015` | Any VM; very easy to check |
| **Performance debugging** | `strace`, `perf`, `bpftrace`, memory leaks, I/O bottlenecks, CPU throttling | Docker VM; checks are harder to write |
| **Container internals** | Namespaces and cgroups by hand, building a container from scratch, `runc`, `ctr` and `crictl`, overlayfs | Docker VM or k8s nodes |
| **QEMU & virtualization** | See below | Docker VM or a dedicated VM; needs nested KVM |

Linux admin relies on other tracks for two topics: routing, DNS and firewalls are in
Networking and DNS & email, and boot failures are built with the QEMU rescue module.

### QEMU modules

| Module | Exercises |
|---|---|
| **Disk images** | `qemu-img` create, convert and resize, qcow2 backing files, snapshots, repairing a corrupt image |
| **Booting guests** | Writing the QEMU command by hand, serial console, UEFI vs BIOS, cloud-init seed images |
| **Networking** | User-mode port forwards, tap devices and bridges, guests that can't reach each other |
| **Devices & performance** | virtio vs emulated devices, CPU models, KVM vs software emulation, disk cache modes |
| **Control & management** | The QEMU monitor and QMP, live migration between two QEMU processes, libvirt and `virsh` |
| **Rescue** | Unbootable guests: broken `fstab`, GRUB or initramfs, fixed with `guestfish` or a rescue boot |

Rescue is the most valuable module: it breaks a throwaway guest and not the lab VM.

- **Memory:** lab VMs have 3GiB, so use tiny guests (CirrOS or Alpine, 128–256MB). Two or
  three fit comfortably.
- **Checks:** script them with `qemu-img info`, QMP, `virsh` or SSH into the guest. Guest
  boot time will make `awesome_labs verify` slower.
- **macOS:** expect no KVM inside the VMs, so guests fall back to much slower software
  emulation. Timing-sensitive exercises may not work there.

## Networking

| Track | What you'd practise | Runs on | Relies on |
|---|---|---|---|
| **Networking & IP routing** | Addressing to BGP, firewalls, packet analysis | Docker VM; k8s lab for Calico and MetalLB | Nothing; the foundation |
| **TLS & PKI** | `openssl`, broken certificate chains, expiry, mTLS, running a small CA, cert-manager | Docker VM; k8s lab for cert-manager | Nothing |
| **Web servers & proxies** | nginx and HAProxy config, caching, load balancing, timeouts, 502 and 504 hunts | Docker VM; light | TLS & PKI, loosely |
| **Degraded networks (netem)** | Delay, loss, reordering, corruption and bandwidth caps | Docker VM and k8s lab | Networking |
| **VPN creation** | WireGuard, OpenVPN, IPsec, mesh VPNs | Docker VM; k8s lab for Calico | Networking, TLS & PKI |
| **DNS & email** | A private DNS hierarchy, DNSSEC, Postfix and Dovecot, SPF, DKIM, DMARC | Docker VM; k8s lab for CoreDNS and ExternalDNS | Networking, TLS & PKI |

### Networking & IP routing modules

| Module | Exercises |
|---|---|
| **Addressing, subnets & DHCP** | CIDR, wrong netmasks, overlapping ranges, multiple addresses, default gateways, DHCP with dnsmasq |
| **Layer 2** | ARP, Linux bridges, VLANs, bonding, duplicate IPs, broadcast problems |
| **Static routing** | Multi-hop paths, forwarding, return-path failures, longest-prefix match, asymmetric routes |
| **Policy routing** | Multiple routing tables, `ip rule`, two uplinks, source-based routing, `rp_filter` drops |
| **NAT & firewalls** | nftables, SNAT and DNAT, port forwards, conntrack, hairpin NAT |
| **IPv6** | OSPFv3, dual-stack repair and BGP over IPv6 done in `07-ipv6-routing.yaml`; neighbor discovery in `net-018`. SLAAC not yet covered |
| **OSPF** | Done in `06-dynamic-routing.yaml`: FRR neighbors, areas, costs, timer mismatches, failover |
| **BGP** | Done in `06-dynamic-routing.yaml`: peering, route filters, communities, path selection, route leaks |
| **High availability** | VRRP (FRR's `vrrpd`) and ECMP done in `06-dynamic-routing.yaml`; anycast in `net-035` |
| **Packet analysis** | `tcpdump` and `tshark`, the TCP handshake, MTU problems, finding faults from captures |
| **Kubernetes** | Done in `exercises/kubernetes/19-bgp-and-external-routing.yaml`: peer Calico with an FRR router over BGP, MetalLB in BGP mode, trace a packet from pod to outside |

- **Router software:** FRR is open source and its CLI is close to Cisco's, so no licensed
  vendor images are needed.
- **Checks:** ping and `traceroute` for the real path, plus `vtysh` to inspect OSPF and BGP
  state.

### Degraded networks modules

Start with Hunt the bad link, which is cheap and needs only containers.

| Module | Exercises | Runs on |
|---|---|---|
| **Learn the tools** | Build `tc` and netem profiles, combine them with HTB, shape only one destination, shape ingress with `ifb` | Docker VM |
| **Hunt the bad link** | A chain of containers has one impaired hop: find it with `mtr`, `ss -ti`, `iperf3` and `tcpdump` retransmits | Docker VM |
| **Make apps resilient** | Add timeouts, retries with backoff and connection pools; tune nginx upstream timeouts until the service survives loss | Docker VM; ties into Python and Go |
| **TCP & transfers** | Window sizes on high-latency links, BBR vs CUBIC under loss, slow large transfers, HTTP/2 vs HTTP/3 | Docker VM |
| **Kubernetes under bad networks** | Probes flapping on latency, a slow node, DNS timeouts from UDP loss, one pod's traffic degraded via its veth | k8s lab |
| **Cluster control plane** | etcd leader elections under latency, heartbeat tuning, nodes going `NotReady` | k8s lab; needs care |

Replication over bad links is in [Data](#data), and netem-based replays of existing
exercises are in [Practice modes](#practice-modes).

### VPN creation modules

Build WireGuard first, then OpenVPN, then IPsec (hardest, but still the most common
between companies).

| Module | Exercises |
|---|---|
| **WireGuard basics** | Generate keys, bring up a point-to-point tunnel, understand `AllowedIPs` and `wg-quick` |
| **Topologies** | Road warrior, site-to-site between two LANs, hub-and-spoke, full mesh |
| **Routing & DNS** | Split vs full tunnel, routing between tunnels and LANs, pushing DNS, a firewall kill switch |
| **OpenVPN** | Server and client config using certificates from a small CA, revoking a client, TCP vs UDP |
| **IPsec** | strongSwan site-to-site, IKEv2, mismatched proposals, NAT traversal |
| **Mesh VPN** | Self-hosted Headscale with Tailscale clients, ACLs, relays when direct paths fail |
| **Troubleshooting** | No handshake, wrong `AllowedIPs`, routes missing on one side, tunnel MTU black holes, NAT timeouts, expired certificates, VPNs on lossy links, TCP-over-TCP slowdowns |
| **Kubernetes** | Turn on Calico's WireGuard encryption, connect two sites, reach a cluster service over a tunnel |

- **Checks:** ping through the tunnel, confirm the handshake with `wg show`, and capture
  the underlying link to prove the traffic is encrypted.

### DNS & email modules

A whole private internet can run in containers, with its own root, TLD and domains, and
several organisations sending mail to each other.

DNS:

| Module | Exercises |
|---|---|
| **Tools & resolution** | `dig +trace`, record types, following a query from the root down |
| **Authoritative servers** | BIND zones, SOA serials, reverse zones, syntax errors, forgotten serial bumps |
| **Delegation** | Build root → TLD → domain, glue records, lame delegations |
| **Replication** | Primary and secondary servers, AXFR and IXFR, TSIG keys, stale secondaries |
| **Resolvers & caching** | Unbound, forwarding, TTLs, negative caching, split-horizon |
| **DNSSEC** | Sign a zone, DS records, key rollover, debugging a broken chain |
| **Encrypted & Kubernetes DNS** | DNS over TLS, CoreDNS plugins and stub domains, ExternalDNS |

Email:

| Module | Exercises |
|---|---|
| **Postfix basics** | Send and receive, MX lookup, aliases, virtual domains |
| **Mailboxes** | Dovecot IMAP, Maildir, quotas, connecting Postfix to Dovecot |
| **Submission & TLS** | SASL login, STARTTLS, port 25 vs 587 |
| **Authentication** | SPF, DKIM signing, DMARC policies, reading `Authentication-Results` headers |
| **Fixing mail flow** | Stuck queues, deferrals, bounces, mail loops, broken reverse DNS |
| **Spam & abuse** | Close an open relay, rspamd rules, greylisting, rate limits |
| **Incidents** | Combined failures, such as a DNS change that quietly breaks DKIM for one domain |

- **Isolation:** use reserved names such as `.test`, and block outbound port 25 and
  external DNS from the lab network, so a misconfiguration can never send real mail.
- **Checks:** `dig` answers for DNS; for email, send with `swaks`, then inspect the
  delivered message and its headers. Flush resolver caches in checks and resets.
- **Clock:** DNSSEC signatures depend on the VM's clock being right.
- **Memory:** Postfix, Dovecot and BIND are light. rspamd with Redis is heavier, so run it
  only in the spam module.

## Cloud & platform

| Track | What you'd practise | Runs on |
|---|---|---|
| **AWS CLI** | CLI fluency, S3, IAM policy documents, `--query` filtering, DynamoDB, SQS and SNS, CloudFormation | Docker VM; moto server in a container |
| **Infrastructure as code** | Ansible against the lab VMs, Terraform state problems, drift, broken modules, the AWS provider against moto | Existing VMs; light |
| **Observability** | Prometheus queries and alerts, Grafana, Loki, finding why a metric or alert is missing | k8s lab; may need more memory |
| **CI/CD & GitOps** | Gitea and a runner, broken pipelines, Argo CD drift and sync failures | k8s lab; memory-heavy |

### AWS CLI notes

Use the real `aws` CLI with `AWS_ENDPOINT_URL` pointing at `motoserver/moto` (port 5000).
Checks can query state with the same CLI, and a reset hook can wipe it with
`POST /moto-api/reset`. LocalStack is out because it has needed an account and auth token
since March 2026.

Moto only emulates the API, so these limits apply:

- EC2 instances are only records: no SSH, and security groups don't block anything.
  Networking labs won't work.
- IAM isn't enforced by default.
- Service coverage varies, so prove each exercise with `awesome_labs verify`.

## Data

| Track | What you'd practise | Runs on |
|---|---|---|
| **Databases & SQL** | See below | Docker VM; easy to check |
| **Messaging & caching** | Redis persistence and eviction, Redis replicas resyncing over bad links (netem), RabbitMQ queues and dead letters | Docker VM; Kafka would be heavier |

### Databases & SQL modules

| Module | Exercises |
|---|---|
| **SQL** | Joins and window functions |
| **Query performance** | Reading query plans, fixing slow queries with indexes, locks |
| **Backup & restore** | Backup and point-in-time restore |
| **Replication** | Postgres replication, lag and stalled synchronous commits over bad links (netem) |
| **Migrations** | Failed migrations |

## Security

| Track | What you'd practise | Runs on |
|---|---|---|
| **Offensive security** | See below | Offsec VM |
| **Defensive security & forensics** | Find the intrusion in logs, auditd, Falco rules, spot persistence, contain and patch | Offsec VM |
| **Supply chain** | SBOMs, Trivy, cosign signing, admission policies with Kyverno, pinning digests | Docker or k8s lab; light |
| **Secrets management** | Vault policies, dynamic credentials, rotation, Kubernetes integration | Docker or k8s lab; light |

### Offensive security modules

The track has 45 exercises so far.

- **Network and credentials:** enumeration, service misconfigurations, hash cracking,
  pivoting between containers.
- **Container and Kubernetes attacks:** container escapes, abusing over-broad RBAC, stolen
  service account tokens, and then fixing each one.
- **Advanced web:** SSTI, deserialization, race conditions, OAuth flaws, business logic
  bugs.

## Programming

Focus on "this program is broken on a real system, fix it" and not on syntax tutorials,
which other sites already cover well. SQL is under [Data](#data).

| Track | What you'd practise | Runs on |
|---|---|---|
| **Python** | Automation scripts, boto3 against moto, the Kubernetes client, packaging and virtualenv breakage, `pytest`, async bugs | Docker VM; ties into AWS CLI |
| **Go** | Goroutine leaks and data races, `pprof`, small static builds, `client-go`, writing a Kubernetes controller | Docker VM; k8s lab for controllers |

- **Checks:** run a hidden test suite plus real behaviour, such as "the service stays up
  under load" or "memory no longer grows". Keep tests outside `/lab` so they can't be
  edited by accident.
- **Toolchains:** run each language in a container on the Docker VM, so no new VM is
  needed.

## Practice modes

New ways to use exercises, most of them reusing existing content.

- **Attack, then defend:** exploit a target, then patch it and prove the exploit no longer
  works. Candidates: zone transfer leaks, open relays, and pivoting through a compromised
  VPN client and then restricting it with firewall rules.
- **On-call mode:** a random fault is injected into a healthy system with no scenario text,
  only an alert, and you're timed to fix it. Netem can be one of the faults.
- **Hard mode:** run any existing exercise with a degraded network (netem), so it's
  replayable without writing new content.
- **Multi-stage campaigns:** several linked steps on one lab, where each step's result
  unlocks the next. Example: fix the bug, rebuild the image, roll it out to the cluster.
- **Review mode:** brings solved exercises back after a few days or weeks, using the dates
  already in `progress.json`.
- **Daily challenge:** one exercise a day, picked from what isn't solved yet, with a streak
  counter.
- **Command golf:** records the shell session and compares the number of commands with the
  reference solution.

## awesome_labs features

- **Weak spots:** shows which modules cost the most hints, checks and solution views. The
  data is already recorded.
- **Exercise packs:** install extra exercises from a Git repo, building on the existing
  `AWESOME_LABS_EXERCISES` setting.

## More lab ideas

Not started. All should fit the existing VMs unless noted. The first table is the suggested
starting point.

| Idea | The lab | Builds on |
|---|---|---|
| **OpenTofu against Moto** | Fix state drift, import hand-made resources, recover a broken plan | The AWS emulator |
| **VXLAN and EVPN fabric** | Leaf-spine on FRR with a stretched Layer 2 network, then debug a missing VNI | The FRR helpers |
| **Prometheus and alerting** | Write the rule that would have caught an outage, fix an alert that never fires | Docker or k8s lab |
| **eBPF with bpftrace** | Find which process touches a file, or what causes latency, without restarting anything | Docker VM |
| **End-to-end capstones** | Dockerfile to registry to cluster to TLS ingress to metrics, with faults at each step | Docker and k8s labs |

| Area | Ideas |
|---|---|
| **Linux and performance** | Flame graphs with `perf`; cgroups v2 by hand, with no container runtime; `mdadm` RAID failure and rebuild on loop devices; systemd sandboxing scored with `systemd-analyze security` |
| **Data** | Postgres point-in-time recovery after a bad `DELETE`; replication with a lagging replica, a broken slot and a failover; Redis Sentinel split-brain and the writes it loses |
| **Delivery and Kubernetes** | Broken Helm charts and a failed upgrade to roll back; GitOps drift (Argo CD may be tight on these nodes); killing a node under load while meeting an availability target |
| **Security** | SSH certificates from a small CA in place of `authorized_keys`; WAF tuning that blocks an attack and not real users; writing a detection from a real attack's logs and proving it catches a variant |
| **Networking** | BGP hijack, then stopping it with RPKI route validation |

### Useful but niche

Problems that are rare until they ruin your day. The first three are the suggested starting point.

| Idea | The lab |
|---|---|
| **Boot repair in a QEMU guest** | Break `initramfs` or `fstab` in the Alpine guest and rescue it from the console |
| **Conntrack table full** | Packets drop at random, and the only clue is one kernel log line |
| **Admission webhook outage** | The webhook is down, and now nothing can be created |
| **Reading AppArmor denials** | A service fails with "permission denied" while the file permissions are fine |
| **Seccomp profile from `strace`** | Build the smallest profile that lets an app run |
| **Core dumps with `gdb`** | A crash with no logs: find the failing line from the dump |
| **Ephemeral port exhaustion** | Connections fail under load with plenty of CPU and memory free |
| **PMTUD black hole** | Small requests work and large ones hang through a tunnel |

| Area | Ideas |
|---|---|
| **Identity and time** | Clock skew breaking TLS, tokens and logs, fixed with `chrony`; a bad PAM change recovered without losing the last root session; local ACME with Pebble and a failed challenge; timezone and DST cron traps |
| **Kubernetes corners** | A namespace stuck `Terminating` on a finalizer; `ndots:5` DNS storms |
| **Data and storage** | Redis AOF corruption and what was lost; PgBouncer pool exhaustion with an idle database; NFS root squash and stale file handles; sparse files and `rsync` |

### Packet capture labs

Only `offsec-for-001` and `net-016` use captures today, both at low difficulty, and the defensive
track has none. Build the defensive set first. `tshark` isn't on the VMs yet, only `tcpdump`:
add it to provisioning, pinned like the other packages, and fetch it on first use on existing VMs.

| Track | Ideas |
|---|---|
| **Defensive** | Reconstruct an intrusion from a capture (timeline, attacker's host, what was taken); find DNS tunnelling and recover the data; find C2 beaconing among normal traffic; carve a dropped payload from an HTTP stream; read TLS metadata (SNI, certificates, handshake) without decrypting |
| **Offsec** | Sniff plaintext credentials on an isolated lab network; ARP spoofing between namespaces, then the defender's view of it; reverse a custom binary protocol from captures |
| **Networking** | Diagnose faults from captures: retransmissions, zero windows, RST storms and MTU problems |

### CI/CD pipeline labs

No lab runs a real pipeline today: `docker-120`, `k8s-203`, `k8s-220` and `cloud-049` only use CI
as the story. Fleshes out the **CI/CD & GitOps** row above. Gitea and a runner look light enough
for the 3GiB Docker VM, which still needs testing. Argo CD needs a feasibility test first, on
nodes that already run the cluster.

| Lab | What goes wrong |
|---|---|
| **A build that fails only in CI** | It works locally, but the runner has a different environment |
| **Secrets in pipelines** | One masked in logs, and one leaking through a step that prints it |
| **Caching and artifacts** | A stale cache serves old code, and a build's output must pass between jobs |
| **Deploy gates** | Tests must pass before an image reaches the registry |
| **A compromised pipeline** | A pull request tries to steal a secret, then the fix |
| **Full delivery** | Commit to image to cluster, with a rollback when the health check fails |

### Object storage labs (MinIO)

The cloud track uses S3 through the Moto emulator, so learners practise the API and never operate
object storage. MinIO should fit on the Docker VM, not yet tried. **Check first:** MinIO's community
edition may have lost admin features and changed how it distributes builds. Confirm a version can
still be pinned and pulled. If not, consider Garage or SeaweedFS, though the commands would differ.

| Lab | What it covers |
|---|---|
| **Erasure coding and a failed disk** | Lose drives, keep reading, then heal and prove the data is intact |
| **Going past the limit** | Lose one drive too many, and see exactly what stops working |
| **Versioning and accidental deletes** | Recover an object from a delete marker |
| **Lifecycle rules** | Expire old versions, and debug the rule that matches nothing |
| **Bucket policies and service accounts** | A real access-denied to debug on a real server |
| **Replication between two sites** | One site goes down, then resync it |
| **Presigned URLs and clock skew** | A link that fails because of the time |
| **Object storage for Kubernetes** | An app or backup job writing to it from the cluster |

### Authentication and authorization labs

Today only basics and token attacks are covered: no OAuth2, OIDC, SSO, Keycloak or LDAP. Keycloak
is the main tool, because it's the most common self-hosted identity provider. The OIDC concepts
carry over to Okta, Entra ID and Auth0, which can't run in a lab.

**Tested on the 3GiB Docker VM with Keycloak 26.4.7 in `start-dev`:** ready in about 18s, 475MiB idle
and about 515MiB after creating a realm, client and user and logging in. Works under an 800MiB
limit, and is OOM-killed at 500MiB, so give it 800MiB. The image is 684MB, so pre-pull it.

| Lab | What it covers | Tool |
|---|---|---|
| **Stand up a realm** | Realm, clients, users and roles | Keycloak |
| **Protect an app with OIDC** | Put a real web app behind login | Keycloak, oauth2-proxy |
| **Debug a broken login** | Wrong redirect URI, wrong audience, clock skew, expired token | Keycloak |
| **401 versus 403** | A valid token that lacks the right role | Keycloak |
| **Service-to-service auth** | Client credentials and token validation | Keycloak |
| **LDAP federation** | Connect a directory and fix a failing bind or group mapping | Keycloak, lldap |
| **Kubernetes OIDC** | Log in to the API server and map groups to RBAC | Dex, because Keycloak is too heavy for the 3GiB and 2.5GiB nodes |

## Left out for now

- **Service mesh:** Istio would strain the 3GiB and 2.5GiB nodes for relatively little
  payoff.
