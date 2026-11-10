# awesome_labs

**Hands-on labs on your own machine** for Docker, Kubernetes, Linux, networking, cloud,
databases, security, programming and data science — real VMs, real breakage, automatic grading.

awesome_labs creates [Lima](https://lima-vm.io) virtual machines, puts each exercise's
scenario in place (a broken deployment, a crashed daemon, a half-finished compose
project…), drops you into a shell, and checks your work when you're done. Every
exercise has a scenario, progressive hints and a full solution you can reveal, and most
have a short tutorial on the concepts and commands to read first.

```text
 awesome_labs  Program  View  Tracks  Help                        ✔ 37/726 solved  ● 2 started, unsolved
╭──────────────────────────────────────────────╮╭───────────────────────────────────────────────╮
│▸ Kubernetes · Troubleshooting the cluster     ││k8s-245 — The API server is gone               │
│ ✔ k8s-241   ●●○○○ fix   node01 is NotReady    ││●●●○○ Medium · Fix something · Kubernetes      │
│ ● k8s-242   ●●○○○ fix   The container runtime…││                                               │
│   k8s-245   ●●●○○ fix   The API server is gone││  kubectl get pods → The connection to the     │
│   ...                                         ││  server controlplane:6443 was refused …       │
╰──────────────────────────────────────────────╯╰───────────────────────────────────────────────╯
 ↑↓ move · enter open · / filter · tab next track · F10 menu · F1 keys · q quit
```

## What you get

| Track | Modules | Exercises |
|-------|---------|-----------|
| **Docker** | containers · images & Dockerfiles · volumes · networking · Compose · registries · security & limits · the daemon & host · production patterns · real-world troubleshooting | 149 |
| **Kubernetes** | pods · deployments · services & DNS · ConfigMaps & Secrets · scheduling · storage · Jobs/CronJobs/DaemonSets · probes, resources & autoscaling · Ingress & Gateway API · NetworkPolicies · RBAC & pod security · app troubleshooting · cluster troubleshooting · cluster administration · Helm, Kustomize & CRDs · BGP with Calico and MetalLB | 234 |
| **Offensive Security** | web exploitation (recon · SQL injection · path traversal · command injection · IDOR · SSRF · XXE · JWT · remediation) · privilege escalation · *(more modules in progress)* | 45 |
| **Linux** | disks, systemd, shell, cgroups, namespaces, LVM · Git beyond the basics · container internals: a container by hand, overlay layers, `runc`, user namespaces, capabilities, PID 1 *(growing)* | 44 |
| **Networking** | routing, NAT, firewalls, DNS, TLS, proxies, WireGuard, netem · OSPF, BGP, VRRP and IPv6 routing on FRR *(growing)* | 42 |
| **Cloud** | AWS CLI, CloudFormation and IAM permission debugging against a local emulator *(growing)* | 47 |
| **Data** | SQL, PostgreSQL operations and Redis · point-in-time recovery, replication and failover, PgBouncer, Redis Sentinel and AOF repair *(growing)* | 37 |
| **Defensive Security** | incident response and hardening · packet-capture forensics with tshark · AppArmor, seccomp, SSH certificates and WAF tuning *(growing)* | 40 |
| **Programming** | Python and Go *(growing)* | 45 |
| **Data Science** | pandas, statistics and machine learning *(growing)* | 43 |

That's 726 exercises so far, with a target of 250 per track; ideas for what comes next are
in [docs/PENDING.md](docs/PENDING.md).

Difficulty rises through each module (1 = beginner … 5 = expert), mixing **build** tasks
("create X" / "capture the flag") and **fix** tasks ("X is broken — find out why" /
"patch the vulnerability"). The Docker and Kubernetes modules end with a multi-problem
*incident*.

## Responsible use

The offensive-security exercises are for learning, and they are self-contained: every
target runs on the lab's own VM and nothing reaches the outside network. Only use these
techniques on systems you own or are explicitly authorized to test.

## The lab environments

| Environment | VMs | What's inside |
|-------------|-----|---------------|
| **Docker host** | `kl-docker` (2 CPU, 3 GiB) | Ubuntu 24.04, Docker Engine + Compose + Buildx, common images pre-pulled, networking and diagnostic tools, a Python data-science image |
| **Kubernetes cluster** | `kl-cp` → `controlplane` (2 CPU, 3 GiB)<br>`kl-worker` → `node01` (2 CPU, 2.5 GiB) | kubeadm **v1.36**, containerd 2, Calico (enforces NetworkPolicies), metrics-server, local-path storage (default StorageClass), Traefik ingress + Gateway API (NodePorts 30080/30443), Helm |
| **Offensive security lab** | `kl-offsec` (2 CPU, 3 GiB) | Ubuntu 24.04, Docker (hosts the vulnerable targets), attacker tooling (nmap, ncat, hydra, john, …) and wordlists under `/usr/share/kl`. Attacker and targets are the same isolated box |

The cluster deliberately runs one minor release behind the newest Kubernetes, so the
upgrade exercise has something to upgrade to.

VMs are created **on first use** (5–15 minutes, mostly package installs and image
pulls) and reused afterwards. Before every exercise awesome_labs **resets** the environment
to a pristine snapshot: it removes leftover containers, namespaces and cluster-scoped
objects, restores changed config files (daemon.json, kubelet config, static pod
manifests, …) and restarts what needs restarting — usually in a few seconds.

Two exercises (the cluster upgrade and encryption-at-rest) change the cluster in ways a
reset can't undo; awesome_labs rebuilds the cluster before your next Kubernetes exercise.

## Requirements

The labs are tested on Fedora 45 (x86_64) on a machine with 16 GB of RAM. The installer is
tested on Ubuntu 24.04, Fedora 45 and Arch Linux. macOS is supported and
confirmed working.

`awesome_labs install` detects macOS or Linux and sets up what's missing: Lima (Homebrew
on macOS when available, otherwise the official release, checksum-verified, into
`~/.local`), QEMU with its firmware on Ubuntu, Debian, Fedora, Arch or openSUSE (through
their package manager, which asks for sudo), and the binary itself in `~/.local/bin`. It shows the plan and asks first; `--dry-run` only
shows the plan.

- macOS (Apple Silicon or Intel) or Linux with [Lima](https://lima-vm.io) ≥ 2.0
  (`awesome_labs install` sets this up)
  - On Linux, awesome_labs runs QEMU through a small generated wrapper in `~/.awesome_labs/qemu`
    to avoid a [QEMU crash on Lima's `user-v2` network](https://github.com/lima-vm/lima/issues/5722).
    Starting the `kl-cp` or `kl-worker` VMs with `limactl` directly skips it.
- Go ≥ 1.27 to build
- ~6 GiB of free RAM (one environment at a time; ~9 GiB with both), ~40 GiB of disk
- Internet access (packages and images are downloaded on first start)

## Download

On Linux x86-64 you can skip building:

```bash
curl -fLo awesome_labs https://github.com/kpatr1981/awesome_labs/releases/latest/download/awesome_labs-linux-amd64
chmod +x awesome_labs
./awesome_labs install
```

## Build & run

```bash
git clone https://github.com/kpatr1981/awesome_labs.git
cd awesome_labs
make build          # or: go build -o awesome_labs .
./awesome_labs install   # installs what's missing and puts awesome_labs on your PATH
awesome_labs             # interactive browser
```

`make build` produces a single static binary with the exercises built in. `make release`
builds one for Linux and macOS on x86-64 and ARM into `dist/`, with checksums.

### Menus and the mouse

Away from the exercise screen, everything is also reachable from the **menu bar** at the top: click a menu, press
`F10`, or `alt` + the underlined letter (`alt+p` Program, `alt+v` View,
`alt+t` Tracks, `alt+h` Help). The environments & VMs screen opens with `e` and the exam
with `X` on the exercise list. Menu items show their
keyboard shortcut only on the screens where pressing that key does the same thing, so a
shown key works with the menu open or closed. `a` (active exercise), `S` (started labs),
`e` (environments & VMs) and `Q` (quit) work on every screen.

Menus only offer what applies to the current screen. What you do with an exercise lives
on the exercise screen itself, as the buttons along its tab row (Start, Terminal, Check,
Hint, Solution, Reset, Back). A button is lit only when pressing it can act now and is
dimmed otherwise, saying why when you press it: Terminal and Check need the lab set up,
and only the active exercise has it (every start resets the environment, in an exam too);
nothing that touches the lab runs while another operation is busy; Hint needs hints and
Reset needs progress to reset; hints, the solution and resetting are locked during an
exam. The Started labs and environments screens dim their buttons the same way.

The exercise screen itself has no menus: its buttons and tabs do everything there, and the
keys that work on every screen still do. Elsewhere, **File** moves between the screens.
**View** and **Tracks** shape the list: View also holds learning paths. **VMs** holds everything about the lab
environments. **View → Show** filters the list by progress: all, solved, in progress
(started, not solved) or no progress. **Tracks** shows how many exercises of each track you
have solved. A greyed-out item says why when you pick it.

The mouse works throughout: pick a track from the **Tracks** menu, click a row to select
it and double-click to open it, click the tabs and the buttons (Start, Terminal, Check,
Hint, Solution…) on the exercise screen, and scroll with the wheel. Each button shows its
key next to its name (on a narrow terminal, just the name). Confirmations open
as a dialog with **Yes** / **No** buttons (`y`/`n` still work). `F1` lists every
shortcut. While the mouse is in use, most terminals select text with `shift`+drag.

### In the browser

| Key | Action |
|-----|--------|
| `↑↓` / `j k` | move · `n`/`p` next/previous module |
| `/` | filter by id, title, tag, module or difficulty |
| `tab` | switch track |
| `u` | cycle the progress filter: all · solved · in progress (started, not solved) · no progress |
| `enter` | open an exercise |
| `L` | **learning paths** — focus the list on a curated path, with progress |
| `X` | **exam mode** — start a timed exam, or return to the running one |
| `a` | jump to the exercise you're **working on** (shown as `▶` in the header and list) |
| `S` | your labs: the active exercise, then others started earlier and not solved |
| `e` | environments & VMs: live state of every VM; create, stop, reset, rebuild, delete — `X` stops all, `K` deletes all |
| `Q` (`q` on the list, `ctrl+c`, Program → Exit) | exit — always **deletes every lab VM**, running or stopped, first, and starting the app deletes any left over (`ctrl+c` while busy or deleting exits at once and leaves them as they are) |

### In an exercise

| Key | Action |
|-----|--------|
| `s` | **start** — prepare the lab (creates the VMs the first time) |
| `t` | open a **terminal** in the docker host / control plane (`exit` to come back) |
| `w` | open a terminal on **node01** (Kubernetes only) |
| `c` | **check** your work |
| `h` | reveal the next **hint** |
| `v` | reveal the **solution** (asks first) |
| `R` | **reset progress** on this exercise (asks first) |
| `1`–`5` / `tab` / `←→` | Scenario · Learn · Hints · Solution · Output |
| `n` / `N` | next / previous in the list you opened it from: the filtered list, Started labs, or the exam's questions |
| `esc` / `q` | back to where you opened it from |

Leaving the exercise that is set up on the VMs, by any route except the environments
screen, always ends it and **cleans its VMs**: the environment is reset to pristine in
place and stays running, so the next exercise on it starts without building or booting
anything (and skips its own reset). An exercise that breaks its environment beyond a
reset has its VMs deleted instead. Starting an exercise **deletes the VMs of every
other environment**, running or stopped, since that exercise doesn't need them. During
an exam the VMs keep running, since they carry on to the next question. The environments screen goes
back to the exercise you came from.

Inside the VMs the scenario is also in **`/lab/TASK.md`**; `/lab` is your working
directory. On the control plane `kubectl` (alias `k`, with completion) is configured
for you, and `ssh node01` gets you onto the worker.

### Learning paths

| Path | What's in it |
|------|--------------|
| Docker fundamentals | containers, images, volumes, networks, Compose (difficulty ≤ 3) |
| Docker in production | registries, hardening, the daemon, production patterns, outages |
| Kubernetes for developers (CKAD-style) | workloads, config, services, probes, ingress, policies, Helm/Kustomize |
| Kubernetes administration (CKA-style) | scheduling, storage, networking, RBAC, cluster troubleshooting, upgrades, etcd |
| Kubernetes security | RBAC, pod security, network policies, secrets, audit, encryption |
| Troubleshooting only | every *fix* exercise |
| Incident drills | the multi-problem incidents at the end of each module |

Paths are defined in `learning-paths.yaml` (filters by track, module, kind, tag,
difficulty or id).

### Exam mode

Press `X`, pick a pool (a whole track or any path), the number of questions, a time
limit and whether hints are allowed. awesome_labs draws a balanced random set: about a
third easy, a third medium, a third hard. A countdown runs in the header, hints and
solutions are locked, and **the last check of each question counts**. When you finish
(`F`) or time runs out you get your score against a **66 % pass mark**, and solutions
unlock. Exams are kept in `~/.awesome_labs/exam-history.json`.

```bash
awesome_labs exam status     # the running exam
awesome_labs exam history    # past results
```

### Saving memory

Only one environment runs at a time: starting a Docker exercise deletes the Kubernetes
VMs and vice versa, while the environment you're using stays up and is cleaned between
exercises. Exiting always deletes every lab VM, running or stopped, and starting the app
deletes any left over. With `auto_stop: true` (the default), solving an exercise with
`awesome_labs check` stops its VMs. Settings live in `~/.awesome_labs/config.yaml`:

```yaml
auto_stop: true
docker:       {cpus: 2, memory: 3GiB}       # applied when a VM is (re)created:
controlplane: {cpus: 2, memory: 3GiB}       #   awesome_labs env rebuild docker|k8s
worker:       {cpus: 2, memory: 2560MiB}
```

### Command line

```bash
awesome_labs install [--dry-run] [--yes] [--bin-dir DIR]   # set up Lima, QEMU and the binary
awesome_labs status                     # the exercise you're working on + running VMs
awesome_labs list [filter]              # list exercises (✔ solved, ● started)
awesome_labs paths [path-id]            # learning paths and progress
awesome_labs show k8s-101 [--solution]
awesome_labs start docker-042           # prepare the lab
awesome_labs shell docker|k8s|node01|offsec    # open a shell
awesome_labs check docker-042           # grade (exit code 0 = solved)
awesome_labs forget docker-042          # erase progress on one exercise
awesome_labs env status|up|stop|reset|rebuild|destroy [docker|k8s|offsec]
awesome_labs vms                        # every lab VM with status, CPU and memory
awesome_labs vms stop|delete <name|all> # e.g. vms delete kl-offsec · vms stop all
```

You work on **one exercise at a time**: starting one resets its environment, so it
replaces whatever was set up before (the UI asks first). It stops being active when
you solve it, or when its environment is reset, rebuilt or deleted.

Only **one awesome_labs runs at a time**: the app and every command that changes the
VMs or your progress take a lock on `~/.awesome_labs/app.lock`, and a second one exits with
*another awesome_labs is already running*. The lock goes away with the process, even after
a crash. The read-only `list`, `show`, `status`, `paths` and `lint` still run beside it.

Progress is stored in `~/.awesome_labs/progress.json`, logs in `~/.awesome_labs/logs/`.

## Writing exercises

Exercises are YAML files in `exercises/<track>/NN-<module>.yaml`, embedded into the
binary at build time. See **[docs/AUTHORING.md](docs/AUTHORING.md)**. Every exercise
ships with a `solve` script, and

```bash
awesome_labs verify [id-prefix ...]
```

runs each one end to end — *reset → setup → check must fail → solve → check must
pass* — which is how the whole catalogue is tested. Each exercise gets 25 minutes (50 for the
destructive ones). Like the app, a verify run needs to be the only awesome_labs running. Set `AWESOME_LABS_EXERCISES=./exercises`
to work on exercise files without rebuilding.

## Project layout

```text
main.go                    CLI entry point, verify runner
internal/exercise          exercise model + loader/validator
internal/lab               environments: create/bootstrap/reset VMs, run setup/check/solve
internal/lima              thin limactl wrapper
internal/install           OS detection and the install command
internal/tui               Bubble Tea interface
internal/progress          ~/.awesome_labs/progress.json
internal/exam              timed exams
internal/config            ~/.awesome_labs/config.yaml
learning-paths.yaml        curated paths
vms/*.yaml                 Lima templates
vms/bootstrap/             one-time provisioning (kubeadm init, add-ons, image pre-pulls)
vms/kit/                   copied to /opt/kubelab in every VM: lib.sh helpers, reset & snapshot scripts
exercises/                 the exercises, one folder per track
```

## License

[MIT](LICENSE)
