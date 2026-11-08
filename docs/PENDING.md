# Pending ideas

Ideas for future labs. Nothing here is started yet.

## New tracks

Linux administration is the suggested next one: it runs on the existing VMs and its
checks are easy to script.

| Idea | What you'd practise | Fit with the current setup |
|---|---|---|
| **Linux admin & troubleshooting** | systemd units, full disks and inodes, LVM, permissions and ACLs, DNS and routing, firewall rules, runaway processes, boot failures | Existing Docker VM; cheapest to build |
| **Defensive security & forensics** | Find the intrusion in logs, auditd, Falco rules, spot persistence, contain and patch | Offsec VM; pairs with the attack modules |
| **Observability** | Prometheus queries and alerts, Grafana, Loki, finding why a metric or alert is missing | k8s lab; may need more memory |
| **CI/CD & GitOps** | Gitea and a runner, broken pipelines, Argo CD drift and sync failures | k8s lab; memory-heavy |
| **Supply chain** | SBOMs, Trivy, cosign signing, admission policies with Kyverno, pinning digests | Docker or k8s lab; light |
| **Infrastructure as code** | Ansible against the lab VMs, Terraform state problems, drift, broken modules | Existing VMs; light |
| **Database operations** | Postgres replication, backup and point-in-time restore, slow queries, locks, failed migrations | Docker VM; easy to check |
| **AWS CLI** | CLI fluency, S3, IAM policy documents, `--query` filtering, DynamoDB, SQS and SNS, CloudFormation or Terraform against a fake endpoint | Docker VM; moto server in a container |
| **Git** | Merge and rebase conflicts, recovering lost commits with reflog, bisect, hooks, cleaning secrets out of history | Any VM; very easy to check |
| **Container internals** | Namespaces and cgroups by hand, building a container from scratch, `runc`, `ctr` and `crictl`, overlayfs | Docker VM or k8s nodes |
| **Shell & text processing** | Bash scripting, `awk`, `sed`, `jq`, log parsing, fixing buggy scripts | Any VM; very easy to check |
| **TLS & PKI** | `openssl`, broken certificate chains, expiry, mTLS, running a small CA, cert-manager | Docker VM, k8s for cert-manager |
| **Networking & IP routing** | Addressing to BGP, including `tcpdump`, packet captures, MTU problems and nftables NAT: see the modules below | Docker VM with containers as routers; k8s lab for Calico and MetalLB |
| **Web servers & proxies** | nginx and HAProxy config, caching, load balancing, timeouts, 502 and 504 hunts | Docker VM; light |
| **Performance debugging** | `strace`, `perf`, `bpftrace`, memory leaks, I/O bottlenecks, CPU throttling | Docker VM; checks are harder to write |
| **Secrets management** | Vault policies, dynamic credentials, rotation, Kubernetes integration | Docker or k8s lab; light |
| **Messaging & caching** | Redis persistence and eviction, RabbitMQ queues and dead letters | Docker VM; Kafka would be heavier |
| **QEMU & virtualization** | `qemu-img` and qcow2, booting guests by hand, cloud-init, tap and bridge networking, virtio, QMP, live migration, libvirt, rescuing unbootable guests | Docker VM or a dedicated VM; needs nested KVM |
| **Python** | Automation scripts, boto3 against moto, the Kubernetes client, packaging and virtualenv breakage, `pytest`, async bugs | Docker VM; ties into the AWS track |
| **Go** | Goroutine leaks and data races, `pprof`, small static builds, `client-go`, writing a Kubernetes controller | Docker VM, k8s lab for controllers |
| **SQL** | Joins and window functions, reading query plans, fixing slow queries with indexes | Docker VM; shares setup with the database track |
| **Degraded networks (netem)** | Delay, loss, reordering, corruption and bandwidth caps: see the modules below | Docker VM and k8s lab |
| **VPN creation** | WireGuard, OpenVPN, IPsec, mesh VPNs, routing and troubleshooting: see the modules below | Docker VM with containers as sites; k8s lab for Calico |
| **DNS & email** | A private DNS hierarchy, DNSSEC, Postfix and Dovecot, SPF, DKIM, DMARC, mail-flow troubleshooting: see the modules below | Docker VM with containers as organisations; k8s lab for CoreDNS and ExternalDNS |

Service mesh was considered and left out for now: Istio would strain the 3GiB and
2.5GiB nodes for relatively little payoff.

### AWS CLI notes

Use the real `aws` CLI with `AWS_ENDPOINT_URL` pointing at `motoserver/moto` (port 5000).
Checks can query state with the same CLI, and a reset hook can wipe it with
`POST /moto-api/reset`. LocalStack is out because it has needed an account and auth
token since March 2026.

Moto only emulates the API, so these limits apply:

- EC2 instances are only records: no SSH, and security groups don't block anything.
  Networking labs won't work.
- IAM isn't enforced by default. Test the optional enforcement mode before building
  permission-debugging exercises on it.
- Service coverage varies, so prove each exercise with `kubelab verify`.

### QEMU notes

The rescue module (broken `fstab`, GRUB or initramfs, fixed with `guestfish` or a rescue
boot) is the most valuable part. It is also how the boot failures in the Linux admin
track should be built, so they break a throwaway guest and not the lab VM.

- **Nested KVM:** enabled on the Linux host, but `/dev/kvm` inside the lab VMs is still
  untested. Check that first.
- **Memory:** lab VMs have 3GiB, so use tiny guests (CirrOS or Alpine, 128–256MB). Two
  or three fit comfortably.
- **Setup:** download guest images during bootstrap so exercises don't need the network.
- **Checks:** script them with `qemu-img info`, QMP, `virsh` or SSH into the guest. Guest
  boot time will make `kubelab verify` slower.
- **macOS:** expect no KVM inside the VMs, so guests fall back to much slower software
  emulation. Timing-sensitive exercises may not work there.

### Programming language notes

Focus on "this program is broken on a real system, fix it" and not on syntax tutorials,
which other sites already cover well.

- **Checks:** run a hidden test suite plus real behaviour, such as "the service stays up
  under load" or "memory no longer grows". Keep tests outside `/lab` so they can't be
  edited by accident.
- **Toolchains:** run each language in a container on the Docker VM, pulled during
  bootstrap, so no new VM is needed.
- **Editing:** learners only have `vim` or `nano` inside the VM. Larger exercises need
  `/lab` shared with the host so they can use their own editor.
- **Mixed-skill labs:** "fix the bug, rebuild the image, roll it out to the cluster"
  combines a language with the Docker and Kubernetes content.

### Networking & IP routing notes

The foundation for the networking ideas: the degraded networks and VPN tracks build on
it.

| Module | Exercises |
|---|---|
| **Addressing & subnets** | CIDR, wrong netmasks, overlapping ranges, multiple addresses, default gateways |
| **Layer 2** | ARP, Linux bridges, VLANs, bonding, duplicate IPs, broadcast problems |
| **Static routing** | Multi-hop paths, forwarding, return-path failures, longest-prefix match, asymmetric routes |
| **Policy routing** | Multiple routing tables, `ip rule`, two uplinks, source-based routing, `rp_filter` drops |
| **NAT & firewalls** | nftables, SNAT and DNAT, port forwards, conntrack, hairpin NAT |
| **Network services** | DHCP with dnsmasq, a basic DNS intro (the DNS & email track goes deeper) |
| **IPv6** | SLAAC, link-local addresses, neighbor discovery, dual-stack bugs |
| **OSPF** | FRR neighbors, areas, costs, timer mismatches, failover when a link dies |
| **BGP** | Peering, route filters and communities, path selection, accidental route leaks |
| **High availability** | VRRP with keepalived, ECMP, anycast services |
| **Packet analysis** | `tcpdump` and `tshark`, the TCP handshake, MTU problems, finding faults from captures |
| **Kubernetes** | Peer Calico with an FRR router over BGP, MetalLB in BGP mode, trace a packet from pod to outside |

Things to get right:

- **Topologies:** [containerlab](https://containerlab.dev) describes a network of
  containers in YAML. Not yet tried on the lab VMs, so test it on the 3GiB Docker VM.
  FRR routers are small, so 6–8 should fit.
- **Router software:** FRR is open source and its CLI is close to Cisco's, so no licensed
  vendor images are needed.
- **Checks:** ping and `traceroute` for the real path, plus `vtysh` to inspect OSPF and
  BGP state. Those protocols take time to converge, so poll with a timeout.
- **Docker gotcha:** Docker's own firewall rules block traffic between its bridge
  networks. Connect routers with direct veth links, as containerlab does.
- **Containment:** keep everything inside containers, so a bad route can't cut off the
  VM's SSH.
- **Reset:** tearing down the topology clears nearly everything.

### DNS & email notes

Build DNS first, because email depends on it: MX records, SPF, DKIM and DMARC are all
DNS. A whole private internet can run in containers, with its own root, TLD and domains,
and several organisations sending mail to each other.

DNS modules:

| Module | Exercises |
|---|---|
| **Tools & resolution** | `dig +trace`, record types, following a query from the root down |
| **Authoritative servers** | BIND zones, SOA serials, reverse zones, syntax errors, forgotten serial bumps |
| **Delegation** | Build root → TLD → domain, glue records, lame delegations |
| **Replication** | Primary and secondary servers, AXFR and IXFR, TSIG keys, stale secondaries |
| **Resolvers & caching** | Unbound, forwarding, TTLs, negative caching, split-horizon |
| **DNSSEC** | Sign a zone, DS records, key rollover, debugging a broken chain |
| **Encrypted & Kubernetes DNS** | DNS over TLS, CoreDNS plugins and stub domains, ExternalDNS |

Email modules:

| Module | Exercises |
|---|---|
| **Postfix basics** | Send and receive, MX lookup, aliases, virtual domains |
| **Mailboxes** | Dovecot IMAP, Maildir, quotas, connecting Postfix to Dovecot |
| **Submission & TLS** | SASL login, STARTTLS, certificates, port 25 vs 587 |
| **Authentication** | SPF, DKIM signing, DMARC policies, reading `Authentication-Results` headers |
| **Fixing mail flow** | Stuck queues, deferrals, bounces, mail loops, broken reverse DNS |
| **Spam & abuse** | Close an open relay, rspamd rules, greylisting, rate limits |
| **Incidents** | Combined failures, such as a DNS change that quietly breaks DKIM for one domain |

Things to get right:

- **Isolation:** use reserved names such as `.test`, and block outbound port 25 and
  external DNS from the lab network, so a misconfiguration can never send real mail.
- **Checks:** `dig` answers for DNS; for email, send with `swaks`, then inspect the
  delivered message and its headers.
- **Docker gotcha:** containers use Docker's built-in resolver by default, so point each
  one at the lab's own resolver.
- **Timing:** flush resolver caches in checks and resets. DNSSEC signatures depend on the
  VM's clock being right.
- **Memory:** Postfix, Dovecot and BIND are light. rspamd with Redis is heavier, so run
  it only in the spam module.
- **Offsec tie-ins:** an attack-then-defend pair covering zone transfer leaks and open
  relays stays within the local-only rules.

### Degraded networks notes

Start with the hunt-the-bad-link exercises, which are cheap and need only containers.

| Module | Exercises | Runs on |
|---|---|---|
| **Learn the tools** | Build `tc` and netem profiles, combine them with HTB, shape only one destination, shape ingress with `ifb` | Docker VM |
| **Hunt the bad link** | A chain of containers has one impaired hop: find it with `mtr`, `ss -ti`, `iperf3` and `tcpdump` retransmits | Docker VM |
| **Make apps resilient** | Add timeouts, retries with backoff and connection pools; tune nginx upstream timeouts until the service survives loss | Docker VM; ties into Python and Go |
| **TCP & transfers** | Window sizes on high-latency links, BBR vs CUBIC under loss, slow large transfers, HTTP/2 vs HTTP/3 | Docker VM |
| **Kubernetes under bad networks** | Probes flapping on latency, a slow node, DNS timeouts from UDP loss, one pod's traffic degraded via its veth | k8s lab |
| **Cluster control plane** | etcd leader elections under latency, heartbeat tuning, nodes going `NotReady` | k8s lab; needs care |
| **Databases over bad links** | Postgres replication lag, synchronous commits stalling, Redis replicas resyncing | Docker VM; shares database setup |

Things to get right:

- **Lockout risk:** netem on a VM's main interface also hits the SSH connection kubelab
  depends on. Rules on nodes should target only cluster traffic, or remove themselves
  after a timeout.
- **Reset:** netem rules outlive the exercise, so the reset hooks have to clear them on
  every interface.
- **Flaky checks:** loss is random, so use generous thresholds and several samples, or
  inspect `tc qdisc show` directly.
- **Kernel module:** Ubuntu cloud kernels may ship `sch_netem` only in
  `linux-modules-extra`, so bootstrap should install and test it. Not yet checked on the
  lab VMs.
- **Permissions:** containers applying their own rules need `NET_ADMIN`; applying them
  from the host side avoids giving learners that.

### VPN creation notes

Build WireGuard first, then OpenVPN (adds certificates), then IPsec (hardest, but still
the most common between companies).

| Module | Exercises |
|---|---|
| **WireGuard basics** | Generate keys, bring up a point-to-point tunnel, understand `AllowedIPs` and `wg-quick` |
| **Topologies** | Road warrior, site-to-site between two LANs, hub-and-spoke, full mesh |
| **Routing & DNS** | Split vs full tunnel, NAT and forwarding, pushing DNS, a firewall kill switch |
| **OpenVPN** | Build the CA and certificates, server and client config, revoking a client, TCP vs UDP |
| **IPsec** | strongSwan site-to-site, IKEv2, mismatched proposals, NAT traversal |
| **Mesh VPN** | Self-hosted Headscale with Tailscale clients, ACLs, relays when direct paths fail |
| **Troubleshooting** | No handshake, wrong `AllowedIPs`, routes missing on one side, MTU black holes, NAT timeouts, expired certificates |
| **Kubernetes** | Turn on Calico's WireGuard encryption, connect two sites, reach a cluster service over a tunnel |

Links to other items on this list:

- **Netem:** VPNs on lossy links, TCP-over-TCP slowdowns, and MTU problems.
- **TLS & PKI:** OpenVPN's certificate work could build on that track.
- **Offsec:** an attack-then-defend pair, such as pivoting through a compromised VPN
  client and then restricting it with firewall rules.

Things to get right:

- **Layout:** each "site" is a container or network namespace on the Docker VM, so
  several networks need no extra VMs.
- **Lockout risk:** a full tunnel on the VM itself would replace the default route and
  could cut off SSH, so keep tunnels inside containers or namespaces.
- **Checks:** ping through the tunnel, confirm the handshake with `wg show`, and capture
  the underlying link to prove the traffic is encrypted.
- **Permissions:** containers need `NET_ADMIN`, and OpenVPN also needs `/dev/net/tun`.
- **Kernel support:** WireGuard should be in the Ubuntu kernel, but bootstrap should test
  it. Not yet checked on the lab VMs.
- **Reset:** the hooks need to remove leftover namespaces, interfaces and routes.

## Offensive security modules

The track has 40 exercises so far, against a target of about 400.

- **Network and credentials:** enumeration, service misconfigurations, hash cracking,
  pivoting between containers.
- **Container and Kubernetes attacks:** container escapes, abusing over-broad RBAC,
  stolen service account tokens, and then fixing each one.
- **Advanced web:** SSTI, deserialization, race conditions, OAuth flaws, business
  logic bugs.

## New formats

These reuse the existing exercises.

- **Attack, then defend:** exploit a target, then patch it and prove the exploit no
  longer works.
- **On-call mode:** a random fault is injected into a healthy system with no scenario
  text, only an alert, and you're timed to fix it.
- **Multi-stage campaigns:** several linked steps on one lab, where each step's result
  unlocks the next.
- **Hard mode:** run any existing exercise with a degraded network (netem), so it's
  replayable without writing new content. On-call mode can also use netem as one of its
  random faults.

## kubelab features

- **Review mode:** brings solved exercises back after a few days or weeks, using the
  dates already in `progress.json`.
- **Weak spots:** shows which modules cost the most hints, failed checks and solution
  views. The data is already recorded.
- **Daily challenge:** one exercise a day, picked from what isn't solved yet, with a
  streak counter.
- **Exercise packs:** install extra exercises from a Git repo, building on the existing
  `KUBELAB_EXERCISES` setting.
- **Command golf:** records the shell session and compares the number of commands with
  the reference solution.
