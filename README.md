# awesome_labs

**Hands-on labs on your own machine** for Docker, Kubernetes, Linux, networking, cloud,
databases, security, programming, algorithms, data science, Git, Terraform and Ansible — real VMs,
real breakage, automatic grading.

awesome_labs creates [Lima](https://lima-vm.io) virtual machines, puts each exercise's
scenario in place (a broken deployment, a crashed daemon, a half-finished compose
project…), drops you into a shell, and checks your work when you're done. Every
exercise has a scenario, progressive hints and a full solution you can reveal, and most
have a short tutorial on the concepts and commands to read first.

Run `awesome_labs` and the labs open **in your web browser**: the catalogue, each
exercise's scenario, tutorial, hints and solution, live output from start and check, and
terminals into the lab VMs right on the page. Everything is also scriptable from the
command line (`awesome_labs start/check <id>`).

## What you get

| Track | Modules | Exercises |
|-------|---------|-----------|
| **Docker** | containers · images & Dockerfiles · volumes · networking · Compose · registries · security & limits · the daemon & host · production patterns · real-world troubleshooting · builds, runtime controls & Compose (BuildKit cache mounts, seccomp, cgroups, projected config) · automating Docker: the Engine API, filters & templates, scripts | <!-- count:docker -->186<!-- /count --> |
| **Kubernetes** | pods · deployments · services & DNS · ConfigMaps & Secrets · scheduling · storage · Jobs/CronJobs/DaemonSets · probes, resources & autoscaling · Ingress & Gateway API · NetworkPolicies · RBAC & pod security · app troubleshooting · cluster troubleshooting · cluster administration · Helm, Kustomize & CRDs · BGP with Calico and MetalLB · advanced workloads & policy (affinity, PDBs, StatefulSets, admission policy, egress) · failure policies & lifecycle controls (pod failure policies, per-index retries, scheduling and readiness gates, PVC retention) | <!-- count:kubernetes -->273<!-- /count --> |
| **Offensive Security** | web exploitation (recon · SQL injection · path traversal · command injection · IDOR · SSRF · XXE · JWT · remediation) · web & API attacks (SSRF, SSTI, XXE, mass assignment, JWT forgery, business logic) · privilege escalation · *(more modules in progress)* · applied cryptography attacks (XOR, ECB, CBC bit-flips, padding oracles, RSA, length extension) | <!-- count:offsec -->83<!-- /count --> |
| **Linux** | disks, systemd, shell, cgroups, namespaces, LVM · Git beyond the basics · container internals: a container by hand, overlay layers, `runc`, user namespaces, capabilities, PID 1 · files, services & recovery (attrs, sysctl, systemd units & limits, filesystem repair) *(growing)* · security & hardening (sudoers, systemd sandboxing, sshd, GPG, mount options, mutual TLS) | <!-- count:linux -->86<!-- /count --> |
| **Networking** | routing, NAT, firewalls, DNS, TLS, proxies, WireGuard, netem · OSPF, BGP, VRRP and IPv6 routing on FRR · paths, tunnels & TLS (subnetting, PMTU, GRE/VXLAN, bonding, certificate chains) *(growing)* · TLS & PKI (CSRs, SANs, SNI, trust stores, `openssl ca`, OCSP, name constraints, cross-signed roots) · TCP & sockets (connection states, TIME-WAIT, ephemeral ports, listen queues, Nagle, the bandwidth-delay product) | <!-- count:networking -->98<!-- /count --> |
| **Cloud** | AWS CLI, CloudFormation and IAM permission debugging against a local emulator · events, storage & governance (SQS/SNS/EventBridge, S3 policies, KMS, DynamoDB, RBAC) *(growing)* · managed databases: RDS & Aurora (backups, replicas, encryption, failover, IAM auth, DR snapshots) | <!-- count:cloud -->88<!-- /count --> |
| **Data** | SQL, PostgreSQL operations and Redis · point-in-time recovery, replication and failover, PgBouncer, Redis Sentinel and AOF repair · SQL in depth & Redis data structures (window functions, JSONB, RLS, sorted sets, geo, ACLs) *(growing)* · logic in the database (PL/pgSQL functions & triggers, SECURITY DEFINER, Redis Lua and Functions) | <!-- count:data -->78<!-- /count --> |
| **Defensive Security** | incident response and hardening · packet-capture forensics with tshark · AppArmor, seccomp, SSH certificates and WAF tuning · threat hunting & containment (log analysis, deobfuscation, persistence, IR) *(growing)* · web & TLS hardening (nginx, security headers, CSP, internal CAs, mutual TLS, revocation) | <!-- count:defensive -->78<!-- /count --> |
| **Programming** | Python and Go · common pitfalls & idioms (closures, decimals, slices, error wrapping, concurrency) *(growing)* · secure coding (path traversal, XSS, password hashing, ReDoS, SSRF, zip slip, webhook signatures) | <!-- count:programming -->86<!-- /count --> |
| **Data Science** | pandas, statistics and machine learning · analysis & model diagnostics (tidy data, joins, chi-square, calibration, importance) *(growing)* · customer & product analytics (funnels, cohorts, RFM, survival, attribution, churn). **Done in Jupyter notebooks** that the checks run | <!-- count:datascience -->83<!-- /count --> |
| **Git** | the basics · branching & merging · inspecting history · undoing changes · rebasing & rewriting · stashing & tagging · remotes & collaboration · recovery & detective work (reflog, bisect, fsck) · plumbing & power tools · real-world workflows · config & aliases · ignore rules & attributes · hooks · submodules & subtrees · advanced merging (strategies, rerere, merge-tree) · searching & comparing · large repositories (shallow, partial, sparse) · repository troubleshooting · identity, SSH signing & trailers · multi-problem incidents · revisions, refs & scripting (gitrevisions, for-each-ref, plumbing commits, grafts) · hosting & serving repositories (bare repos, mirrors, git daemon, receive protections, deploy hooks) | <!-- count:git -->255<!-- /count --> |
| **Terraform** | the basics (init, plan, apply, outputs, provider versions) · variables, expressions & meta-arguments (`count`, `for_each`, templates, validation) · working with state (moved, removed, import, workspaces, backends, recovery) · modules (local, Git-pinned, refactoring into modules) · real Docker infrastructure with the `kreuzwerker/docker` provider · functions, lifecycle & testing (dynamic blocks, `lifecycle`, `check`, `import`, `terraform test`) *(growing)* · Terraform in automation (saved plans, exit codes, JSON plan gates, state locks, mock-provider tests, pipelines) | <!-- count:terraform -->92<!-- /count --> |
| **Ansible** | inventories & ad-hoc commands · playbooks (handlers, loops, conditionals, idempotence) · templates, files & variables (Jinja2, precedence, Vault) · roles & reuse (role design, dependencies, Galaxy from Git, ansible-lint) · operations & incidents (check mode, validate, block/rescue, become, constructed inventories) against SSH-managed nodes · control flow & extending Ansible (tags, blockinfile, delegation, handlers, vault IDs, collections, custom filters/modules) *(growing)* · data structures & types (dicts, merging, JSON in and out, type errors) | <!-- count:ansible -->89<!-- /count --> |
| **Algorithms** | searching & sorting · data structures · graphs · dynamic programming · strings, greedy & maths — each exercise solvable in **Python or Go** *(growing)* · recursion & backtracking | <!-- count:algorithms -->70<!-- /count --> |

That's <!-- count:total -->1645<!-- /count --> exercises so far, with a target of 250 per track; ideas for what comes next are
in [docs/PENDING.md](docs/PENDING.md).

Every module goes **beginner → intermediate → advanced** (difficulty 1 = beginner … 5 =
expert), mixing **build** tasks ("create X" / "capture the flag") and **fix** tasks ("X is
broken — find out why" / "patch the vulnerability"), and ends with a **capstone** or a
multi-problem **incident**: one big, realistic lab (a ticket from a real team) that combines
the module's skills.

## Responsible use

The offensive-security exercises are for learning, and they are self-contained: every
target runs on the lab's own VM and nothing reaches the outside network. Only use these
techniques on systems you own or are explicitly authorized to test.

## The lab environments

| Environment | VMs | What's inside |
|-------------|-----|---------------|
| **Docker host** | `kl-docker` (2 CPU, 4 GiB) | Ubuntu 24.04, Docker Engine + Compose + Buildx, common images pre-pulled, networking and diagnostic tools, a Python data-science image, Terraform 1.16 with an offline provider mirror, Ansible (ansible-core 2.21) with SSH-managed node containers |
| **Kubernetes cluster** | `kl-cp` → `controlplane` (2 CPU, 3 GiB)<br>`kl-worker` → `node01` (2 CPU, 2.5 GiB) | kubeadm **v1.36**, containerd 2, Calico (enforces NetworkPolicies), metrics-server, local-path storage (default StorageClass), Traefik ingress + Gateway API (NodePorts 30080/30443), Helm |
| **Offensive security lab** | `kl-offsec` (2 CPU, 3 GiB) | Ubuntu 24.04, Docker (hosts the vulnerable targets), attacker tooling (nmap, ncat, hydra, john, …) and wordlists under `/usr/share/kl`. Attacker and targets are the same isolated box |

The cluster deliberately runs one minor release behind the newest Kubernetes, so the
upgrade exercise has something to upgrade to.

VMs are created **on first use** (5–15 minutes, mostly package installs and image
pulls). A freshly built environment is kept as a **saved copy**, so later starts clone it
in seconds instead of building again, and pulled images go into a local cache
(`~/.awesome_labs/cache/images/`) that is loaded into new VMs, which keeps rebuilds off
Docker Hub's pull limits (images come through `mirror.gcr.io` first). On its first launch the app offers to build each
environment right away instead, one question per environment, so later exercises start
in seconds; `awesome_labs env prepare [docker|k8s|offsec]` (or **Prepare** on the Environments & VMs
page) does the same at any time. Before every exercise awesome_labs **resets** the environment
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
- ~6 GiB of free RAM (one environment at a time; ~10 GiB with Docker and Kubernetes both up), ~40 GiB of disk
- Internet access (packages and images are downloaded on first start)

## Download

You can skip building: the [latest release](https://github.com/kpatr1981/awesome_labs/releases/latest)
(v0.21.0) has static binaries for Linux and macOS on x86-64 and ARM. On Linux x86-64:

```bash
curl -fLo awesome_labs https://github.com/kpatr1981/awesome_labs/releases/latest/download/awesome_labs-linux-amd64
chmod +x awesome_labs
./awesome_labs install
```

For another platform, swap the suffix for `linux-arm64`, `darwin-amd64` or `darwin-arm64`.
Check the download against `SHA256SUMS` from the same release. `awesome_labs version --check`
tells you when a newer release is out.

## Build & run

```bash
git clone https://github.com/kpatr1981/awesome_labs.git
cd awesome_labs
make build          # or: go build -o awesome_labs .
./awesome_labs install   # installs what's missing and puts awesome_labs on your PATH
awesome_labs             # opens the labs in your web browser
```

`make build` produces a single static binary with the exercises built in. `make release`
builds one for Linux and macOS on x86-64 and ARM into `dist/`, with checksums.

## Using awesome_labs

`awesome_labs` (the same as `awesome_labs web`) serves the labs as a web page on
`127.0.0.1:7681`, with **terminals into the lab VMs in the browser**. It prints a link and
opens it; the page and its terminals run until you press `ctrl+c` in the terminal you
started it from.

```bash
awesome_labs                              # http://127.0.0.1:7681, opens your browser
awesome_labs --addr 127.0.0.1:9000 --no-open
```

- **The list** on the left filters by text, track, progress and learning path, and groups
  each module Beginner → Intermediate → Advanced, then its **Capstone** or **Incident**
  (with a badge). Drag its right edge to widen it (double-click resets it). `/` focuses the
  filter, `j`/`k` move through the list and `1`–`6` switch tabs.
- **The exercise page** has the tabs Scenario, Learn, Hints, Solution, Output and Notes,
  and the buttons Start, Terminal, Check, Hint, Solution, End, Restart and Reset progress.
  A button is dimmed when it can't act yet and says why when pressed. Start, Check and the
  environment operations stream their output live into the Output tab and the status bar.
- **Terminals** open in a panel below the exercise, beside it (`⇆`) or maximized (`⤢`),
  one tab per VM (`controlplane` and `node01` for Kubernetes). Starting the lab opens one by
  itself. Each code block in the text has **Copy** and **▸ Terminal**, which pastes it into
  the open terminal for you to run with Enter. A shell that ended reconnects on Enter.
- **Notebooks:** the Data Science track is done in Jupyter notebooks. **📓 Notebook** opens
  JupyterLab in the same panel, on the exercise's notebook `/lab/<id>.ipynb`, which
  starting the exercise writes: how it's graded, the code to fix (if any) and a save cell
  for each file to produce. It runs on the docker host in `/lab`, as the lab user, with
  pandas, NumPy, SciPy, scikit-learn and matplotlib. **The notebook is what's graded:**
  Check runs it from top to bottom in a fresh kernel, on a copy of `/lab` without the
  files it must write, then grades what it wrote, and calls the functions the task names
  with the check's own data. Files made by hand don't count, and the check's hidden data
  is out of the notebook's reach while it runs. The container is capped at 2.5 GiB;
  closing the notebook tab stops it and resetting the lab removes it.
- **Continue where you left off:** when an exercise is still set up on the (stopped) VMs,
  the page opens with *Continue <id> — your lab is kept*; Continue boots the VMs and opens
  a terminal.
- **Notes and your commands:** the Notes tab is yours per exercise (kept in
  `~/.awesome_labs/notes.json`; resetting progress keeps it). Shells opened from
  awesome_labs keep a history per exercise, which each check collects; once you reveal the
  solution, the Solution tab lists your commands under it, to compare.
- **Exams** (top bar → Exam): pick a pool (a whole track or a learning path), the number of
  questions, a time limit and whether hints are allowed. awesome_labs draws a balanced
  set — about a third easy, a third medium, a third hard. A countdown runs, hints and
  solutions are locked, and **the last check of each question counts**. **Finish** (or the
  end of the time) scores you against a **66 % pass mark**, with your previous best on that
  pool; **Abandon** gives up and records it as abandoned. Exams are kept in
  `~/.awesome_labs/exam-history.json`.
- **Weak spots** (top bar): the modules where you needed the most hints, solutions and
  checks, and the exercises worth doing again without the solution.
- **Environments & VMs** shows every environment and VM, with start, stop, reset,
  prepare, rebuild and delete, how much disk each VM and saved copy really takes, and how
  long a VM has been stopped.
- **Settings** (⚙) edits `~/.awesome_labs/config.yaml` (see [Settings](#settings)), keeping
  its comments, and says when each setting applies.
- **First launch** offers to build the environments that have no saved copy yet, one
  checkbox each. Each is built, saved and stopped, so its first exercise starts in seconds.

### How a lab works

You work on **one exercise at a time**: starting one resets its environment to a pristine
snapshot, so it replaces whatever was set up before (the page asks first), and deletes the
VMs of every other environment, since that exercise doesn't need them. Leaving an
exercise's page **keeps its lab**: your files and containers stay in the VMs, and opening
it again takes you back where you left off. The lab is cleaned when you **End** the
exercise (the environment is reset in place and left running, so the next exercise starts
without building or booting anything), when you start another one, or when its environment
is reset, rebuilt or deleted. An exercise that breaks its environment beyond a reset has
its VMs deleted instead. During an exam the VMs keep running, since they carry on to the
next question.

Inside the VMs the scenario is also in **`/lab/TASK.md`**; `/lab` is your working
directory. On the control plane `kubectl` (alias `k`, with completion) is configured for
you, and `ssh node01` gets you onto the worker. While an environment builds or boots, the
status bar shows the step it's on, the time so far and, from the last run of the same kind,
an estimate.

**Only what's needed runs.** With `auto_stop: true` (the default) the server stops the
running lab VMs once no exercise is set up and nothing (an operation, a terminal, a
notebook) has used them for 10 minutes, or when an exercise is set up but no page has been
open for 30 minutes. Stopping keeps the disks and the lab: the exercise's page boots it
again. Solving an exercise with `awesome_labs check` stops its VMs too.

**Stopping the server.** `ctrl+c` handles the lab VMs by `on_exit`: by default it
**stops** them, freeing their memory and keeping their disks and the exercise you were on;
`keep` leaves them running and `delete` deletes them. A second `ctrl+c` leaves them as they
are, and so does stopping while an operation runs (which cancels it) or failing to start
(say, the port is taken).

**Security.** A terminal in a web page is a shell on your lab VMs, so the server listens on
`127.0.0.1` only, and every request needs the random token in the printed link. The token
is kept in a same-site cookie after the first visit and is new each run. Requests from
other sites and other `Host` names are refused. `--addr 0.0.0.0:7681` makes it reachable
from your network: anyone with the link then gets those shells, so only do that on a
network you trust.

### In VS Code

`awesome_labs code [docker|k8s|node01|offsec]` (it defaults to the exercise you're on)
opens the lab in **your own VS Code**. Nothing is installed in the VMs: no VS Code server,
no extension, no mount.

- **Files:** the workspace folder `~/.awesome_labs/vscode/<lab>/` mirrors the VM's `/lab`
  both ways while the window is open. A task in the workspace, *awesome_labs: sync /lab*,
  runs `awesome_labs sync` over the VM's existing SSH connection. VS Code starts it when
  the window opens; allow automatic tasks if it asks. Saving a file writes it into the VM
  within a second, and anything created or changed in the VM shows up in the editor.
  Files are written into the VM in place, so a file bind-mounted into a container keeps
  working. New files belong to the lab user. `.git` folders, symlinks and files over
  16 MiB are not mirrored.
- **Terminal:** the workspace's terminal opens a shell in the VM (`awesome_labs shell`),
  with a node01 profile in the Kubernetes lab. `shell`, `code` and `sync` run beside the
  web server.
- **The VM wins:** when the sync (re)connects, and when a file changed on both sides, the
  VM's version is kept. A reset or a new exercise empties the folder along with `/lab`.
  Any of your edits that the VM never got are copied to `~/.awesome_labs/vscode/backups/`
  first.

Closing the window stops the sync, and so do stopping the server and deleting all VMs.
`code` must be on your PATH (in VS Code: *Shell Command: Install 'code' command in
PATH*); on macOS the app in `/Applications` is found without it. Set
`AWESOME_LABS_CODE=codium` (or `cursor`, …) to use another editor of the same family.
Files outside `/lab` (`/etc`, the cluster's manifests…) are still edited from the terminal.

### Learning paths

| Path | What's in it |
|------|--------------|
| Docker fundamentals | containers, images, volumes, networks, Compose (difficulty ≤ 3) |
| Docker in production | registries, hardening, the daemon, production patterns, outages |
| Kubernetes for developers (CKAD-style) | workloads, config, services, probes, ingress, policies, Helm/Kustomize |
| Kubernetes administration (CKA-style) | scheduling, storage, networking, RBAC, cluster troubleshooting, upgrades, etcd |
| Kubernetes security | RBAC, pod security, network policies, secrets, audit, encryption |
| Web exploitation | injection, access control, SSRF, XXE and more against vulnerable web apps, then the patch |
| Terraform fundamentals | plan & apply, variables and expressions, state, modules, Docker infrastructure (difficulty ≤ 3) |
| Ansible fundamentals | inventories, playbooks, templates, variables, Vault and roles (difficulty ≤ 3) |
| Troubleshooting only | every *fix* exercise |
| Incident drills | the capstones and incidents at the end of each module |

Paths are defined in `learning-paths.yaml` (filters by track, module, kind, tag,
difficulty or id).

### Settings

Settings live in `~/.awesome_labs/config.yaml`; the Settings page edits them.

```yaml
on_exit: stop                  # stop · delete · keep — what ctrl+c does to the lab VMs
auto_stop: true                # stop idle lab VMs (see "Only what's needed runs")
delete_stopped_after_days: 0   # delete lab VMs stopped this many days (0 = never)
docker:       {cpus: 2, memory: 4GiB}   # applied when a VM is (re)created:
controlplane: {cpus: 2, memory: 3GiB}   #   awesome_labs env rebuild docker|k8s
worker:       {cpus: 2, memory: 2560MiB}
```

Changes apply at once, `on_exit` included. With
`delete_stopped_after_days: N` the server deletes lab VMs stopped for more than N days, at
start and once a day, freeing their disk; saved copies are kept, so the next exercise still
starts in seconds.

### Command line

```bash
awesome_labs [web] [--addr HOST:PORT] [--no-open]   # the labs in your browser (the default)
awesome_labs install [--dry-run] [--yes] [--bin-dir DIR]   # set up Lima, QEMU and the binary
awesome_labs status                     # the exercise you're working on + running VMs
awesome_labs list [filter]              # list exercises (✔ solved, ● started)
awesome_labs paths [path-id]            # learning paths and progress
awesome_labs show k8s-101 [--solution]
awesome_labs start docker-042           # prepare the lab
awesome_labs check docker-042           # grade (exit code 0 = solved)
awesome_labs forget docker-042          # erase progress on one exercise
awesome_labs shell docker|k8s|node01|offsec    # open a shell
awesome_labs code [docker|k8s|node01|offsec]   # open the lab's /lab in VS Code
awesome_labs exam status|history|abandon       # the running exam, past results, give up
awesome_labs env status|up|stop|reset|rebuild|destroy [docker|k8s|offsec]
awesome_labs env prepare [docker|k8s|offsec]   # build now and keep a saved copy
awesome_labs env cache [docker|k8s|offsec]     # fill the local image cache (no env: show it)
awesome_labs vms                        # every lab VM with status, CPU and memory
awesome_labs vms stop|delete <name|all> # e.g. vms delete kl-offsec · vms stop all
awesome_labs doctor [--fix]             # check this machine for the usual trouble
awesome_labs version [--check]          # this build's version; --check: is a newer release out?
```

Only **one awesome_labs runs at a time**: the web server and every command that changes
the VMs or your progress take a lock on `~/.awesome_labs/app.lock`, and a second one exits
with *another awesome_labs is already running*. The lock goes away with the process, even
after a crash. The read-only `list`, `show`, `status`, `paths` and `lint` still run beside
it, and so do `shell`, `code` and `sync`, which only work inside a running VM.

Progress is stored in `~/.awesome_labs/progress.json`, logs in `~/.awesome_labs/logs/`.
When something seems stuck or broken, `awesome_labs doctor` is the first thing to run:
it checks Lima (and QEMU on Linux), free disk and RAM, whether the `awesome_labs` on your
PATH is a different build from the one you're running, environments left marked busy by
a crashed run (`--fix` clears them), stray VMs and missing saved copies, and says how to
fix each one.

A terminal UI is still included (`awesome_labs tui`; `F1` lists its keys) for working
without a browser.

## Writing exercises

Exercises are YAML files in `exercises/<track>/NN-<module>.yaml`, embedded into the
binary at build time. See **[docs/AUTHORING.md](docs/AUTHORING.md)**. Every exercise
ships with a `solve` script.

```bash
awesome_labs lint                    # validate every exercise file
awesome_labs verify [-from id] [-record verified.json] [id-prefix ...]
awesome_labs verified                # the release gate: exercises not verified in their current form
awesome_labs counts                  # rewrite the exercise counts in this README and docs/PENDING.md
awesome_labs notebook ds-001         # print the starter notebook a notebook exercise writes
```

`verify` runs each one end to end — *reset → setup → check must fail → solve → check must
pass* — which is how the whole catalogue is tested. Each exercise gets 25 minutes (50 for the
destructive ones). Like the app, a verify run needs to be the only awesome_labs running. Set `AWESOME_LABS_EXERCISES=./exercises`
to work on exercise files without rebuilding. With `-record`, each pass is noted in
`verified.json`, and `make release` refuses to build while an exercise is new or changed
since its last pass (`make release FORCE=1` overrides it); CI runs the tests, `lint` and the
count check on every push. See [docs/AUTHORING.md](docs/AUTHORING.md#verifying-and-releasing).

## Project layout

```text
main.go                    CLI entry point, verify runner
verified.go                the verified/counts author commands
verified.json              which exercises passed verify in their current form (the release gate)
internal/exercise          exercise model + loader/validator
internal/lab               environments: create/bootstrap/reset VMs, run setup/check/solve, VS Code sync
internal/lima              thin limactl wrapper
internal/install           OS detection and the install command
internal/tui               Bubble Tea interface
internal/web               the web interface (awesome_labs web): HTTP API, web terminals, static/ page
internal/progress          ~/.awesome_labs/progress.json and notes.json
internal/doctor            awesome_labs doctor
internal/update            the newer-release check
internal/exam              timed exams
internal/config            ~/.awesome_labs/config.yaml
internal/verified          exercise hashes, the verified record, the docs' count markers
learning-paths.yaml        curated paths
vms/*.yaml                 Lima templates
vms/bootstrap/             one-time provisioning (kubeadm init, add-ons, image pre-pulls)
vms/kit/                   copied to /opt/kubelab in every VM: lib.sh helpers, reset & snapshot scripts
exercises/                 the exercises, one folder per track
.github/workflows/         CI (tests, lint, counts) and the experimental VM verify
```

## License

[MIT](LICENSE)
