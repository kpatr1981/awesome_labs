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
terminals into the lab VMs right on the page. A terminal UI (`awesome_labs tui`) and plain
commands (`awesome_labs start/check <id>`) are there too.

## What you get

| Track | Modules | Exercises |
|-------|---------|-----------|
| **Docker** | containers · images & Dockerfiles · volumes · networking · Compose · registries · security & limits · the daemon & host · production patterns · real-world troubleshooting · builds, runtime controls & Compose (BuildKit cache mounts, seccomp, cgroups, projected config) · automating Docker: the Engine API, filters & templates, scripts | 184 |
| **Kubernetes** | pods · deployments · services & DNS · ConfigMaps & Secrets · scheduling · storage · Jobs/CronJobs/DaemonSets · probes, resources & autoscaling · Ingress & Gateway API · NetworkPolicies · RBAC & pod security · app troubleshooting · cluster troubleshooting · cluster administration · Helm, Kustomize & CRDs · BGP with Calico and MetalLB · advanced workloads & policy (affinity, PDBs, StatefulSets, admission policy, egress) · failure policies & lifecycle controls (pod failure policies, per-index retries, scheduling and readiness gates, PVC retention) | 269 |
| **Offensive Security** | web exploitation (recon · SQL injection · path traversal · command injection · IDOR · SSRF · XXE · JWT · remediation) · web & API attacks (SSRF, SSTI, XXE, mass assignment, JWT forgery, business logic) · privilege escalation · *(more modules in progress)* · applied cryptography attacks (XOR, ECB, CBC bit-flips, padding oracles, RSA, length extension) | 80 |
| **Linux** | disks, systemd, shell, cgroups, namespaces, LVM · Git beyond the basics · container internals: a container by hand, overlay layers, `runc`, user namespaces, capabilities, PID 1 · files, services & recovery (attrs, sysctl, systemd units & limits, filesystem repair) *(growing)* · security & hardening (sudoers, systemd sandboxing, sshd, GPG, mount options, mutual TLS) | 79 |
| **Networking** | routing, NAT, firewalls, DNS, TLS, proxies, WireGuard, netem · OSPF, BGP, VRRP and IPv6 routing on FRR · paths, tunnels & TLS (subnetting, PMTU, GRE/VXLAN, bonding, certificate chains) *(growing)* · TCP & sockets (connection states, TIME-WAIT, ephemeral ports, listen queues, Nagle, the bandwidth-delay product) | 77 |
| **Cloud** | AWS CLI, CloudFormation and IAM permission debugging against a local emulator · events, storage & governance (SQS/SNS/EventBridge, S3 policies, KMS, DynamoDB, RBAC) *(growing)* · managed databases: RDS & Aurora (backups, replicas, encryption, failover, IAM auth, DR snapshots) | 82 |
| **Data** | SQL, PostgreSQL operations and Redis · point-in-time recovery, replication and failover, PgBouncer, Redis Sentinel and AOF repair · SQL in depth & Redis data structures (window functions, JSONB, RLS, sorted sets, geo, ACLs) *(growing)* · logic in the database (PL/pgSQL functions & triggers, SECURITY DEFINER, Redis Lua and Functions) | 72 |
| **Defensive Security** | incident response and hardening · packet-capture forensics with tshark · AppArmor, seccomp, SSH certificates and WAF tuning · threat hunting & containment (log analysis, deobfuscation, persistence, IR) *(growing)* · web & TLS hardening (nginx, security headers, CSP, internal CAs, mutual TLS, revocation) | 75 |
| **Programming** | Python and Go · common pitfalls & idioms (closures, decimals, slices, error wrapping, concurrency) *(growing)* · secure coding (path traversal, XSS, password hashing, ReDoS, SSRF, zip slip, webhook signatures) | 80 |
| **Data Science** | pandas, statistics and machine learning · analysis & model diagnostics (tidy data, joins, chi-square, calibration, importance) *(growing)* · customer & product analytics (funnels, cohorts, RFM, survival, attribution, churn). **Done in Jupyter notebooks** that the checks run | 78 |
| **Git** | the basics · branching & merging · inspecting history · undoing changes · rebasing & rewriting · stashing & tagging · remotes & collaboration · recovery & detective work (reflog, bisect, fsck) · plumbing & power tools · real-world workflows · config & aliases · ignore rules & attributes · hooks · submodules & subtrees · advanced merging (strategies, rerere, merge-tree) · searching & comparing · large repositories (shallow, partial, sparse) · repository troubleshooting · identity, SSH signing & trailers · multi-problem incidents · revisions, refs & scripting (gitrevisions, for-each-ref, plumbing commits, grafts) · hosting & serving repositories (bare repos, mirrors, git daemon, receive protections, deploy hooks) | 235 |
| **Terraform** | the basics (init, plan, apply, outputs, provider versions) · variables, expressions & meta-arguments (`count`, `for_each`, templates, validation) · working with state (moved, removed, import, workspaces, backends, recovery) · modules (local, Git-pinned, refactoring into modules) · real Docker infrastructure with the `kreuzwerker/docker` provider · functions, lifecycle & testing (dynamic blocks, `lifecycle`, `check`, `import`, `terraform test`) *(growing)* · Terraform in automation (saved plans, exit codes, JSON plan gates, state locks, mock-provider tests, pipelines) · every module runs beginner → advanced and ends with a real-world capstone lab | 92 |
| **Ansible** | inventories & ad-hoc commands · playbooks (handlers, loops, conditionals, idempotence) · templates, files & variables (Jinja2, precedence, Vault) · roles & reuse (role design, dependencies, Galaxy from Git, ansible-lint) · operations & incidents (check mode, validate, block/rescue, become, constructed inventories) against SSH-managed nodes · control flow & extending Ansible (tags, blockinfile, delegation, handlers, vault IDs, collections, custom filters/modules) *(growing)* · data structures & types (dicts, merging, JSON in and out, type errors) | 85 |
| **Algorithms** | searching & sorting · data structures · graphs · dynamic programming · strings, greedy & maths — each exercise solvable in **Python or Go** *(growing)* · recursion & backtracking | 65 |

That's 1553 exercises so far, with a target of 250 per track; ideas for what comes next are
in [docs/PENDING.md](docs/PENDING.md).

Difficulty rises through each module (1 = beginner … 5 = expert), mixing **build** tasks
("create X" / "capture the flag") and **fix** tasks ("X is broken — find out why" /
"patch the vulnerability"). The Docker and Kubernetes modules end with a multi-problem
*incident*. Every Terraform module goes beginner → intermediate → advanced and ends with a
**capstone**: one big, realistic lab (a ticket from a real team) that combines the module's skills.

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
in seconds; `awesome_labs env prepare [docker|k8s|offsec]` (or **P** on the environments
screen) does the same at any time. Before every exercise awesome_labs **resets** the environment
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
(v0.19.0) has static binaries for Linux and macOS on x86-64 and ARM. On Linux x86-64:

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
awesome_labs             # the labs in your web browser (the default)
awesome_labs tui         # or the interactive browser in this terminal
```

`make build` produces a single static binary with the exercises built in. `make release`
builds one for Linux and macOS on x86-64 and ARM into `dist/`, with checksums.

### The web interface

The web interface is awesome_labs' main interface: `awesome_labs` (the same as
`awesome_labs web`) serves the labs as a web page, with **terminals into the lab VMs in
the browser**. It prints a link and opens it; the page and the terminals run until you
press `ctrl+c` in the terminal you started it from.

```bash
awesome_labs                         # http://127.0.0.1:7681, opens your browser
awesome_labs web --addr 127.0.0.1:9000 --no-open
```

- **The exercise screen** has the same tabs as the terminal UI (Scenario, Learn, Hints,
  Solution, Output, Notes) and buttons for Start, Terminal, Check, Hint, Solution, End,
  Restart and Reset progress. A button is dimmed when it can't act yet and says why when
  pressed. Start, Check and the environment operations stream their output live into the
  Output tab and the status bar.
- **Terminals** open in a panel below the exercise, or beside it (`⇆`), or maximized
  (`⤢`): one tab per VM (`controlplane` and `node01` for Kubernetes). They run the same
  shell as `t`/`w`, so your command history per exercise is kept. Starting the lab opens
  one by itself. Each code block in the text has **Copy** and **▸ Terminal**, which pastes
  it into the open terminal for you to run with Enter. A shell that ended reconnects on
  Enter.
- **Notebooks:** the Data Science track is done in Jupyter notebooks. **📓 Notebook** opens
  JupyterLab in the same panel, on the exercise's notebook `/lab/<id>.ipynb`, which
  starting the exercise writes: how it's graded, the code to fix (if any) and a save cell
  for each file to produce. It runs on the docker host in `/lab`, as the lab user, with
  pandas, NumPy, SciPy, scikit-learn and matplotlib. **The notebook is what's graded:**
  Check runs it from top to bottom in a fresh kernel, on a copy of `/lab` without the
  files it must write, then grades what it wrote, and calls the functions the task names
  with the check's own data. Files made by hand don't count, and the check's hidden data
  is out of the notebook's reach while it runs. The page lists the files each exercise
  grades and how to save them. The container is capped at 2.5 GiB; closing the notebook
  tab stops it and resetting the lab removes it. JupyterLab is only reachable through
  the web interface, which adds the notebook's token itself. In the terminal UI, open
  the notebook in VS Code (`o`).
- **Environments & VMs** shows every environment and VM with start, stop, reset, prepare,
  rebuild and delete.
- **First launch** offers to build the environments that have no saved copy yet, one
  checkbox each (the terminal UI asks the same once; whichever asks first, it isn't asked
  again). Each is built, saved and stopped, so its first exercise starts in seconds.
- **Only what's needed runs.** With `auto_stop: true` (the default) the server stops the
  running lab VMs once no exercise is set up and nothing (an operation, a terminal, a
  notebook) has used them for 10 minutes, or when an exercise is set up but no page has
  been open for 30 minutes. Stopping keeps the disks and the lab: the exercise's page
  boots it again.
- The list on the left filters by text, track, progress and learning path; drag its
  right edge to widen it (double-click resets it). `/` focuses the filter, `j`/`k` move
  through the list and `1`–`6` switch tabs.

Exams are started from the terminal UI (`X`); while one runs the web page honours it:
hints and the solution of its questions are locked and its checks count.

**Security.** A terminal in a web page is a shell on your lab VMs, so the server listens on
`127.0.0.1` only, and every request needs the random token in the printed link. The token
is kept in a same-site cookie after the first visit and is new each run. Requests from other
sites and other `Host` names are refused. `--addr 0.0.0.0:7681` makes it reachable from
your network: anyone with the link then gets those shells, so only do that on a network
you trust.

Like the terminal UI, the web interface takes the single-instance lock, and `ctrl+c` treats
the lab VMs the way leaving the terminal UI does (`on_exit`: by default every lab VM is
stopped, keeping the exercise you were on set up; `ctrl+c` a second time leaves them as
they are). Stopping it while an operation runs cancels that operation and leaves the VMs
as they are, and so does failing to start (say, the port is taken). Unlike the terminal UI with `on_exit: delete`, it does not
delete leftover VMs when it starts.

### The terminal UI

`awesome_labs tui` opens the same labs in your terminal: a keyboard- and mouse-driven
exercise browser that drops you into a shell in the lab VM. It is also where exams are
started (`X`).

#### Menus and the mouse

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
keys that work on every screen still do. Elsewhere, **Program** holds Exit. **View** and
**Tracks** shape the list: View also holds learning paths, and **View → Show** filters the
list by progress: all, solved, in progress (started, not solved) or no progress. **Tracks**
lists every track in the catalogue (including Git, Terraform, Ansible and Algorithms) with
how many of its exercises you have solved. A greyed-out item says why when you pick it.

The mouse works throughout: pick a track from the **Tracks** menu, click a row to select
it and double-click to open it, click the tabs and the buttons (Start, Terminal, Check,
Hint, Solution…) on the exercise screen, and scroll with the wheel. Each button shows its
key next to its name (on a narrow terminal, just the name). Confirmations open
as a dialog with **Yes** / **No** buttons (`y`/`n` still work). `F1` lists every
shortcut. While the mouse is in use, most terminals select text with `shift`+drag.

#### The exercise list

| Key | Action |
|-----|--------|
| `↑↓` / `j k` | move · `n`/`p` next/previous module |
| `/` | filter by id, title, tag, module or difficulty · `ctrl+t` while filtering also searches the scenarios, tutorials and hints |
| `tab` | switch track |
| `u` | cycle the progress filter: all · solved · in progress (started, not solved) · no progress |
| `enter` | open an exercise |
| `space` / `*` | mark or unmark the highlighted exercise / mark every exercise shown (again: clear) · `esc` clears the marks |
| `R` | **reset progress** of the marked exercises, or the highlighted one when none is marked (asks first; notes are kept). View → *Reset progress of all shown* resets the whole filtered list — e.g. `u` to *in progress*, then reset them all |
| `L` | **learning paths** — focus the list on a curated path, with progress |
| `W` | **weak spots** — the modules where you needed the most hints, solutions and checks, and the exercises worth redoing |
| `X` | **exam mode** — start a timed exam, or return to the running one |
| `a` | jump to the exercise you're **working on** (shown as `▶` in the header and list) |
| `S` | your labs: the active exercise, then others started earlier and not solved |
| `e` | environments & VMs: live state of every VM; create, stop, reset, rebuild, delete, **P** prepare (build and keep a saved copy) — `X` stops all, `K` deletes all |
| `Q` (`q` on the list, `ctrl+c`, Program → Exit) | exit — handles the lab VMs by `on_exit` first: by default **stops** them, keeping the exercise you were on set up (`ctrl+c` while busy or stopping exits at once and leaves them as they are) |

#### In an exercise

| Key | Action |
|-----|--------|
| `s` | **start** — prepare the lab (creates the VMs the first time) |
| `t` | open a **terminal** in the docker host / control plane (`exit` to come back) |
| `w` | open a terminal on **node01** (Kubernetes only) |
| `o` | work on the lab in your own **VS Code** (see below); while its sync runs, `o` (**■ Stop VS Code**) stops it |
| `c` | **check** your work |
| `h` | reveal the next **hint** |
| `v` | reveal the **solution** (asks first) |
| `x` | **end** the exercise: forget its lab and clean its VMs for the next one (asks first) |
| `R` | **reset progress** on this exercise (asks first) |
| `A` | **auto-check** on/off: check whenever you leave the terminal or save in VS Code |
| `y` | **copy** a code block of the tab you're on; `y` again copies the next one |
| `enter` | after a pass, open the suggested next exercise (the next unsolved one in the list) |
| `1`–`6` / `tab` / `←→` | Scenario · Learn · Hints · Solution · Output · Notes |
| `n` / `N` | next / previous in the list you opened it from: the filtered list, Started labs, or the exam's questions |
| `esc` / `q` | back to where you opened it from |

Leaving the exercise that is set up on the VMs **keeps its lab**: your files and
containers stay in the VMs, and `a` (or opening it again) takes you back to carry on
where you left off. The lab is cleaned only when you **end** the exercise (`x`), start
another one (asks first), or exit the app with `on_exit: delete`. Ending it resets the environment to pristine
in place and leaves it running, so the next exercise on it starts without building or
booting anything (and skips its own reset). An exercise that breaks its environment
beyond a reset has its VMs deleted instead. Starting an exercise **deletes the VMs of
every other environment**, running or stopped, since that exercise doesn't need them.
During an exam the VMs keep running, since they carry on to the next question. The environments screen goes
back to the exercise you came from.

**Notes** (tab 6, `enter` to edit, `esc` saves) are yours per exercise, kept in
`~/.awesome_labs/notes.json`; resetting progress keeps them. **Your commands:** shells
opened from awesome_labs (`t`, `w`, or VS Code's terminal) keep a history per exercise,
which each check copies to `~/.awesome_labs/history/`. Once you reveal the solution, the
Solution tab lists your commands under it, to compare. **Copying** uses the system
clipboard (`pbcopy`, `wl-copy`, `xclip`…) and falls back to the terminal (OSC 52), which
also works over SSH.

While an environment builds or boots, the status line shows the step it's on, the time
so far and, from the last run of the same kind, an estimate with a progress bar.

Inside the VMs the scenario is also in **`/lab/TASK.md`**; `/lab` is your working
directory. On the control plane `kubectl` (alias `k`, with completion) is configured
for you, and `ssh node01` gets you onto the worker.

### In VS Code

`o` on the exercise screen (or `awesome_labs code [docker|k8s|node01|offsec]`, which
defaults to the exercise you're on) opens the lab in **your own VS Code**. Nothing is
installed in the VMs: no VS Code server, no extension, no mount.

- **Files:** the workspace folder `~/.awesome_labs/vscode/<lab>/` mirrors the VM's `/lab`
  both ways while the window is open. A task in the workspace, *awesome_labs: sync /lab*,
  runs `awesome_labs sync` over the VM's existing SSH connection. VS Code starts it when
  the window opens; allow automatic tasks if it asks. Saving a file writes it into the VM
  within a second, and anything created or changed in the VM shows up in the editor.
  Files are written into the VM in place, so a file bind-mounted into a container keeps
  working. New files belong to the lab user. `.git` folders, symlinks and files over
  16 MiB are not mirrored.
- **Terminal:** the workspace's terminal opens a shell in the VM (`awesome_labs shell`),
  with a node01 profile in the Kubernetes lab. `awesome_labs shell`, `code` and `sync` run
  beside the app.
- **The VM wins:** when the sync (re)connects, and when a file changed on both sides, the
  VM's version is kept. A reset or a new exercise empties the folder along with `/lab`.
  Any of your edits that the VM never got are copied to `~/.awesome_labs/vscode/backups/`
  first.

**Stopping:** press `o` again on the exercise screen (the button reads **■ Stop VS Code**
while a sync runs), or close the window. Exiting awesome_labs, or deleting all VMs, stops
every sync too. Stopping leaves the VS Code window open, since VS Code can't be told from
outside to close one window. Its edits just stop reaching the VM.

`code` must be on your PATH (in VS Code: *Shell Command: Install 'code' command in
PATH*); on macOS the app in `/Applications` is found without it. Set
`AWESOME_LABS_CODE=codium` (or `cursor`, …) to use another editor of the same family.
Files outside `/lab` (`/etc`, the cluster's manifests…) are still edited from the
terminal.

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
| Incident drills | the multi-problem incidents at the end of each module |

Paths are defined in `learning-paths.yaml` (filters by track, module, kind, tag,
difficulty or id).

### Exam mode

In the terminal UI (`awesome_labs tui`), press `X`, pick a pool (a whole track or any path), the number of questions, a time
limit and whether hints are allowed. awesome_labs draws a balanced random set: about a
third easy, a third medium, a third hard. A countdown runs in the header, hints and
solutions are locked, and **the last check of each question counts**. When you finish
(`F`) or time runs out you get your score against a **66 % pass mark**, and solutions
unlock. Exams are kept in `~/.awesome_labs/exam-history.json`.

```bash
awesome_labs exam status     # the running exam
awesome_labs exam history    # past results
awesome_labs exam abandon    # give up the running exam
```

### Saving memory, and other settings

Only one environment runs at a time: starting a Docker exercise deletes the Kubernetes
VMs and vice versa, while the environment you're using stays up and is cleaned between
exercises. By default exiting stops every lab VM (their memory is freed, their disks
kept): the exercise you were on is still set up next time, and `t` or `c` starts its VMs
again without a reset, while starting a different exercise resets the lab as always. With
`on_exit: keep` they are left running, and with `on_exit: delete` exiting deletes every lab
VM, running or stopped, and starting the app deletes any left over. With
`auto_stop: true` (the default), solving an exercise with `awesome_labs check` stops its
VMs. Settings live in `~/.awesome_labs/config.yaml`:

```yaml
auto_stop: true
on_exit: stop          # stop · delete · keep
auto_check: false      # start with auto-check on (A toggles it)
check_updates: false   # look for a newer release once a day; nothing about you is sent
theme: auto            # auto · dark · light (NO_COLOR=1 turns colours off)
docker:       {cpus: 2, memory: 4GiB}       # applied when a VM is (re)created:
controlplane: {cpus: 2, memory: 3GiB}       #   awesome_labs env rebuild docker|k8s
worker:       {cpus: 2, memory: 2560MiB}
```

### Command line

```bash
awesome_labs install [--dry-run] [--yes] [--bin-dir DIR]   # set up Lima, QEMU and the binary
awesome_labs [web] [--addr HOST:PORT] [--no-open]   # the labs in your browser, with web terminals (the default)
awesome_labs tui                        # the interactive browser in this terminal
awesome_labs status                     # the exercise you're working on + running VMs
awesome_labs list [filter]              # list exercises (✔ solved, ● started)
awesome_labs paths [path-id]            # learning paths and progress
awesome_labs show k8s-101 [--solution]
awesome_labs start docker-042           # prepare the lab
awesome_labs shell docker|k8s|node01|offsec    # open a shell
awesome_labs code [docker|k8s|node01|offsec]   # open the lab's /lab in VS Code
awesome_labs sync [docker|k8s|node01|offsec]   # mirror /lab into ~/.awesome_labs/vscode/<lab> (VS Code runs it)
awesome_labs check docker-042           # grade (exit code 0 = solved)
awesome_labs forget docker-042          # erase progress on one exercise
awesome_labs env status|up|stop|reset|rebuild|destroy [docker|k8s|offsec]
awesome_labs env prepare [docker|k8s|offsec]   # build now and keep a saved copy (VMs deleted after)
awesome_labs env cache [docker|k8s|offsec]     # fill the local image cache (no env: show it)
awesome_labs vms                        # every lab VM with status, CPU and memory
awesome_labs vms stop|delete <name|all> # e.g. vms delete kl-offsec · vms stop all
awesome_labs doctor [--fix]             # check this machine and awesome_labs for the usual trouble
awesome_labs version [--check]          # this build's version; --check: is a newer release out?
```

You work on **one exercise at a time**: starting one resets its environment, so it
replaces whatever was set up before (the UI asks first). It stops being active when
you solve it, or when its environment is reset, rebuilt or deleted.

Only **one awesome_labs runs at a time**: the app and every command that changes the
VMs or your progress take a lock on `~/.awesome_labs/app.lock`, and a second one exits with
*another awesome_labs is already running*. The lock goes away with the process, even after
a crash. The read-only `list`, `show`, `status`, `paths` and `lint` still run beside it,
and so do `shell`, `code` and `sync`, which only work inside a running VM.

Progress is stored in `~/.awesome_labs/progress.json`, logs in `~/.awesome_labs/logs/`.
When something seems stuck or broken, `awesome_labs doctor` is the first thing to run:
it checks Lima (and QEMU on Linux), free disk and RAM, whether the `awesome_labs` on your
PATH is a different build from the one you're running, environments left marked busy by
a crashed run (`--fix` clears them), stray VMs and missing saved copies, and says how to
fix each one.

## Writing exercises

Exercises are YAML files in `exercises/<track>/NN-<module>.yaml`, embedded into the
binary at build time. See **[docs/AUTHORING.md](docs/AUTHORING.md)**. Every exercise
ships with a `solve` script.

```bash
awesome_labs lint                    # validate every exercise file
awesome_labs verify [-from id] [id-prefix ...]
awesome_labs notebook ds-001         # print the starter notebook a notebook exercise writes
```

`verify` runs each one end to end — *reset → setup → check must fail → solve → check must
pass* — which is how the whole catalogue is tested. Each exercise gets 25 minutes (50 for the
destructive ones). Like the app, a verify run needs to be the only awesome_labs running. Set `AWESOME_LABS_EXERCISES=./exercises`
to work on exercise files without rebuilding.

## Project layout

```text
main.go                    CLI entry point, verify runner
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
learning-paths.yaml        curated paths
vms/*.yaml                 Lima templates
vms/bootstrap/             one-time provisioning (kubeadm init, add-ons, image pre-pulls)
vms/kit/                   copied to /opt/kubelab in every VM: lib.sh helpers, reset & snapshot scripts
exercises/                 the exercises, one folder per track
```

## License

[MIT](LICENSE)
