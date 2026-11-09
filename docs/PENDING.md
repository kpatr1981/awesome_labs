# Pending ideas

Ideas for future labs. Each topic has one home; where tracks touch, the row says which
track it relies on.

**Depth over count.** There is no exercise target for any topic. A lab should carry a real
multi-step scenario and teach a concept, and a smaller catalog after merging thin exercises
is a good outcome. The parts of this list that the existing exercises don't cover are not
started.

Exercise counts as of v0.5.0:

| Topic | Exercises | Track | Module files |
|---|---|---|---|
| Docker | 149 | `docker` | `exercises/docker/` |
| Kubernetes | 231 | `kubernetes` | `exercises/kubernetes/` |
| Offensive security | 70 | `offsec` | `exercises/offsec/` |
| Linux & systems | 45 | `linux` | `exercises/linux/` |
| Networking | 45 | `networking` | `exercises/networking/` |
| Cloud & platform | 45 | `cloud` | `exercises/cloud/` |
| Data | 45 | `data` | `exercises/data/` |
| Defensive security | 45 | `defensive` | `exercises/defensive/` |
| Programming | 45 | `programming` | `exercises/programming/` |
| Data science | 45 | `datascience` | `exercises/datascience/` |

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
| `/dev/kvm` inside the lab VMs | QEMU | Present in `kl-docker`; running a guest is still untested |
| containerlab on the 3GiB Docker VM | Networking and tracks built on it | Not tried; FRR routers are small, so 6–8 should fit |
| `sch_netem` kernel module | Degraded networks | Works in `kl-docker`, and `net-005` uses it |
| WireGuard kernel support | VPN creation | Works in `kl-docker`, and `net-015` uses it |
| Moto's optional IAM enforcement | AWS CLI | Not tested; needed before any permission-debugging exercises |

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
| **Git** | Merge and rebase conflicts, recovering lost commits with reflog, bisect, hooks, cleaning secrets out of history | Any VM; very easy to check |
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
| **IPv6** | SLAAC, link-local addresses, neighbor discovery, dual-stack bugs |
| **OSPF** | FRR neighbors, areas, costs, timer mismatches, failover when a link dies |
| **BGP** | Peering, route filters and communities, path selection, accidental route leaks |
| **High availability** | VRRP with keepalived, ECMP, anycast services |
| **Packet analysis** | `tcpdump` and `tshark`, the TCP handshake, MTU problems, finding faults from captures |
| **Kubernetes** | Peer Calico with an FRR router over BGP, MetalLB in BGP mode, trace a packet from pod to outside |

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

The track has 70 exercises so far.

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

## Left out for now

- **Service mesh:** Istio would strain the 3GiB and 2.5GiB nodes for relatively little
  payoff.
