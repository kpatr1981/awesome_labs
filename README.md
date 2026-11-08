# kubelab

**500 hands-on Docker & Kubernetes exercises on your own machine** — real VMs, real
breakage, automatic grading.

kubelab creates [Lima](https://lima-vm.io) virtual machines, puts each exercise's
scenario in place (a broken deployment, a crashed daemon, a half-finished compose
project…), drops you into a shell, and checks your work when you're done. Every
exercise has a scenario, progressive hints and a full solution you can reveal.

```text
 kubelab Docker & Kubernetes labs                                ✔ 37/500 solved  ● 2 in progress
 All  Docker  Kubernetes
╭──────────────────────────────────────────────╮╭───────────────────────────────────────────────╮
│▸ Kubernetes · Troubleshooting the cluster     ││k8s-245 — The API server is gone               │
│ ✔ k8s-241   ●●○○○ fix   node01 is NotReady    ││●●●○○ Medium · Fix something · Kubernetes      │
│ ● k8s-242   ●●○○○ fix   The container runtime…││                                               │
│   k8s-245   ●●●○○ fix   The API server is gone││  kubectl get pods → The connection to the     │
│   ...                                         ││  server controlplane:6443 was refused …       │
╰──────────────────────────────────────────────╯╰───────────────────────────────────────────────╯
 ↑↓ move · enter open · / filter · tab track · n/p module · u unsolved · e environments · q quit
```

## What you get

| Track | Modules | Exercises |
|-------|---------|-----------|
| **Docker** | containers · images & Dockerfiles · volumes · networking · Compose · registries · security & limits · the daemon & host · production patterns · real-world troubleshooting | 200 |
| **Kubernetes** | pods · deployments · services & DNS · ConfigMaps & Secrets · scheduling · storage · Jobs/CronJobs/DaemonSets · probes, resources & autoscaling · Ingress & Gateway API · NetworkPolicies · RBAC & pod security · app troubleshooting · cluster troubleshooting · cluster administration · Helm, Kustomize & CRDs | 300 |

Each module has 20 exercises of increasing difficulty (1 = beginner … 5 = expert),
mixing **build** tasks ("create X") and **fix** tasks ("X is broken, find out why").
Each module ends with a multi-problem *incident*.

## The lab environments

| Environment | VMs | What's inside |
|-------------|-----|---------------|
| **Docker host** | `kl-docker` (2 CPU, 3 GiB) | Ubuntu 24.04, Docker Engine + Compose + Buildx, common images pre-pulled |
| **Kubernetes cluster** | `kl-cp` → `controlplane` (2 CPU, 3 GiB)<br>`kl-worker` → `node01` (2 CPU, 2.5 GiB) | kubeadm **v1.36**, containerd 2, Calico (enforces NetworkPolicies), metrics-server, local-path storage (default StorageClass), Traefik ingress + Gateway API (NodePorts 30080/30443), Helm |

The cluster deliberately runs one minor release behind the newest Kubernetes, so the
upgrade exercise has something to upgrade to.

VMs are created **on first use** (5–15 minutes, mostly package installs and image
pulls) and reused afterwards. Before every exercise kubelab **resets** the environment
to a pristine snapshot: it removes leftover containers, namespaces and cluster-scoped
objects, restores changed config files (daemon.json, kubelet config, static pod
manifests, …) and restarts what needs restarting — usually in a few seconds.

Two exercises (the cluster upgrade and encryption-at-rest) change the cluster in ways a
reset can't undo; kubelab rebuilds the cluster before your next Kubernetes exercise.

## Requirements

- macOS (Apple Silicon or Intel) or Linux with [Lima](https://lima-vm.io) ≥ 2.0
  (`brew install lima`)
- Go ≥ 1.24 to build
- ~6 GiB of free RAM (one environment at a time; ~9 GiB with both), ~40 GiB of disk
- Internet access (packages and images are downloaded on first start)

## Build & run

```bash
make build          # or: go build -o kubelab .
./kubelab           # interactive browser
```

### In the browser

| Key | Action |
|-----|--------|
| `↑↓` / `j k` | move · `n`/`p` next/previous module |
| `/` | filter by id, title, tag, module or difficulty |
| `tab` | All / Docker / Kubernetes |
| `u` | show only unsolved |
| `enter` | open an exercise |
| `L` | **learning paths** — focus the list on a curated path, with progress |
| `X` | **exam mode** — start a timed exam, or return to the running one |
| `e` | environments screen (create, stop, reset, rebuild, delete VMs) |
| `Q` | quit **and stop all lab VMs** |

### In an exercise

| Key | Action |
|-----|--------|
| `s` | **start** — prepare the lab (creates the VMs the first time) |
| `t` | open a **terminal** in the docker host / control plane (`exit` to come back) |
| `w` | open a terminal on **node01** (Kubernetes only) |
| `c` | **check** your work |
| `h` | reveal the next **hint** |
| `v` | reveal the **solution** (asks first) |
| `1`–`4` / `tab` | Scenario · Hints · Solution · Output |
| `n` / `N` | next / previous exercise |

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
limit and whether hints are allowed. kubelab draws a balanced random set: about a
third easy, a third medium, a third hard. A countdown runs in the header, hints and
solutions are locked, and **the last check of each question counts**. When you finish
(`F`) or time runs out you get your score against a **66 % pass mark**, and solutions
unlock. Exams are kept in `~/.kubelab/exam-history.json`.

```bash
kubelab exam status     # the running exam
kubelab exam history    # past results
```

### Saving memory

Only one environment needs to run at a time. With `auto_stop: true` (the default),
starting a Docker exercise stops the Kubernetes VMs and vice versa; `Q` in the browser
quits and stops everything. Settings live in `~/.kubelab/config.yaml`:

```yaml
auto_stop: true
docker:       {cpus: 2, memory: 3GiB}       # applied when a VM is (re)created:
controlplane: {cpus: 2, memory: 3GiB}       #   kubelab env rebuild docker|k8s
worker:       {cpus: 2, memory: 2560MiB}
```

### Command line

```bash
kubelab list [filter]              # list exercises (✔ solved, ● started)
kubelab paths [path-id]            # learning paths and progress
kubelab show k8s-101 [--solution]
kubelab start docker-042           # prepare the lab
kubelab shell docker|k8s|node01    # open a shell
kubelab check docker-042           # grade (exit code 0 = solved)
kubelab env status|up|stop|reset|rebuild|destroy [docker|k8s]
```

Progress is stored in `~/.kubelab/progress.json`, logs in `~/.kubelab/logs/`.

## Writing exercises

Exercises are YAML files in `exercises/<track>/NN-<module>.yaml`, embedded into the
binary at build time. See **[docs/AUTHORING.md](docs/AUTHORING.md)**. Every exercise
ships with a `solve` script, and

```bash
kubelab verify [id-prefix ...]
```

runs each one end to end — *reset → setup → check must fail → solve → check must
pass* — which is how the whole catalogue is tested. Set `KUBELAB_EXERCISES=./exercises`
to work on exercise files without rebuilding.

## Project layout

```text
main.go                    CLI entry point, verify runner
internal/exercise          exercise model + loader/validator
internal/lab               environments: create/bootstrap/reset VMs, run setup/check/solve
internal/lima              thin limactl wrapper
internal/tui               Bubble Tea interface
internal/progress          ~/.kubelab/progress.json
internal/exam              timed exams
internal/config            ~/.kubelab/config.yaml
learning-paths.yaml        curated paths
vms/*.yaml                 Lima templates
vms/bootstrap/             one-time provisioning (kubeadm init, add-ons, image pre-pulls)
vms/kit/                   copied to /opt/kubelab in every VM: lib.sh helpers, reset & snapshot scripts
exercises/                 the 500 exercises
```
