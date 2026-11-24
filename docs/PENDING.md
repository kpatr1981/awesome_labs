# Pending ideas

Ideas for future labs. Each topic has one home; where tracks touch, the row says which
track it relies on.

**Target: 250 labs in each of the 14 tracks, 3,500 in total.** Once tracks reach it, review
what to consolidate. The target doesn't lower the bar: a lab should carry a real multi-step
scenario and teach a concept. The parts of this list that the existing exercises don't cover
are not started.

Exercise counts (<!-- count:total -->1,772<!-- /count --> total):

| Topic | Exercises | Target | Track | Module files |
|---|---|---|---|---|
| Docker | <!-- count:docker -->186<!-- /count --> | 250 | `docker` | `exercises/docker/` |
| Kubernetes | <!-- count:kubernetes -->318<!-- /count --> | 250 | `kubernetes` | `exercises/kubernetes/` |
| Offensive security | <!-- count:offsec -->83<!-- /count --> | 250 | `offsec` | `exercises/offsec/` |
| Linux & systems | <!-- count:linux -->86<!-- /count --> | 250 | `linux` | `exercises/linux/` |
| Networking | <!-- count:networking -->106<!-- /count --> | 250 | `networking` | `exercises/networking/` |
| Cloud & platform | <!-- count:cloud -->93<!-- /count --> | 250 | `cloud` | `exercises/cloud/` |
| Data | <!-- count:data -->78<!-- /count --> | 250 | `data` | `exercises/data/` |
| Defensive security | <!-- count:defensive -->78<!-- /count --> | 250 | `defensive` | `exercises/defensive/` |
| Programming | <!-- count:programming -->107<!-- /count --> | 250 | `programming` | `exercises/programming/` |
| Data science | <!-- count:datascience -->83<!-- /count --> | 250 | `datascience` | `exercises/datascience/` |
| Git | <!-- count:git -->275<!-- /count --> | 250 | `git` | `exercises/git/` |
| Terraform | <!-- count:terraform -->120<!-- /count --> | 250 | `terraform` | `exercises/terraform/` |
| Ansible | <!-- count:ansible -->89<!-- /count --> | 250 | `ansible` | `exercises/ansible/` |
| Algorithms | <!-- count:algorithms -->70<!-- /count --> | 250 | `algorithms` | `exercises/algorithms/` |

- [Suggested order](#suggested-order)
- [Deepen every module](#deepen-every-module)
- [Gaps in Kubernetes, Terraform and Git](#gaps-in-kubernetes-terraform-and-git)
- [Shared lab foundations](#shared-lab-foundations)
- [Linux & systems](#linux--systems)
- [Networking](#networking)
- [Cloud & platform](#cloud--platform)
- [Data](#data)
- [Security](#security)
- [Reverse engineering](#reverse-engineering)
- [Active Directory on Linux](#active-directory-on-linux)
- [Industrial & IoT](#industrial--iot)
- [Event streaming](#event-streaming)
- [AI engineering with tiny models](#ai-engineering-with-tiny-models)
- [Programming](#programming)
- [Practice modes](#practice-modes)
- [awesome_labs features](#awesome_labs-features)
- [More lab ideas](#more-lab-ideas)
- [Left out for now](#left-out-for-now)

## Suggested order

### Build next (decided 2026-11-24)

These come **before** everything else on this page, in this order. None is started.

1. **Reviewing AI-written code** (Programming track, new module). *"An AI wrote this PR:
   find what's wrong before it merges."* Each lab is a plausible-looking diff in Python or
   Go with one planted defect: a data race (`go test -race`, a Python counter shared by
   threads), a silent logic error (an off-by-one in pagination, wrong rounding, a timezone
   slip), a leak (goroutines, file handles, an unbounded cache), a **made-up or
   typo-squatted package** in `requirements.txt`/`go.mod` (the offline mirror holds a
   malicious look-alike), an insecure default, a test that passes without testing
   anything. Checks: hidden tests and the race detector on the learner's fixed code, plus a
   `findings.txt` naming the defect's file and line. No models and no new infrastructure.
2. **MCP servers** (Programming track, new module). Build Model Context Protocol tool
   servers in Python (official `mcp` SDK) or Go, over stdio and HTTP: expose a read-only
   database query tool, a service-status tool, a file reader limited to one directory.
   Practise input schemas, an allow-list of tools and arguments, refusing path traversal
   and SQL outside a fixed set of queries, timeouts, an audit log of every call, a secret
   that must never appear in a tool result. Checks: a scripted MCP client lists the tools,
   calls them with good and hostile arguments, and inspects the replies and the audit log.
   No model needed, so it's fully repeatable. Pre-pull the SDKs into the image.
3. **AI engineering: *Serving a model*** (see
   [AI engineering with tiny models](#ai-engineering-with-tiny-models)), **with a LiteLLM
   gateway added**: two llama.cpp servers (a 0.5B "cheap" model and a 1.5B "expensive"
   one) plus a deliberately failing upstream behind LiteLLM's proxy. Labs: virtual keys,
   rate limits and per-key budgets, a fallback chain when the upstream fails, routing short
   requests to the cheap model, reading LiteLLM's spend logs. Also cover Ollama beside
   llama.cpp, GGUF quantization levels (Q8 vs Q4: RSS, tokens/s, `llama-perplexity`) and the
   KV-cache settings (`--ctx-size`, `-np` parallel slots, `--cache-type-k`). vLLM, AWQ and
   FP8 need a GPU: explain them in the tutorial only. **Test memory first**: two models
   plus LiteLLM on the 4GiB Docker VM.
4. **AI engineering: *Retrieval*** (same section), including **hybrid search** (PostgreSQL
   full-text or BM25 plus pgvector), a small cross-encoder re-ranker, and **knowledge
   hygiene**: near-duplicate documents, a stale version that wins the search, metadata
   filters, re-indexing on change.
5. **Sandboxing untrusted code with gVisor** (Defensive security or Docker track). Run
   generated code under `runsc` (arm64 works) in the Docker VM: no network or an egress
   allow-list in nftables, seccomp, read-only root, CPU/memory/pids limits, no secrets
   mounted. Checks attempt escapes, network calls, fork bombs and reading host files.
   Firecracker needs nested virtualization (Apple M3 or newer only), so at most an optional
   lab.
6. **Durable workflows** (Programming track, new module): Temporal's dev server (single
   binary) with Python or Go workers. Retries with backoff, activity timeouts,
   idempotent activities, a human-approval step via a signal, a worker killed mid-run that
   resumes where it left off, a workflow change that breaks determinism. Optionally
   LangGraph with SQLite checkpoints and a fake model. Checks read workflow history through
   the Temporal CLI.
7. **Model supply chain** (Defensive security or Offensive security track). Model files
   can run code when loaded: a pickle-based `.pt`/`.bin` that opens a shell on
   `torch.load`, found with `fickling`/`modelscan`-style inspection (or a small stdlib
   `pickletools` scanner), and converted to **safetensors**. Also: pinning model files by
   SHA-256 in a manifest, a typo-squatted model name in a config, a model swapped on a
   mirror after the hash was recorded, provenance notes (who published it, which
   licence). Checks: the loader refuses unpinned or pickle files, the safe copy loads with
   identical tensors, the malicious file never executes (a canary file stays absent).
8. **When LLM APIs fail** (Programming track, beside *Serving a model*). A mock
   OpenAI-compatible upstream that the lab controls: 429s with `Retry-After`, 5xx bursts,
   a stream that stops mid-answer, a slow first token, a tool call that times out. The
   learner's client must back off with jitter and honour `Retry-After`, retry without doing
   the work twice (idempotency keys on the side-effecting tool), resume or cleanly fail a
   broken stream, open a circuit breaker, and keep a deadline across a multi-step tool
   chain. Checks read the mock's request log (counts, timing, duplicate side effects).
   Deterministic, no model needed.
9. **Running coding agents safely** (Defensive security track; overlaps item 5). A coding
   agent is untrusted code with your credentials. Labs: a devcontainer with no host home
   directory and no SSH agent, an egress allow-list (package mirror and Git server only),
   a scoped short-lived CI token instead of a personal token, a secrets scan that blocks a
   commit, a hook/permission policy that refuses `rm -rf`, `curl | sh` and pushes to
   `main`, and an audit log of every command. Checks replay a scripted "agent" session
   with hostile steps and confirm each is refused and logged. The real agent tools need
   accounts, so the agent is a script.

### Earlier order

1. **Quick wins, no new infrastructure:** Linux admin first, then Git and Shell. They run
   on the existing VMs and are easy to check.
2. **Networking & IP routing**, which the rest of the networking group builds on.
3. ~~**TLS & PKI**~~, before OpenVPN and the mail TLS exercises. Done in
   `exercises/networking/10-tls-and-pki.yaml`, and cert-manager in
   `exercises/kubernetes/23-cert-manager.yaml`.
4. **Degraded networks and VPN creation.**
5. **DNS & email**, with DNS first, because MX records, SPF, DKIM and DMARC are all DNS.
6. **Everything else**, in any order.

## Deepen every module

Every module now runs beginner → intermediate → advanced and ends with a capstone or
incident, but many are thin: **39 of the 142 modules have fewer than 10 exercises** and 103
have fewer than 15. The next round of content should fill the modules before opening new
ones.

- **Target per module:** 20 exercises for a track's first module and 15 for the others,
  split across the levels — see [Module targets](#module-targets).
- **Empty levels first.** A module with no beginner exercises drops a learner straight
  into the middle; two of them, *Networking essentials* and *Web exploitation —
  fundamentals*, are the first modules of their track. Then modules with no advanced
  exercises, then the rest by size.
- **New exercises go under their level's header** (`# ===== Beginner =====` …), before the
  capstone, so the order stays easy → hard. Ids are never reused or renumbered, so new ones
  take the track's next free number.
- **Same bar as before:** a real multi-step scenario that teaches a concept, each one
  checked with `verify -local` (docker profile) while writing and with `verify -record` on
  a lab VM before release, so the release gate stays green.

### Module targets

Each module gets a target by its role. **The first module of a track: 20 exercises** —
8 beginner, 6 intermediate, 5 advanced and the capstone — since it's where newcomers start
and needs the gentlest ramp. **Every other module: 15** — 5 beginner, 5 intermediate,
4 advanced and the capstone or incident. The targets are floors for each level, not caps:
a big module keeps what it has but still fills a level it's short on. Levels count the
exercises before the capstone/incident (B = difficulty 1–2, I = 3, A = 4–5).

Reaching every module target takes **836 new exercises**:

| Track | Modules | Now | New exercises for the module targets | Then |
|---|---|---|---|---|
| Docker | 15 | 186 | 86 | 272 |
| Kubernetes | 21 | 273 | 105 | 378 |
| Offensive security | 7 | 83 | 40 | 123 |
| Linux & systems | 9 | 86 | 69 | 155 |
| Networking | 10 | 98 | 70 | 168 |
| Cloud & platform | 8 | 88 | 50 | 138 |
| Data | 8 | 78 | 61 | 139 |
| Defensive security | 8 | 78 | 63 | 141 |
| Programming | 7 | 86 | 38 | 124 |
| Data science | 7 | 83 | 37 | 120 |
| Git | 22 | 255 | 123 | 378 |
| Terraform | 7 | 92 | 32 | 124 |
| Ansible | 7 | 89 | 31 | 120 |
| Algorithms | 6 | 70 | 31 | 101 |

Tracks still under 250 after that reach it with new modules on topics they don't cover yet
(the sections below), each built to the same 15-exercise shape.

**Docker**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Containers — first steps *(first module)* | 9 | 20 | 2 / 4 / 5 |
| Images & Dockerfiles | 17 | 15 | 0 / 0 / 2 |
| Volumes & storage | 16 | 15 | 0 / 0 / 3 |
| Networking | 14 | 15 | 0 / 0 / 3 |
| Docker Compose — essentials | 15 | 15 | 0 / 0 / 4 |
| Registries & image distribution | 16 | 15 | 0 / 0 / 2 |
| Security & resource limits | 14 | 15 | 2 / 0 / 4 |
| The Docker daemon & the host | 13 | 15 | 1 / 0 / 4 |
| Production patterns | 11 | 15 | 2 / 0 / 4 |
| Troubleshooting in the real world | 11 | 15 | 5 / 0 / 3 |
| Advanced builds & operations | 5 | 15 | 5 / 3 / 2 |
| Registry & runtime internals | 4 | 15 | 5 / 3 / 3 |
| Observability & logging | 6 | 15 | 5 / 1 / 3 |
| Builds, runtime controls & Compose II | 20 | 15 | 0 / 0 / 4 |
| Automating Docker — the Engine API & scripts | 15 | 15 | 2 / 0 / 0 |

**Kubernetes**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Pods & kubectl essentials *(first module)* | 13 | 20 | 2 / 0 / 5 |
| Deployments & rollouts | 11 | 15 | 1 / 0 / 4 |
| Services & DNS | 14 | 15 | 0 / 0 / 4 |
| ConfigMaps & Secrets | 16 | 15 | 0 / 0 / 4 |
| Scheduling | 14 | 15 | 0 / 0 / 4 |
| Storage | 18 | 15 | 0 / 0 / 4 |
| Jobs, CronJobs, DaemonSets & more | 16 | 15 | 0 / 0 / 4 |
| Health, resources & autoscaling | 15 | 15 | 0 / 0 / 4 |
| Ingress & Gateway API | 17 | 15 | 1 / 0 / 2 |
| Network policies | 9 | 15 | 4 / 0 / 3 |
| RBAC & pod security | 16 | 15 | 0 / 0 / 3 |
| Troubleshooting applications | 13 | 15 | 3 / 0 / 3 |
| Troubleshooting the cluster | 11 | 15 | 3 / 0 / 1 |
| Cluster administration | 13 | 15 | 5 / 0 / 0 |
| Helm, Kustomize, CRDs & the API | 15 | 15 | 2 / 0 / 3 |
| Day-two operations | 6 | 15 | 4 / 2 / 3 |
| Config & scheduling II | 9 | 15 | 4 / 2 / 0 |
| Workloads & reliability II | 8 | 15 | 4 / 2 / 1 |
| BGP & routing to the outside | 4 | 15 | 5 / 5 / 1 |
| Advanced workloads & policy | 20 | 15 | 0 / 0 / 1 |
| Failure policies & lifecycle controls | 15 | 15 | 2 / 0 / 0 |

**Offensive security**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Web exploitation — fundamentals *(first module)* | 9 | 20 | 8 / 2 / 1 |
| Linux privilege escalation | 13 | 15 | 5 / 2 / 0 |
| Network services & credentials | 6 | 15 | 3 / 3 / 3 |
| Tokens & weak crypto | 9 | 15 | 1 / 2 / 3 |
| Forensics & artifact analysis | 11 | 15 | 1 / 1 / 2 |
| Web & API attacks | 20 | 15 | 0 / 0 / 1 |
| Applied cryptography attacks | 15 | 15 | 2 / 0 / 0 |

**Linux & systems**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Linux essentials *(first module)* | 5 | 20 | 6 / 4 / 5 |
| Systems & internals | 8 | 15 | 2 / 2 / 3 |
| Everyday administration | 7 | 15 | 3 / 4 / 1 |
| Processes & storage | 8 | 15 | 3 / 0 / 4 |
| Networking & services | 7 | 15 | 5 / 1 / 2 |
| Git beyond the basics | 5 | 15 | 5 / 3 / 2 |
| Container internals | 11 | 15 | 5 / 4 / 0 |
| Files, services & recovery | 20 | 15 | 0 / 0 / 3 |
| Security & hardening | 15 | 15 | 2 / 0 / 0 |

**Networking**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Networking essentials *(first module)* | 4 | 20 | 8 / 5 / 3 |
| Switching, services & tunnels | 9 | 15 | 2 / 2 / 2 |
| Traffic & troubleshooting | 8 | 15 | 3 / 4 / 0 |
| Services & scale | 8 | 15 | 5 / 1 / 1 |
| DNS, NAT & firewalls | 5 | 15 | 5 / 3 / 2 |
| Dynamic routing with FRR | 10 | 15 | 5 / 2 / 0 |
| IPv6 routing with FRR | 4 | 15 | 5 / 4 / 2 |
| Paths, tunnels & TLS | 20 | 15 | 0 / 0 / 4 |
| TCP & sockets | 15 | 15 | 2 / 0 / 0 |
| TLS & PKI | 15 | 15 | 0 / 0 / 0 |

**Cloud & platform**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| AWS CLI essentials *(first module)* | 6 | 20 | 5 / 5 / 4 |
| AWS services in practice | 10 | 15 | 1 / 1 / 3 |
| AWS automation & governance | 10 | 15 | 2 / 2 / 1 |
| Data & messaging | 10 | 15 | 3 / 0 / 3 |
| Compute & networking | 11 | 15 | 0 / 3 / 2 |
| IAM & permissions | 6 | 15 | 4 / 3 / 2 |
| Events, storage & governance | 20 | 15 | 0 / 0 / 4 |
| Managed databases (RDS & Aurora) | 15 | 15 | 2 / 0 / 0 |

**Data**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Databases essentials *(first module)* | 4 | 20 | 7 / 4 / 5 |
| Database operations | 8 | 15 | 3 / 1 / 3 |
| Advanced SQL & data services | 8 | 15 | 4 / 2 / 1 |
| Performance & Redis patterns | 5 | 15 | 5 / 3 / 2 |
| Transactions & integrity | 7 | 15 | 4 / 3 / 1 |
| Recovery & resilience | 11 | 15 | 5 / 4 / 0 |
| SQL in depth & Redis data structures | 20 | 15 | 0 / 0 / 2 |
| Logic in the database | 15 | 15 | 2 / 0 / 0 |

**Defensive security**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| First response *(first module)* | 3 | 20 | 8 / 4 / 5 |
| Hardening & investigation | 6 | 15 | 2 / 3 / 4 |
| Detection & monitoring | 8 | 15 | 4 / 1 / 2 |
| Response & recovery | 8 | 15 | 3 / 2 / 2 |
| Forensics & evidence | 7 | 15 | 4 / 2 / 2 |
| Network forensics & hardening | 11 | 15 | 5 / 4 / 0 |
| Threat hunting & containment | 20 | 15 | 0 / 0 / 4 |
| Web & TLS hardening | 15 | 15 | 2 / 0 / 0 |

**Programming**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Python and Go essentials *(first module)* | 6 | 20 | 6 / 4 / 4 |
| Python and Go in practice | 11 | 15 | 1 / 0 / 3 |
| Testing & tooling | 11 | 15 | 2 / 1 / 1 |
| Data structures & algorithms | 11 | 15 | 0 / 2 / 3 |
| Concurrency & I/O | 11 | 15 | 2 / 0 / 3 |
| Pitfalls & idioms | 21 | 15 | 0 / 0 / 4 |
| Secure coding | 15 | 15 | 2 / 0 / 0 |

**Data science**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Data wrangling with pandas *(first module)* | 8 | 20 | 5 / 2 / 5 |
| Statistics & machine learning | 7 | 15 | 4 / 2 / 2 |
| Features & evaluation | 11 | 15 | 2 / 2 / 0 |
| Text & time series | 11 | 15 | 2 / 0 / 2 |
| Pipelines & tuning | 11 | 15 | 1 / 1 / 2 |
| Analysis & model diagnostics | 20 | 15 | 0 / 0 / 3 |
| Customer & product analytics | 15 | 15 | 2 / 0 / 0 |

**Git**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Getting started *(first module)* | 11 | 20 | 0 / 6 / 5 |
| Branching and merging | 11 | 15 | 0 / 0 / 4 |
| Inspecting history | 11 | 15 | 0 / 3 / 4 |
| Undoing changes | 11 | 15 | 2 / 0 / 3 |
| Rebasing and rewriting | 11 | 15 | 5 / 3 / 0 |
| Stashing and tagging | 11 | 15 | 0 / 3 / 4 |
| Remotes and collaboration | 11 | 15 | 0 / 1 / 4 |
| Recovery and detective work | 11 | 15 | 5 / 3 / 0 |
| Plumbing and power tools | 11 | 15 | 2 / 0 / 3 |
| Real-world workflows | 11 | 15 | 5 / 2 / 0 |
| Configuration & aliases | 11 | 15 | 1 / 0 / 3 |
| Ignore rules & attributes | 11 | 15 | 1 / 2 / 1 |
| Hooks | 11 | 15 | 1 / 2 / 1 |
| Submodules & subtrees | 11 | 15 | 2 / 2 / 0 |
| Advanced merging | 11 | 15 | 4 / 0 / 2 |
| Searching & comparing | 11 | 15 | 2 / 0 / 2 |
| Large repositories | 11 | 15 | 1 / 0 / 3 |
| Repository troubleshooting | 11 | 15 | 0 / 2 / 2 |
| Identity, signing & trailers | 11 | 15 | 1 / 1 / 2 |
| Git incidents | 10 | 15 | 5 / 5 / 4 |
| Revisions, refs & scripting | 21 | 15 | 0 / 0 / 2 |
| Hosting & serving repositories | 15 | 15 | 2 / 0 / 0 |

**Terraform**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Terraform basics *(first module)* | 11 | 20 | 0 / 4 / 5 |
| Variables, expressions & meta-arguments | 11 | 15 | 0 / 1 / 3 |
| Working with state | 11 | 15 | 3 / 0 / 1 |
| Modules | 11 | 15 | 2 / 1 / 1 |
| Docker infrastructure | 11 | 15 | 2 / 0 / 3 |
| Functions, lifecycle & testing | 21 | 15 | 0 / 0 / 4 |
| Terraform in automation | 16 | 15 | 2 / 0 / 0 |

**Ansible**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Inventory & ad-hoc commands *(first module)* | 11 | 20 | 1 / 4 / 4 |
| Playbooks | 11 | 15 | 0 / 1 / 4 |
| Templates, files & variables | 11 | 15 | 0 / 0 / 4 |
| Roles & reuse | 11 | 15 | 2 / 0 / 2 |
| Operations & incidents | 10 | 15 | 3 / 2 / 0 |
| Control flow & extending Ansible | 20 | 15 | 0 / 0 / 2 |
| Data structures & types | 15 | 15 | 2 / 0 / 0 |

**Algorithms**

| Module | Now | Target | Still needed (B / I / A) |
|---|---|---|---|
| Searching & sorting *(first module)* | 11 | 20 | 1 / 3 / 5 |
| Data structures | 11 | 15 | 0 / 1 / 4 |
| Graphs | 11 | 15 | 1 / 0 / 3 |
| Dynamic programming | 11 | 15 | 0 / 1 / 4 |
| Strings, greedy & maths | 11 | 15 | 0 / 3 / 3 |
| Recursion & backtracking | 15 | 15 | 2 / 0 / 0 |

## Gaps in Kubernetes, Terraform and Git

Added 2026-11-21 after a coverage review of the three tracks. Not started. Where an idea
already has a row elsewhere in this file, the row here points to it rather than repeating it.

- **Git** is close to complete: everything from first commits to plumbing, hooks, signing and
  hosting. The gaps are small.
- **Kubernetes** has the core done (all of CKA and CKAD, plus Helm, cert-manager, Gateway
  API and BGP). What's missing is the tooling most production clusters run on top.
- **Terraform** is the furthest from done: every exercise uses local or Docker providers, while
  real Terraform work is mostly cloud infrastructure and shared remote state.

Suggested order: Terraform's AWS provider and remote backends first (the biggest real-world gap),
then GitOps for Kubernetes, then Git LFS.

### Terraform gaps

| Gap | The lab | Runs on | Notes |
|---|---|---|---|
| **The AWS provider against Moto** | Done in `exercises/terraform/08-aws.yaml` (tf-093..103) | | |
| **Remote state backends with locking** | Done in `exercises/terraform/09-remote-state.yaml` (tf-104..110): S3 with `use_lockfile`, migrate, stale locks, partial config, versioned recovery, the `pg` backend, a shared key, and a capstone | | |
| **Provider aliases** | Done in `exercises/terraform/08-aws.yaml` (tf-093..103) | | |
| **Ephemeral values and write-only arguments** | Done in `exercises/terraform/08-aws.yaml` (tf-093..103) | | |
| **OpenTofu differences** | Done in `exercises/terraform/10-opentofu-terragrunt-policy.yaml` (tf-111..117) | | |
| **Policy checks on plans** | Done in `exercises/terraform/10-opentofu-terragrunt-policy.yaml` (tf-111..117) | | |
| **Terragrunt** | Done in `exercises/terraform/10-opentofu-terragrunt-policy.yaml` (tf-111..117) | | |
| **Plan and apply from pull requests** | Done in `exercises/terraform/11-pull-requests.yaml` (tf-118..120), on Gitea | | |

### Kubernetes gaps

| Gap | The lab | Runs on | Notes |
|---|---|---|---|
| **GitOps with Argo CD or Flux** | Done with Argo CD in `exercises/kubernetes/24-gitops-argocd.yaml` (k8s-394..401), on an in-cluster Gitea | | |
| **Prometheus and Grafana in the cluster** | Prometheus Operator part done in `exercises/kubernetes/26-prometheus-operator.yaml` (k8s-410..417). Still open: a Grafana dashboard with no data (the image is already cached) | k8s lab | See *Observability* (Cloud & platform) and *Prometheus and alerting* (Shared lab foundations) |
| **Policy engines** | Kyverno done in `exercises/kubernetes/25-kyverno.yaml` (k8s-402..409). Still open: Gatekeeper constraint templates, Kyverno image verification (see *Supply chain*) | k8s lab | Built-in CEL policies are covered already |
| **CKS-level security** | AppArmor profiles on pods, custom seccomp profiles from the kubelet's directory, Falco catching a shell in a container, cosign-signed images with signature checks at admission, Trivy scans, kube-bench findings | k8s lab | seccomp has two exercises so far. See *Supply chain* (Security) |
| **Autoscaling beyond the HPA** | VPA recommendations, an HPA on a custom metric (prometheus-adapter), scaling from zero | k8s lab | KEDA already has a row in *Even more ideas* |
| **Volume snapshots and expansion** | Snapshot a PVC, restore it to a new claim, clone a volume, grow one that's full | k8s lab; csi-driver-host-path | local-path supports neither, so this needs a CSI driver |
| **Secrets from outside the cluster** | External Secrets Operator syncing from Vault, a sync that silently stopped; or Sealed Secrets | k8s lab; Vault | See *Secrets management* (Security) |
| **Write your own controller** | A small operator with controller-runtime (Go) or kopf (Python): the reconcile loop, finalizers, status conditions, a controller that hot-loops | k8s lab | CRDs are covered; nothing writes a controller yet |
| **Already queued** | | | *Linkerd service mesh* and *Cluster backup with Velero* in *Next round*; *Canary releases with Argo Rollouts* in *Even more ideas* |
| **Smaller** | kube-proxy in IPVS or nftables mode, dual-stack Services | k8s lab | |

### Git gaps

| Gap | The lab | Runs on | Notes |
|---|---|---|---|
| **Git LFS** | Done in `exercises/git/23-lfs.yaml` (git-256..262), on a local Gitea | | |
| **Credential helpers and HTTPS auth** | Done in `exercises/git/24-credentials.yaml` (git-263..268) | | |
| **Pull-request workflows** | Done in `exercises/git/25-pull-requests.yaml` (git-269..275), on Gitea | | |

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
| **Container internals** | Done in `exercises/linux/07-container-internals.yaml`: namespaces, cgroups, overlay layers, `runc`, user namespaces, capabilities, mount propagation, PID 1. Still to do: `ctr` and `crictl` against containerd | Docker VM or k8s nodes |
| **QEMU & virtualization** | See below | Docker VM or a dedicated VM; needs nested KVM |
| **Hardware health** | See below: failing RAM, CPUs, disks, motherboards and power supplies, found from Linux | Docker VM, a QEMU guest and recorded data |

Linux admin relies on other tracks for two topics: routing, DNS and firewalls are in
Networking and DNS & email, and boot failures are built with the QEMU rescue module.

### Build your own container runtime

Added 2026-11-21. Not started. *Container internals* (`exercises/linux/07-container-internals.yaml`)
builds a container **by hand**, one command at a time. This module turns that into a **program**:
a small Docker-like tool, `/lab/minibox`, written in Go or Python (or Bash for the first labs),
that grows a feature per lab. Each lab's check runs the learner's tool, as root on the Docker VM,
and inspects the result from outside: `/proc/PID/ns/*`, `/sys/fs/cgroup`, `ip netns`, mounts.
Every check also runs the earlier labs' tests, so a later feature can't break an earlier one.

| Level | Lab | What `minibox` learns to do |
|---|---|---|
| Beginner | **`run` = fork and exec** | `minibox run /bin/echo hi`: run a command, pass its exit code back, forward signals (Ctrl-C reaches the child) |
| Beginner | **Its own hostname and PIDs** | New UTS and PID namespaces (`clone`/`unshare` flags): the command sees itself as PID 1 and a hostname given with `--hostname` |
| Beginner | **Its own root filesystem** | `--rootfs DIR` from an exported busybox or alpine image: a mount namespace, `pivot_root` (not just `chroot`, and why), mounting `/proc` inside |
| Beginner | **Images from a tarball** | `minibox pull` stand-in: unpack an image saved with `docker save` (manifest, layer tarballs) into a rootfs, whiteout files included |
| Beginner | **Memory and process limits** | `--memory 64m --pids 20`: create a cgroup v2 group, write `memory.max` and `pids.max`, move the child in before it execs; the check runs a memory hog and a fork bomb |
| Intermediate | **CPU limits and accounting** | `--cpus 0.5` via `cpu.max`, `minibox stats` reading `memory.current`, `cpu.stat` and `pids.current` |
| Intermediate | **Overlay layers** | Several layers + a writable upper dir with `overlayfs`; changes stay out of the image, `--rm` removes the upper dir |
| Intermediate | **Networking** | `--net bridge`: a network namespace, a veth pair onto a `minibox0` bridge, an address from a small pool, NAT out with nftables, `-p 8080:80` as a DNAT rule |
| Intermediate | **Volumes and environment** | `-v HOST:CTR` bind mounts (read-only too), `-e KEY=VAL`, `-w DIR`, running as a non-root `--user` |
| Intermediate | **Detached containers** | `run -d`, `ps`, `logs`, `stop` (SIGTERM, then SIGKILL after a timeout), `rm`: state kept in `/var/lib/minibox/ID/` |
| Advanced | **Dropping privileges** | Drop capabilities (`capset`, the bounding set), `no_new_privs`, a read-only root, masked `/proc` paths; the check tries the escapes that should now fail |
| Advanced | **A seccomp filter** | Block a list of syscalls with a BPF filter (libseccomp or a hand-built filter), so `unshare` or `mount` inside returns `EPERM` |
| Advanced | **Rootless** | User namespaces with `uid_map`/`gid_map` (and `newuidmap`), so an ordinary user can run it; what breaks without root (cgroup delegation, networking) |
| Advanced | **`exec` into a running container** | `minibox exec ID cmd` with `setns` into each namespace, joining its cgroup too |
| Capstone | **Run a real image** | Run a two-container app (nginx in front of a small Python service) from saved images with limits, a private network, a published port and a volume, then `stop` and `rm` without leaking a mount, cgroup, veth or rule (the check looks for leftovers) |

- **Language:** Go is the natural fit (`syscall.SysProcAttr` has `Cloneflags`, `Unshareflags`,
  `UidMappings`; it is what runc and Docker are written in) and runs from the pinned
  `golang:1.24-alpine` image already on the VM. Python works with `os.unshare`, `os.setns`,
  `ctypes` for `pivot_root`/`mount`, and needs nothing new. Bash with `unshare`, `nsenter`
  and `ip` covers the first labs. A `go_test`-style helper builds the tool and runs the
  check's scenarios against it.
- **Tutorials** carry the full picture once, in the first lab, as a diagram of what
  `docker run` does (image → rootfs → namespaces → cgroup → network → exec), and each later
  lab points to the step it adds. They link back to the *Container internals* lab that did
  the same step by hand (linux-050 rootfs, linux-051 cgroups, linux-052 layers, linux-053
  OCI config, linux-054 user namespaces, linux-055 capabilities).
- **Cleanup:** cgroups under `/sys/fs/cgroup/minibox/`, the `minibox0` bridge, veths,
  nftables table `minibox`, overlay mounts and `/var/lib/minibox` must all be removed on
  reset (`on_reset`), since a learner's buggy tool will leak them.
- **Grows into:** an OCI `config.json` mode that makes `minibox` a drop-in for `runc` (the
  check runs it under containerd or Docker as a custom runtime), and a tiny image registry
  client that pulls from the lab's `registry:2`.

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

### Hardware health modules

Added 2026-11-21. Not started. These labs teach how to tell, from inside Linux, whether a server's
RAM, CPU, disks, motherboard or power supply is failing, and which part to replace. Lab VMs
have no real hardware to break, so the faults come from three places, and each one is read
with the real tools:

1. **Emulated hardware that fails on cue.** This uses a guest from the QEMU modules. The
   monitor's `mce` command injects machine-check errors into a CPU, and `pcie_aer_inject_error`
   raises a PCIe error. `blkdebug` returns I/O errors at chosen sectors or events. Setting an
   NVMe drive's `smart_critical_warning` property over QMP raises a SMART warning. A
   simulated BMC gives `ipmitool` sensors, an event log and FRU data. The BMC is OpenIPMI's
   `ipmi_sim`, whose sensor values can be scripted, or QEMU's `ipmi-bmc-sim` for the
   in-guest KCS path.
2. **Kernel fault injection in the VM itself.** `scsi_debug` disks have medium errors at
   chosen sectors, and `dm-dust` and `dm-flakey` act as disks with bad blocks or ones that
   come and go. `madvise(MADV_HWPOISON)` or `hwpoison-inject` poisons a memory page. A CPU
   can be hot-unplugged through `/sys/devices/system/cpu/cpuN/online`.
3. **Recordings from real machines,** replayed by the real tools where they can read a
   recording: `dmidecode --from-dump` and `lspci -F`. Where they can't, generated
   `smartctl -x --json`, `sensors`, `edac-util` output, `rasdaemon` databases and kernel logs
   stand in. The faulty DIMM, slot or disk changes on every run, so the answer can't be
   memorised.

| Level | Lab | What you practise |
|---|---|---|
| Beginner | **Inventory a server** | Write a hardware report with `lscpu`, `lsmem`, `dmidecode -t bios,baseboard,memory`, `lspci`, `lsblk -o +MODEL,SERIAL`, `lshw -short` and `inxi`. It covers the board and BIOS version, each DIMM's slot, size, speed and part number, the empty slots, and the disks with serials. The check compares it with the dump |
| Beginner | **Hardware errors in the kernel log** | `dmesg -T --level=err,warn` and `journalctl -k -b -1`. Pick out the real hardware lines (MCE, EDAC, `ata`/`nvme` timeouts, AER, thermal) from driver noise and red herrings that have been there for months |
| Beginner | **SMART basics** | Read `smartctl -x` and `nvme smart-log`, and learn which attributes matter: 5 reallocated, 197 pending, 198 offline uncorrectable, NVMe media errors, percentage used and the critical warning. Attribute 199 (CRC errors) points to the cable, not the disk. Of several disks, say which to replace, which needs a new cable and which is fine |
| Beginner | **Temperatures, fans and voltages** | `sensors`, plus `ipmitool sdr` and `ipmitool sensor` against the simulated BMC. Spot a fan below its lower-critical threshold and a 12 V rail out of range, and learn what the thresholds mean |
| Beginner | **Is the CPU throttling?** | `lscpu`, `cpupower frequency-info`, a `turbostat` recording and the `thermal_throttle` counters. Tell heat from a `powersave` governor or a power cap, which are the more usual causes |
| Intermediate | **Which DIMM do I pull?** | EDAC's per-DIMM `dimm_ce_count` and `dimm_label`, `ras-mc-ctl --summary` and `--errors`, then mapping the controller, channel and slot to the `dmidecode` locator and the label printed on the board. One DIMM's corrected errors keep rising while another had one error a year ago. The answer is the slot label |
| Intermediate | **Testing memory** | `memtester` (it can't test memory the kernel is using), `stress-ng --vm --verify`, and `memtest=` on a guest's kernel command line. Know what a pass does and doesn't prove |
| Intermediate | **A poisoned memory page** | Poison a page under a running process. Read the `Memory failure: … recovery action` line, `HardwareCorrupted` in `/proc/meminfo`, and the process's `SIGBUS`. Compare soft-offlining and hard-offlining a page |
| Intermediate | **Machine checks** | Inject MCEs into an x86 guest with QEMU's `mce` command and decode them with `rasdaemon` or `mcelog`. Tell corrected from uncorrected errors, find the bank, CPU and socket, and tell a cache error from a memory-controller error |
| Intermediate | **Bad sectors** | A `dm-dust` or `scsi_debug` disk with bad blocks. Run read-only `badblocks -sv` and a `smartctl -t short` self-test, find which file sits on the bad sector with `debugfs icheck`/`ncheck`, and save the rest of it with `ddrescue` |
| Intermediate | **Flaky, not dead** | `dm-flakey` or `blkdebug` timeouts show up as link resets and timeouts in the log, latency spikes in `iostat -x`, and md RAID failing a member. Use the pattern to decide between disk, cable and controller, such as CRC errors, or every disk on one controller failing together |
| Intermediate | **PCIe errors and slow links** | A NIC at x1 2.5 GT/s in an x8 slot (`lspci -vvv` `LnkCap` vs `LnkSta`), correctable and uncorrectable AER errors from `pcie_aer_inject_error`, and the `aer_dev_*` counters in sysfs |
| Intermediate | **The BMC's event log** | `ipmitool sel elist`, `chassis status` and `fru`. Rebuild an overnight power-supply failure and reboot from the event log, find the board serial for the RMA, and save the log before clearing it |
| Advanced | **Alert before it breaks** | `smartd.conf` with scheduled self-tests and an `-M exec` alert, `rasdaemon` running, and an alert on a rising corrected-error rate from `smartctl_exporter` or the node_exporter textfile collector. The check injects a fault and the alert must fire |
| Advanced | **Burn in a new server** | A pass/fail script that runs `stress-ng` and `fio --verify` and watches temperatures and the error counters while they run. The check runs it on a guest with a hidden fault, and it must fail |
| Advanced | **Firmware and microcode** | The BIOS version against an advisory, `microcode` in `/proc/cpuinfo` and the early-load log line, `/sys/devices/system/cpu/vulnerabilities/*`, and `fwupdmgr get-devices` and `get-updates` against fwupd's test plugin. Nothing real is flashed |
| Advanced | **Hardware or software?** | Random segfaults and corrupted downloads across unrelated programs point to RAM. One binary crashing at the same code address points to a bug. Decide which from the logs, `coredumpctl` and checksums |
| Capstone | **Triage a dying server** | A sosreport-style bundle and a live guest with several injected faults and red herrings. Write the RMA ticket: the parts (DIMM slot label, disk serial, PSU), the evidence for each, and what to leave alone. The check grades the parts and the evidence |

- **Architecture:** Apple Silicon hosts run arm64 VMs. MCE, `mcelog`, `turbostat` and
  `thermal_throttle` are x86, so those labs use a small x86 QEMU guest under software
  emulation, or recordings. Slow is fine there: one kernel and an initramfs boot in seconds.
  On arm64, memory errors arrive through APEI/GHES, so the recordings cover both.
- **Packages:** `smartmontools`, `nvme-cli`, `ipmitool`, OpenIPMI, `lm-sensors`, `rasdaemon`,
  `edac-utils`, `dmidecode`, `pciutils`, `memtester`, `stress-ng`, `fio`, `gddrescue` and
  `fwupd`. `scsi_debug` and `dm-dust` need `linux-modules-extra` on the Ubuntu VM kernel, and
  hwpoison needs `CONFIG_MEMORY_FAILURE`. Check both at bootstrap.
- **Recordings:** build a small library from real machines with `dmidecode --dump-bin` and
  `lspci -xxxx` (`-F` needs the full config space). Scrub the serials and MACs, and generate
  variants with the fault moved.
- **Answers:** the learner writes `/lab/answer.yaml` (slot, serial, part, and evidence for the
  capstone). This ties into the *Critical-thinking labs*.
- **Safety:** tutorials say which commands harm real hardware or destroy evidence:
  `badblocks -w`, `hdparm --write-sector`, a long self-test on a busy disk, and clearing the
  event log. The labs only run them on emulated devices.
- **Relies on:** the QEMU modules for the guest and QMP helpers, the `mdadm` RAID idea, and
  *Prometheus and alerting* for the monitoring lab.

## Networking

| Track | What you'd practise | Runs on | Relies on |
|---|---|---|---|
| **Networking & IP routing** | Addressing to BGP, firewalls, packet analysis | Docker VM; k8s lab for Calico and MetalLB | Nothing; the foundation |
| **TLS & PKI** | Done in `exercises/networking/10-tls-and-pki.yaml`: keys and CSRs, SANs and wildcards, PKCS#12 and DER, SNI, picking intermediates, the system trust store, key pinning, `openssl ca` with a CRL, name constraints, OCSP, dual RSA/ECDSA certificates, root rollover with a cross-signed root. cert-manager is done in `exercises/kubernetes/23-cert-manager.yaml` | Docker VM; k8s lab for cert-manager | Nothing |
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
| **Infrastructure as code** | Terraform started in `exercises/terraform/` (basics, expressions, state, modules, Docker). Ansible started in `exercises/ansible/` (inventory, playbooks, templates & vault, roles, operations) against kl-node containers. Still to do: Ansible against the real lab VMs and k8s nodes, Molecule-style role testing; the AWS provider against moto (the provider is ~600 MB, so it needs its own mirror image); CI/plan review workflows | Existing VMs; light |
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
- With Terraform's AWS provider 6, Moto 5.2.4 drops the tags sent when a bucket is created
  (they go in CreateBucket and are read back through S3 Control), so a tagged `aws_s3_bucket`
  never plans clean. Keep tags off buckets in Terraform labs; tags on other resources work.
- Terraform's wrapper (`tf_aws_start`) uses `http://localhost:5000` plus a hosts entry for
  `123456789012.localhost`: S3 Control puts the account id in front of the host name, and Moto
  reads an unknown host's first label as a bucket name.

## Data

| Track | What you'd practise | Runs on |
|---|---|---|
| **Databases & SQL** | See below | Docker VM; easy to check |
| **Messaging & caching** | Redis persistence and eviction, RabbitMQ queues and dead letters. Replica resyncs, AOF repair and Sentinel split-brain are done in `data-053` to `data-055` | Docker VM; Kafka would be heavier |

### Databases & SQL modules

| Module | Exercises |
|---|---|
| **SQL** | Joins and window functions |
| **Query performance** | Reading query plans, fixing slow queries with indexes, locks |
| **Backup & restore** | Point-in-time restore is done in `data-046` |
| **Replication** | Done in `data-047` to `data-049`: a lagging replica and its slot, failover, stalled synchronous commits |
| **Migrations** | A failed migration is done in `data-052` |

### Statistics modules (Data science track)

Added 2026-11-21. Not started. A **statistics ladder from zero to advanced** in the
`datascience` track (Jupyter notebooks, like the rest of it): eight modules of 15 (the first
one 20), about 125 exercises, which takes the track past its 250 target. Today statistics only
appears in passing (`ds` outliers, the A/B test, the bootstrap median, chi-square, Simpson's
paradox, Spearman); those stay where they are, and the new modules point back to them.

**Every exercise teaches with a worked example.** The tutorial works through the idea on a
small dataset of its own (6–10 numbers, small enough to follow by hand), with the arithmetic
written out, then does the same in pandas/scipy/statsmodels and shows that the numbers match.
Then it gives the reading: what the result means in a sentence a manager would understand, and
the wrong conclusion people usually draw. The exercise then uses a different, per-run dataset,
so the example never gives the answer away. Checks compare numbers with a tolerance, ask for
the decision as well as the statistic ("ship / don't ship", "can't tell yet"), and where it
fits, run the learner's function on hidden data.

| Module | Labs, beginner → advanced |
|---|---|
| **Describing data** *(first module)* | Mean vs median when one value is huge · spread: range, IQR, variance and standard deviation (why `n-1`) · percentiles and the box plot's five numbers · skew, and which average to report · z-scores to compare across scales · frequency tables and histograms with honest bins · weighted averages (a mean of means that is wrong) · grouped summaries · correlation, and what a correlation of 0 still allows (Anscombe's quartet) · *capstone:* summarise a salary survey without misleading anyone |
| **Probability** | Counting and simple probability · conditional probability from a two-way table · Bayes' rule on a medical test (the base-rate trap) · independence, checked rather than assumed · expected value and variance of a game · simulation to check an answer you derived (Monte Carlo) · the birthday problem · *capstone:* is the fraud alert worth acting on? |
| **Distributions** | Binomial (defects in a batch) · Poisson (calls per hour) and when it fails (overdispersion) · normal, and the 68-95-99.7 rule · exponential waiting times · log-normal incomes and response times · fitting a distribution and checking the fit (Q-Q plots, KS test) · the central limit theorem by simulation · heavy tails, where the mean misleads · *capstone:* capacity planning from request-latency data |
| **Estimation & confidence** | Sampling variability by simulation · standard error vs standard deviation · confidence intervals for a mean and a proportion (Wilson, not the textbook one, for small counts) · what 95% really means (simulated coverage) · the bootstrap for any statistic · sample size for a target margin of error · biased samples that no sample size fixes (survivorship, non-response) · *capstone:* an election poll with weights |
| **Hypothesis testing** | Null, alternative and p-value, built from a permutation test first · one- and two-sample t-tests and their assumptions · paired vs unpaired (before/after) · proportions test · chi-square goodness of fit · non-parametric tests (Mann-Whitney, Wilcoxon) · ANOVA and post-hoc comparisons · effect size next to the p-value · multiple comparisons (Bonferroni, Benjamini-Hochberg) · p-hacking by simulation · *capstone:* review a colleague's "significant" findings |
| **Experiments & A/B testing** | Randomisation and units of analysis · power and sample size before you start · minimum detectable effect · peeking stops a test too early (simulated false-positive rate) · sequential testing done right · a sample ratio mismatch that invalidates a test · metrics as ratios (delta method) · CUPED variance reduction · novelty effects over time · guardrail metrics · *capstone:* decide on a pricing experiment with three variants |
| **Regression** | Simple linear regression by hand and with statsmodels · reading the coefficient table (estimate, SE, CI, p) · residual plots and the assumptions · categorical predictors and dummy coding · interactions · confounding: a coefficient that flips when a variable is added · multicollinearity (VIF) · transformations (log-log as elasticity) · logistic regression and odds ratios · Poisson regression for counts · robust standard errors · *capstone:* what drives delivery time, explained to operations |
| **Bayesian & causal** *(advanced)* | Beta-binomial updating of a conversion rate · priors and how much they matter · credible vs confidence intervals · Bayesian A/B: probability B beats A and the expected loss · hierarchical shrinkage (ratings with few votes) · a tiny Metropolis sampler in NumPy · correlation vs causation with a DAG · difference-in-differences · propensity scores and matching · regression discontinuity · instrumental variables · *capstone:* did the loyalty programme really raise spend? |

- **Tooling:** numpy, pandas and scipy are already in `kl-datascience:1`. Add **statsmodels**
  (regression tables, ANOVA, proportion tests, power) pinned in the image; optional:
  matplotlib for plots in tutorials (already in the Jupyter image). The image change needs an
  `env rebuild docker`, or a pinned pip install on first use like `tshark_ready`, so saved VM
  copies don't go stale.
- **Data:** generated in setup with a per-run seed from known parameters, so the check knows
  the true answer (the real effect, the real distribution) and can also grade "can't tell
  yet" when the data can't settle it.
- **Learning path:** a `statistics` path in `learning-paths.yaml` across the eight modules,
  plus the existing ds labs that use statistics.

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

## Reverse engineering

A new track (`reversing`) on taking apart what you only have in compiled form: binaries,
firmware images and ROMs. Everything it analyses is **built by the lab itself** — small
programs compiled in setup, firmware images assembled from open-source parts, homebrew ROMs
assembled from source — so no copyrighted ROM or vendor firmware is ever shipped or needed.
Same shape as every other track: 15-exercise modules, beginner → advanced, each ending
with a capstone.

| Module | What you'd practise | Runs on |
|---|---|---|
| **Binary basics** | `file`, `strings`, `readelf`/`objdump`, sections and symbols, `ldd`, `strace`/`ltrace`; find a hard-coded key or URL in a stripped binary | Docker VM; light |
| **Disassembly & decompilation** | x86-64 calling conventions, reading control flow in radare2/rizin and Ghidra (headless), renaming and annotating, finding the check a crackme makes | Docker or offsec VM; Ghidra is the heavy part |
| **Dynamic analysis** | gdb (+ pwndbg/gef), breakpoints and watchpoints, patching registers at runtime, `LD_PRELOAD` hooks, Frida on a running process | Docker VM |
| **Patching & keygens** | patch a jump to bypass a licence check, write a key generator from the reversed algorithm, binary diffing (`radiff2`, BinDiff-style) to find what a patch fixed | Docker VM |
| **Firmware analysis** | `binwalk`/unblob extraction, squashfs/cramfs/jffs2, U-Boot and uImage headers, recovering hard-coded credentials and keys, running extracted services under `qemu-user` (ARM/MIPS), repacking a modified image | Docker VM (`qemu-user-static`) |
| **ROMs & embedded** | file formats and headers (iNES, Game Boy cartridge header and checksums), 6502/Z80/ARM Thumb disassembly, memory maps, emulators with debuggers (e.g. mGBA's gdb stub), patching a homebrew ROM and fixing its checksum, IPS/BPS patches | Docker VM |
| **Malware-style analysis** | triage a (harmless, lab-built) sample: unpacking UPX, string decryption, config extraction, YARA rules and `capa` to describe what it does — the defensive side of the same skills | Offsec VM; ties into defensive security |
| **Protocols & file formats** | reverse an undocumented binary file format or network protocol from samples and a client binary (Kaitai Struct, Wireshark dissectors in Lua) and write a parser that the check feeds new samples | Docker VM; ties into networking |

- **Checks** stay concrete: a recovered flag or key in `/lab`, a patched binary or ROM that
  now behaves differently (the check runs it, or runs the ROM headless in the emulator with
  a scripted input), a keygen whose keys the original binary accepts, a parser that handles
  the check's own hidden samples, or a YARA rule that matches the hidden variants and none
  of the clean files.
- **Determinism:** setup compiles the targets with fixed flags (and `-ffile-prefix-map`,
  no timestamps) or picks per-run keys stored in `$KL_STATE`, so answers can't be looked up.
- **Tooling to add to the image:** binutils, gdb + gef, radare2/rizin, `qemu-user-static`,
  binwalk/unblob, squashfs-tools, UPX, YARA, `capa`, cross toolchains for ARM/MIPS, and the
  homebrew assemblers (cc65 for NES, RGBDS for Game Boy). Ghidra (headless, ~1 GB with its
  JDK) is optional: only the decompilation module needs it, so it could live in its own
  image pulled on first use.
- **Start with** *Binary basics* and *Firmware analysis* — they need nothing heavier than
  the Docker VM and teach the triage every later module builds on.

### Command-line disassemblers

Added 2026-11-21. Not started. A from-zero ladder for **reading disassembly in a terminal**, to
come before *Disassembly & decompilation* (which assumes it). Each lab gives a small program that
setup compiles with per-run constants, so the answer is a value read out of the machine code, not
looked up.

| Level | Lab | What it teaches |
|---|---|---|
| Beginner | **Your first `objdump -d`** | `objdump -d --no-show-raw-insn`, `-M intel`, finding `main`, matching a 5-line C function to its instructions; answer: the constant it adds |
| Beginner | **Sections and symbols** | `objdump -h`/`-t`, `readelf -S -s`, `.text` vs `.rodata` vs `.data`, `nm`; find which function a string belongs to |
| Beginner | **Calls and the calling convention** | Arguments in registers (`rdi`, `rsi`… / `x0`, `x1`…), return values, `call`/`ret`, the PLT: which libc functions are called, with what |
| Beginner | **Branches and loops** | `cmp`/`test` + `jcc` (or `cmp` + `b.cond`), reading an `if` and a `for` loop back into pseudo-code; answer: the loop's bound |
| Intermediate | **`gdb` as a disassembler** | `disassemble`, `x/20i $pc`, `layout asm`, `break *ADDR`, `info registers`, stepping with `si`/`ni`; read a value computed at run time |
| Intermediate | **radare2 / rizin basics** | `aaa`, `afl`, `pdf @ main`, `iz`, `axt` (cross-references), visual mode `V`/`VV`; find the function that uses a string |
| Intermediate | **Stripped binaries** | No symbols: find `main` from `_start`/`__libc_start_main`, name functions by what they call, `strings -t x` and xrefs to anchor |
| Intermediate | **Raw blobs** | Bytes with no ELF header: `objdump -D -b binary -m i386:x86-64` (or `aarch64`), `ndisasm`, `r2 -a x86 -b 64`, find where code starts |
| Advanced | **Optimised code** | Compare `-O0` and `-O2` output of the same function: inlining, strength reduction (`lea` tricks, multiply-by-reciprocal for division), tail calls |
| Advanced | **Switches and jump tables** | A `switch` compiled into a table in `.rodata`, follow it by hand; answer: which input reaches which case |
| Advanced | **Patch it from the command line** | Change one conditional jump with `r2 -w` (`wa`/`wx`) or `printf | dd`, prove the binary's behaviour changed |
| Capstone | **The crackme** | A stripped, optimised password checker: reverse the check with only terminal tools and write the password that the original binary accepts (per-run algorithm constants) |

- **Architecture:** the lab VMs are **arm64** on Apple Silicon. Either teach AArch64 first
  (native, runnable), or cross-compile x86-64 with `gcc-x86-64-linux-gnu`, disassemble with
  `binutils-multiarch` (`objdump` for any architecture) and run with `qemu-user-static`. Best
  is both: every lab ships the same program for both architectures and the tutorial shows
  the two side by side, since x86-64 is what most guides and CTFs use.
- **Tooling:** binutils, `gdb` (with `gdb-multiarch`), radare2 or rizin (pin one; their
  commands differ slightly), `nasm` for `ndisasm`. All are light and fit on the Docker VM.
- **Checks:** an answer file with a value read from the code, a pseudo-code answer replaced
  by "what does it return for input X" (the check runs the binary with hidden inputs), or a
  patched binary the check runs.

## Active Directory on Linux

Real Active Directory skills without a Windows licence: a **Samba AD domain controller** in
containers (or on the offsec VM), with Linux members joined to it. The same domain serves an
administration module, an attack module and a defence module, so it feeds the offensive and
defensive security tracks as well as Linux.

| Module | What you'd practise | Runs on |
|---|---|---|
| **Stand up a domain** | provision a Samba AD DC (`samba-tool domain provision`), AD-integrated DNS and SRV records, users, groups and OUs, a second DC and replication between them | Docker VM; light |
| **Join and log in** | `realm join` / `sssd` on Linux members, Kerberos tickets with `kinit`/`klist`, keytabs for services, `sudo` and SSH by AD group, fixing "clock skew too great" and a broken `krb5.conf` | Docker VM |
| **Directory & policy** | LDAP queries (`ldapsearch`, `samba-tool`), password and lockout policies, delegated admin rights on an OU, Group Policy as far as Samba applies it, service accounts | Docker VM |
| **Attacking the domain** | enumeration with BloodHound-style queries, kerberoasting and AS-REP roasting a weak service account, abusing unconstrained delegation and an over-broad ACL, pass-the-ticket — all inside the lab's own domain | Offsec VM |
| **Defending the domain** | find the attack in the DC's logs and Kerberos events, tier the admin accounts, fix the delegation and ACLs, set strong service-account passwords or gMSA-style rotation, prove each attack path is now closed | Offsec VM; ties into defensive security |
| **Troubleshooting** | a member that can't find the DC (DNS), replication that stopped, an expired machine password, SPN collisions, a trust that broke | Docker VM |

- **Checks** use the domain itself: `kinit` as a user succeeds or fails, `samba-tool`/`ldapsearch`
  show the object or ACL, a member resolves and authenticates, the attack tool no longer gets
  a crackable ticket, a captured flag sits behind a group the learner had to reach.
- **Determinism:** setup provisions the domain from a script with fixed names and per-run
  passwords stored in `$KL_STATE`; weak passwords for the roasting labs come from the lab's
  own wordlist.
- **Tooling to add:** `samba` (AD DC), `krb5-user`, `sssd`/`realmd`, `ldap-utils`, and for the
  attack module Impacket and a BloodHound-compatible collector. A Samba DC fits in about 512 MB,
  so a DC, a second DC and two members run in containers on the Docker VM.
- **Start with** *Stand up a domain* and *Join and log in*; the attack and defence modules
  reuse that domain.

## Industrial & IoT

The protocols that run factories, buildings and devices, simulated in containers and on
virtual interfaces, so it runs fully offline. Each module first builds or operates the
system, then attacks it on the lab's own network, then secures it.

| Module | What you'd practise | Runs on |
|---|---|---|
| **MQTT & device fleets** | a Mosquitto broker, topics and retained messages, QoS levels, ACLs per device, TLS with client certificates, a device that floods the broker, bridging two brokers | Docker VM; light |
| **Modbus & PLCs** | read and write coils and registers on a simulated PLC (OpenPLC or a Python Modbus server) with `mbpoll`/pymodbus, find the register that opens a valve, then segment the network so only the HMI can write | Docker VM |
| **OPC UA & SCADA** | browse an OPC UA server's address space, subscribe to values, secure it with certificates and user roles, find an HMI that talks to it unencrypted | Docker VM |
| **CAN bus & vehicles** | `vcan` interfaces with `can-utils` (`candump`, `cansniffer`, `cansend`), reverse which ID drives the speedometer on a simulated car network (ICSim-style), replay and fuzz frames, then add message authentication | Docker VM (`vcan` kernel module) |
| **Industrial network forensics** | Modbus, MQTT and CAN traffic in captures: find the write that changed a setpoint, the rogue device on the bus, the replayed command | Offsec VM; ties into packet capture labs |
| **Securing an OT network** | zones and conduits, a one-way flow from OT to IT, an allow-list firewall for industrial protocols, monitoring that alerts on a write from the wrong host | Docker VM; ties into networking |

- **Checks** read the simulated process itself: a register or coil holds the right value, the
  simulated valve is closed, a write from the attacker's host now fails while the HMI's still
  works, the CAN frame that unlocks the simulated car is in `/lab`, a broker refuses a device
  without a valid certificate.
- **Determinism:** the simulated devices are small Python services with per-run register maps
  or CAN IDs stored in `$KL_STATE`, so answers can't be looked up.
- **Tooling to add:** Mosquitto, pymodbus/`mbpoll`, an OPC UA library (asyncua), `can-utils`
  and the `vcan` module (check that the VM kernel ships it, as `sch_netem` was checked), and
  Wireshark dissectors already in tshark.
- **Start with** *MQTT & device fleets* and *Modbus & PLCs*: light, visual and easy to check.

### ESP32 without external sensors

Added 2026-11-21. Not started. ESP32 labs that need **no sensors or wiring**: only what is on the
chip, and for the graded part no board at all.

**Graded labs in an emulator (on the Docker VM).** Espressif's QEMU fork emulates the ESP32,
ESP32-C3 and ESP32-S3, and `idf.py qemu` runs firmware in it: flash, eFuses and Ethernet
networking (openeth) work, **Wi-Fi and Bluetooth don't**. Checks read the serial console, the
flash image, the eFuse file and the device's network services, so they run unattended like
every other track.

| Module | Labs |
|---|---|
| **ESP-IDF & FreeRTOS from zero** | Build and run in QEMU, tasks pinned to two cores, queues and event groups, a mutex that deadlocks, a stack overflow the task watchdog catches, priorities that starve a task |
| **Crashes & debugging** | Decode a Guru Meditation backtrace with `addr2line`, read a core dump saved to flash, step through with `gdb` on QEMU's gdb stub, find a heap corruption with heap tracing |
| **Flash layout** | Partition tables, NVS key-value storage (and a namespace that fills up), a LittleFS filesystem out of space, firmware that no longer fits its partition |
| **OTA updates** | Update over HTTP across QEMU's Ethernet, A/B app partitions, automatic rollback when the new firmware fails its self-test |
| **Secure boot & flash encryption** | Turn both on, sign images, prove an unsigned or tampered image won't boot. **Emulator only:** on a real board these burn eFuses for good |
| **Firmware reverse engineering** | Dump a lab-built image, parse its partition table, pull Wi-Fi credentials out of NVS, find a hard-coded API key, patch and re-flash. Ties into *Firmware analysis* under Reverse engineering |
| **A networked device** | An HTTP server and REST API on the device, MQTT to a broker on the VM, mDNS, SNTP time, TLS with a client certificate from the lab CA. Ties into *MQTT & device fleets* |

**Bring-your-own-board extras (optional).** For a bare devkit on the learner's desk. Lima VMs
can't reach USB devices, so these flash and read the serial port from the Mac
(`esptool`, `idf.py monitor`) and grade themselves from the serial output: the firmware prints
a result line that a small host-side checker verifies.

| Built-in part | Labs |
|---|---|
| **Wi-Fi** | Station and access-point modes, a captive provisioning portal, scanning channels and RSSI |
| **Wi-Fi as a sensor** | Detect movement in the room from Wi-Fi channel state information (CSI) or RSSI changes, with no sensor at all |
| **Promiscuous mode** | Capture your own network's beacons and probe requests to a pcap and open it in Wireshark |
| **BLE** | A GATT server read from a phone (nRF Connect), advertising as a beacon, pairing and bonding |
| **ESP-NOW** *(two boards)* | Messages between boards with no router, measuring range and latency |
| **Touch pins** | A bare pin as a capacitive button: calibration, thresholds, waking from deep sleep on touch |
| **Hardware accelerators** | The hardware RNG and the AES/SHA engines against software versions, in speed and output |
| **On-chip sensors** | The internal temperature sensor (S2/S3/C3); the original ESP32's hall sensor (older ESP-IDF only) |
| **LED & BOOT button** | LEDC PWM fading, debouncing GPIO0, a Morse beacon on the onboard LED |
| **Power** | Deep sleep with timer, touch or button wake-up, the ULP coprocessor counting while the cores sleep, data kept in RTC memory across sleeps |
| **DAC → ADC with one jumper** *(original ESP32)* | Generate a waveform on a DAC pin, measure it on an ADC pin, map the ADC's non-linearity |

- **Tooling:** ESP-IDF (pinned, about 1–2 GB with toolchains) and Espressif's QEMU build in their
  own image, pulled on first use rather than in bootstrap; check that the Xtensa/RISC-V
  toolchains and QEMU build exist for arm64 Linux first.
- **Start with** *ESP-IDF & FreeRTOS from zero* and *Firmware reverse engineering*: both work
  entirely in QEMU.

## Event streaming

Kafka-style event streaming, which the roadmap so far only touches through RabbitMQ.
**Kafka in KRaft mode** (no ZooKeeper) or **Redpanda** for a lighter broker, with producers
and consumers the lab ships, so each module is a running system with something wrong in it.

| Module | What you'd practise | Runs on |
|---|---|---|
| **Topics & partitions** | create topics, partitions and replication factor, keys and ordering, `kafka-console-*` and `kcat`, retention by time and size, compaction on a changelog topic | Docker VM |
| **Consumers & groups** | consumer groups and offsets, lag that keeps growing, rebalance storms from a slow consumer, resetting offsets to replay a day, a consumer that commits before processing and loses messages | Docker VM |
| **Producers & delivery** | `acks`, retries and idempotent producers, duplicates after a retry, exactly-once with transactions, batching and compression trade-offs | Docker VM |
| **Schemas & contracts** | a schema registry with Avro or JSON Schema, a producer change that breaks consumers, compatibility modes, evolving a schema safely | Docker VM |
| **Operating a cluster** | three brokers in KRaft mode, a broker that dies and comes back, under-replicated partitions, reassigning partitions, disk filling up from retention, quotas for a noisy client | Docker VM (about 1.5 GB for three brokers) |
| **Streaming pipelines** | Kafka Connect moving data between PostgreSQL and a topic (change data capture with Debezium), a stuck connector, a dead-letter topic for poison messages | Docker VM; ties into the data track |

- **Checks** inspect the cluster and the data: consumer group lag is zero and offsets point
  where they should, every message from the check's own producer arrives exactly once,
  a topic has the right partitions and settings, a schema change is refused or accepted as
  the compatibility mode says, the replicated table in PostgreSQL matches the source.
- **Determinism:** the lab's producers write known message sets with per-run ids stored in
  `$KL_STATE`, so the check can count exactly what arrived.
- **Tooling to add:** the Kafka (KRaft) or Redpanda image, `kcat`, a schema registry
  (Karapace or Apicurio to stay open source), Kafka Connect with Debezium, pre-pulled like
  the other images.
- **Start with** *Topics & partitions* and *Consumers & groups* on a single broker; the
  three-broker cluster comes with *Operating a cluster*.

## AI engineering with tiny models

Building, running and securing applications on language models, with **small local models
only** — e.g. a 0.5–1.5B instruct model in GGUF (Qwen2.5-0.5B/1.5B, SmolLM2) under
**llama.cpp's server** (an OpenAI-compatible API), and a small embedding model
(all-MiniLM, bge-small) for search. No API keys and no internet: the models are pre-pulled
like the other images, so the labs stay offline. The focus is the engineering around the
model — retrieval, structured output, evaluation, security, operations — where small models
behave just like big ones, and their mistakes make the lessons easier to see.

| Module | Real-world example | What you'd practise |
|---|---|---|
| **Serving a model** | The platform team needs an internal LLM endpoint for three apps by Friday | run llama.cpp's server, quantization levels vs memory and speed, context size, concurrent requests and queueing, an API gateway with keys and rate limits in front, health checks, a model that OOMs the VM |
| **Retrieval (RAG)** | An on-call runbook bot that answers "how do I fail over the database?" from the team's own runbooks — and keeps quoting the old procedure | chunking, embeddings, a vector index in **pgvector** (ties into the data track) or Qdrant, hybrid keyword + vector search, re-ranking, citing sources, finding why the wrong document wins, re-indexing on change |
| **Structured output & extraction** | Accounts payable wants invoice e-mails turned into JSON (supplier, IBAN, total, due date) for the ERP | JSON Schema, grammar-constrained decoding (llama.cpp GBNF), validation and retries, normalising dates and amounts, routing low-confidence results to a human |
| **Classification & triage** | Route support tickets to the right team and flag the urgent ones | zero-shot vs few-shot prompts vs a small classifier trained on embeddings (ties into data science), confusion matrices, when the boring model is the better choice |
| **Tools & agents** | An ops assistant that may look up a ticket and restart a service — and must not do anything else | tool / function calling, an allow-list of tools and arguments, confirmation for risky actions, timeouts and step limits, an audit log of every call |
| **Evaluation** | A prompt "improvement" shipped last week and answers got worse; nobody noticed | a golden set with expected answers, scoring retrieval (hit rate, MRR) and answers (exact match, rubric checks), regression tests in CI (ties into automation), comparing two prompts or models on cost, latency and quality |
| **LLM security** | The runbook bot leaked a credential after someone hid instructions in a wiki page | the OWASP Top 10 for LLM applications: direct and indirect prompt injection, data exfiltration through tools and links, insecure output handling (XSS in a chat UI), PII redaction before text reaches the model, keeping secrets out of the context entirely |
| **Operations & cost** | Finance asks what the chatbot costs per user, and on-call asks why it's slow at 9 am | token accounting, latency percentiles, caching identical and similar prompts, streaming, batching, logging prompts without logging personal data, a dashboard that shows it all (ties into observability) |
| **Capstone: ship the assistant** | Take the half-built support assistant to production | retrieval over the help centre, structured ticket creation, an eval suite gating releases, injection defences, rate limits and monitoring — each graded |

- **Checks grade the system, not the model's prose.** Model output varies, so checks look at
  what the learner built: retrieval returns the right document ids for the check's own
  hidden questions (deterministic, since embeddings are), every extraction validates against
  the schema and matches the hidden invoices' fields, the eval harness computes the right
  score on a labelled set, a tool call outside the allow-list is refused, a canary secret
  can't appear in any answer because it never reaches the context, chat output is escaped.
  Where a model answer is graded, it's with temperature 0, a fixed seed and a pinned
  llama.cpp build and model file, and with generous criteria (the right document cited, the
  right label), never exact wording.
- **Determinism:** setup generates the corpora (runbooks, invoices, tickets) with per-run
  facts stored in `$KL_STATE`, so the right answer exists only in this run's documents and
  can't be guessed from a model's training data.
- **Resources:** a 0.5B model at 4-bit is about 400 MB and runs at usable speed on the Docker
  VM's 2 vCPUs; 1.5B is about 1 GB and slower. The embedding model is under 100 MB. They'd
  live in their own pre-pulled image (`kl-llm`), fetched on first use, like Jupyter.
- **Tooling to add:** llama.cpp (server and CLI), sentence-transformers or llama.cpp
  embeddings, pgvector (already PostgreSQL), optionally Qdrant, and Python with an
  OpenAI-compatible client, so the code learners write works unchanged against a hosted model.
- **Start with** *Serving a model* and *Retrieval*; nearly every later module builds on a
  served model and a working index.
- **Also cover (added 2026-11-24):**
  - **Statistics for evals**, in *Evaluation* (ties into the Statistics modules): is 82%
    → 85% on 50 questions real or noise? Confidence intervals for an eval score, a paired
    comparison of two prompts on the same questions, how many questions you need,
    measuring how often a grader disagrees with itself.
  - **Inference on Kubernetes**, as a module of its own or in the Kubernetes track:
    llama.cpp behind a Service, readiness that waits for the model to load, getting a
    large model file onto the nodes (init container, a PVC, an OCI artifact), scaling on
    queue depth with KEDA instead of CPU, a pod OOM-killed by a too-large context.
  - **Context and cost**, in *Operations & cost*: what goes into the context, token
    budgets per request, prompt caching (a stable prefix first), why quality drops in
    very long contexts, trimming conversation history.

## Programming

Focus on "this program is broken on a real system, fix it" and not on syntax tutorials,
which other sites already cover well. SQL is under [Data](#data).

| Track | What you'd practise | Runs on |
|---|---|---|
| **Python** | Automation scripts, boto3 against moto, the Kubernetes client, packaging and virtualenv breakage, `pytest`, async bugs | Docker VM; ties into AWS CLI |
| **Go** | Goroutine leaks and data races, `pprof`, small static builds, `client-go`, writing a Kubernetes controller | Docker VM; k8s lab for controllers |

Good practices and design patterns, in Python and Go, are done in
`exercises/programming/08-good-practices-patterns.yaml` (prog-087..107).

- **Checks:** run a hidden test suite plus real behaviour, such as "the service stays up
  under load" or "memory no longer grows". Keep tests outside `/lab` so they can't be
  edited by accident.
- **Toolchains:** run each language in a container on the Docker VM, so no new VM is
  needed.

### C64 game programming (6502 assembly and C)

Added 2026-11-21. Not started. A new track (`retro`) that teaches **making a game for the
Commodore 64 from zero**: first 6502 assembly, then the machine, then a game, in assembly
with ca65 or in C with cc65. Over the course the learner builds **one original single-screen
game of their own design** (no recreation of an existing game), plus small throwaway programs
for each new idea. Every tutorial works the idea through on a different example (other
addresses, other values), with the listing, what each line does, and what you should see.

| Module | Labs, beginner → advanced |
|---|---|
| **6502 from zero** *(first module, 20)* | Registers A/X/Y and the first `LDA`/`STA` · hex, binary and the byte · the flags (N, Z, C, V) after each instruction · branches and loops (`CPX`, `BNE`) · addressing modes (immediate, zero page, absolute, indexed, indirect-indexed) · 16-bit add and subtract with the carry · shifts for multiply/divide by 2 · subroutines, the stack and `JSR`/`RTS` · lookup tables instead of maths · BCD with `SED`/`ADC`/`CLD` for scores · *capstone:* a 16-bit multiply routine checked on hidden inputs |
| **The C64 machine** | The memory map and banking (`$01`) · screen RAM `$0400` and colour RAM `$D800` · screen codes vs PETSCII · clearing and drawing with loops · reading the joystick (`$DC00`, active low) and the keyboard matrix · waiting for a raster line (`$D012`) for 50 Hz timing · border and background colours · custom character sets (`$D018`) · *capstone:* a playable maze screen with a moving character in characters only |
| **Sprites** | Enable, position, colour and pointer (`$D000`–`$D02E`, `$07F8`) · X beyond 255 (`$D010`) · multicolour sprites and expansion · animation by changing the pointer · hardware collision (`$D01E`/`$D01F`) vs software boxes · priority against the background · *capstone:* the player and three enemies moving with the joystick and simple AI |
| **Making it a game** | The game loop and a frame counter · raster interrupts (`$D01A`, `$D019`, the vector at `$0314` or `$FFFE`) · movement that feels right (acceleration, friction, a jump table) · collisions with characters (walls, pickups) · score, lives and a HUD · a pseudo-random generator (16-bit LFSR, seeded from SID voice 3 at `$D41B`) · title screen, game over, high score · *capstone:* the learner's game, complete and playable |
| **Sound with the SID** | Voices, frequency tables and notes · ADSR envelopes · waveforms (triangle, saw, pulse, noise) · sound effects as frame-by-frame scripts · filters · playing a GoatTracker or SID Factory tune from the IRQ while keeping one voice for effects · hard restart for crisp notes · *capstone:* full sound for the game |
| **C with cc65** | `cl65` and the C64 target · `conio` and `joystick` drivers · writing VIC and SID registers from C (`<c64.h>`, `PEEK`/`POKE` style) · why `int` is slow: `unsigned char`, globals and `register` · mixing C and assembly (`__fastcall__`) · measuring a routine's cycles · *capstone:* the same game in C, with its tightest loop rewritten in assembly |
| **Advanced techniques** | Smooth scrolling (fine scroll in `$D016`/`$D011`, the coarse shift every 8 pixels, spreading the copy over frames) · raster splits for a status bar · double buffering by flipping the screen or VIC bank · animated characters · sprite multiplexing (sort by Y, re-use sprites from raster IRQs) · stable rasters · compressing the program (exomizer) and a loader · *capstone:* a scrolling level with more than 8 moving objects |

- **Design numbers for the tutorials** come from measurements of real C64 games, with credit
  to the game measured. Examples: input lag of at most 2 frames (Giana Sisters: 2 frames, 18
  frames to a top speed of 2 px/frame, a 27 px skid, a jump that starts at −4 px/frame with
  +0.12 px/frame² gravity); shooters move with no inertia (1942: 3 px/frame); hitboxes smaller
  than the 24-pixel sprite (8–16 px reach is common); multiplexers placing 32–60 sprites a
  frame (IK+, Bomberland, Bill & Ted); BCD scores with `SED`. Ideas and numbers only: no code,
  graphics or music from those games.
- **Toolchain:** cc65 (ca65, ld65, cl65), 64tass as an alternative assembler, VICE (`x64sc`)
  for running, and exomizer. All are light, build on arm64, and fit in one pinned image on the
  Docker VM.
- **ROMs: decide first.** VICE needs KERNAL, BASIC and character ROMs, which are copyrighted;
  Debian and Ubuntu's `vice` package ships without them. Options: the open-source
  [MEGA65 Open ROMs](https://github.com/MEGA65/open-roms) (free, mostly compatible for loading and
  running PRGs), or programs that take over the machine and need no KERNAL calls, so they run
  on any ROM. Never download the original ROMs in setup.
- **Checks** run the learner's PRG headless in VICE (`-warp`, `-limitcycles`, joystick and
  key input scripted, VICE's binary monitor read from Python) and look at the machine: screen
  and colour RAM, VIC and SID registers, a variable's address, cycles per frame. Pure 6502
  labs can also run in a Python 6502 emulator (py65) on hidden inputs. A screenshot
  (`-exitscreenshot`) saved to `/lab` lets the learner see the result.
- **Seeing and playing it:** the VM has no screen, so learners run VICE on their own machine
  (`brew install vice`, plus ROMs they own or Open ROMs) on the PRG that `awesome_labs sync`
  mirrors to `~/.awesome_labs/vscode/<lab>/`. Later: a browser C64 emulator in the web
  interface, so the game runs on the exercise page.

## Practice modes

New ways to use exercises, most of them reusing existing content.

- **Tutorial mode (guided, from zero):** a *Guide me* switch on the exercise page that
  walks a beginner through the exercise one step at a time instead of showing the whole
  scenario: **explain** (what and why, from the Learn text), **do** (the command, with
  ▸ Terminal), **what happened** (how to read the output), then a **step check** that
  advances when it passes.
  - **Steps reuse the checks:** each `check "…"` line already names one requirement, so a
    step maps to one or more check lines; the guide re-runs the check and looks only at its
    step's lines. No new grading code for most exercises.
  - **An optional `guide:` field** in the exercise YAML holds hand-written steps
    (explanation, command, the check lines it satisfies, what to look for in the output).
    Without one, the guide builds rougher steps from the tutorial, hints and check
    descriptions, so all exercises get a guide from the start.
  - **"From zero":** a short primer per track (what a container, pod, commit or plan is),
    linked from a step whenever it uses a concept the learner hasn't met yet (by tag).
  - **Honest progress:** "solved with guide" is recorded like hints and solutions are, so
    weak spots and exams stay meaningful; exams turn the guide off.
  - **Start with** hand-written guides for the beginner exercises of each track's first
    module (about 70), and the automatic fallback everywhere else. Guides are tested with
    `verify`/`verify -local` like exercises.
  - **Later, optional:** an *explain this output* button that asks an AI tutor about what's
    in the learner's terminal. It needs an API key and network access, so it stays opt-in;
    the labs themselves keep working offline.
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
| **Data** | Done in `exercises/data/06-recovery-and-resilience.yaml` |
| **Delivery and Kubernetes** | Broken Helm charts and a failed upgrade to roll back; GitOps drift (Argo CD may be tight on these nodes); killing a node under load while meeting an availability target |
| **Security** | SSH certificates from a small CA in place of `authorized_keys`; WAF tuning that blocks an attack and not real users; writing a detection from a real attack's logs and proving it catches a variant |
| **Networking** | BGP hijack, then stopping it with RPKI route validation |

### Next round

Added 2026-11-21. cert-manager is done (`exercises/kubernetes/23-cert-manager.yaml`, k8s-383..393).
The rest are not started; the first two are the suggested next step: they need almost no new
infrastructure and build on the networking and Terraform modules.

| Idea | The lab | Runs on | Builds on |
|---|---|---|---|
| **SSH tunnels & pivoting** | Reach a database two hops away with `-L`, `-R`, `-D` and `ProxyJump`, then lock the jump host down (`PermitOpen`, `AllowTcpForwarding`) | Docker or offsec VM; namespaces or containers as hosts | Networking essentials |
| **Purging secrets from Git** | gitleaks finds a key in history; remove it with `git filter-repo`, force-push to a bare remote, and prove no ref, tag or reflog still holds it | Docker VM | Git recovery & plumbing |
| **Linkerd service mesh** | Turn on mTLS between two apps, find the pod without its proxy, add retries and timeouts. Much lighter than Istio (see *Left out*), but **test the memory on the nodes first** | k8s lab | Services & DNS |
| **Cluster backup with Velero** | Back up a namespace, delete it, restore it, then find why the volume data didn't come back | k8s lab + MinIO | The MinIO labs |
| **Backups with restic or borg** | Encrypted repository, a retention policy that deletes too much, a restore drill against a deadline, a repository locked by a dead job | Docker VM | Linux admin |
| **Podman** | Rootless containers, Quadlet systemd units, a port below 1024 that won't bind, `podman generate kube` | Docker VM (pinned package) | Docker containers |
| **Load testing (k6 or wrk)** | Find the request rate where p99 latency breaks, spot coordinated omission, then fix the connection-pool limit behind it | Docker VM | Web servers & proxies |
| **SLOs & error budgets** | Work out the error budget from access logs, then write a multi-window burn-rate alert that fires on a fast burn and stays quiet on a blip | Docker VM; with Prometheus | Prometheus and alerting |
| **HTTP for developers** | CORS preflight failures, `SameSite` cookies, cache headers, redirect loops, content negotiation, all diagnosed with `curl` | Docker VM | Web servers & proxies |
| **Distributed systems** | A 3-node etcd cluster under netem partitions: lose quorum, read stale data, restore it; idempotency keys for a client that retries | Docker VM; netem | Degraded networks |
| **gRPC & Protobuf** | A schema change that breaks old clients (caught with `buf breaking`), reflection and `grpcurl` debugging, deadlines that never propagate | Docker VM | Programming |
| **ZFS or Btrfs on loop devices** | Snapshots, send/receive replication, a full pool, scrubbing after injected corruption | Docker VM; loop devices (risky for `verify -local`) | Linux disks & LVM |
| **Packaging** | Build a `.deb`, host a signed apt repository, fix a pinning or dependency conflict, a postinst script that fails halfway | Docker VM | Linux admin |
| **dbt with DuckDB** | Fix broken model lineage, failing data tests, and an incremental model that double-counts | Docker VM; data-science image | Data, SQL in depth |

### New lab formats

Added 2026-11-21. Not started. Ways of grading that existing tracks can reuse.

| Format | How it works | Fits |
|---|---|---|
| **Zero-downtime labs** | A traffic generator runs for the whole lab and logs every failed request; the check fails if even one dropped during the change. Proves "it works" and "it never broke" together | nginx reload, swapping a container behind a proxy, a k8s rollout with `preStop` and readiness, a Postgres expand/contract schema migration |
| **Famous outages, rebuilt** | Small versions of public incidents, each linking the real postmortem: Cloudflare 2019 (a regex that burned the CPU), GitLab 2017 (a deleted database and backups that didn't work), Facebook 2021 (a BGP withdrawal that took its DNS with it), left-pad (an unpublished dependency), AWS S3 2017 (a typo in an operational command), a leap-second bug | Every track; a natural incident at the end of a module |
| **Build your own X** | Write a small Redis-compatible server (RESP), DNS server or load balancer in Python or Go. The check drives it with the real clients: `redis-cli` and `redis-benchmark`, `dig`, `curl` under load | Programming, networking, data |
| **Make it 10x faster** | A slow script, query or service with hidden correctness tests and a time budget. Profile it (`cProfile`, `pprof`, `EXPLAIN ANALYZE`) and fix the real bottleneck, not the obvious one | Programming, data, algorithms |
| **King of the hill** | A scripted attacker hits your service every 30 seconds. Patch it while it keeps serving; the score counts the attacks that got through while you worked, and downtime counts against you | Defensive, offsec |

### More cool ideas

Added 2026-11-21. Not started.

| Idea | The lab | Runs on |
|---|---|---|
| **Kubernetes the hard way** | Bootstrap a cluster by hand: your own CA and certificates, etcd, the API server, controller manager, scheduler and kubelet, then the CNI. All the PKI from the TLS & PKI module, used for real. Destructive, so it runs on a cluster that is rebuilt afterwards | k8s lab (rebuilt after) |
| **Malicious packages** | A typosquatted package on a local `pypiserver` steals a secret during install. Find what it did, then stop it from happening again with hash-pinned requirements and a lockfile. The attack side of the *Supply chain* row | Docker VM |
| **Disk forensics with Sleuth Kit** | Recover a deleted file from an ext4 image with `fls` and `icat`, build a timeline from inode times with `mactime`, find a file hidden in slack space | Docker or offsec VM |
| **Text encoding problems** | Garbled UTF-8 (mojibake from a double decode), a BOM that breaks a script or a CSV header, CRLF in a shebang (`bad interpreter: /bin/bash^M`), Unicode normalisation making two equal-looking usernames differ, `LC_ALL` changing how `sort` orders | Docker VM |
| **Hardening scored by Lynis** | Raise the Lynis hardening index above a threshold while every service on the box keeps working (the check tests both) | Docker VM |
| **A mini Internet** | Capstone: several BGP autonomous systems meeting at an exchange point, a root DNS server and a CA, all on FRR and namespaces. Build it until a host in one AS can reach a TLS site in another by name, then fix faults injected at each layer | Docker VM; the FRR helpers |
| **A key-value store that survives crashes** | Write one with a write-ahead log. The check `kill -9`s it in the middle of writes and restarts it: nothing that was acknowledged may be lost (`fsync`, atomic renames, a torn last record) | Docker VM; Python or Go |

### Even more ideas

Added 2026-11-21. Not started. The suggested first five: **radio from recordings**, **a tiny OS
kernel**, **Open vSwitch**, **PostGIS** and **Suricata**: each is far from what the catalogue
covers and needs no heavy infrastructure.

**Low-level & building things**

| Idea | The lab | Runs on |
|---|---|---|
| **A tiny OS kernel** | Boot your own kernel in QEMU (RISC-V is simplest: `qemu-system-riscv64 -machine virt`), print to the serial port, take a timer interrupt, switch between two tasks. The check reads the serial output | Docker VM; QEMU |
| **Write an interpreter** | Tokenizer → parser → bytecode VM for a small language, graded by hidden test programs (output and error messages) | Docker VM; Python or Go |
| **RISC-V assembly** | A cleaner instruction set to learn than x86: registers, calls, the stack, syscalls, run with `qemu-user` on the arm64 VMs | Docker VM; cross toolchain |
| **Verilog in simulation** | A counter, a UART transmitter, VGA timing in Icarus Verilog or Verilator; the check runs a hidden testbench. No FPGA | Docker VM |
| **Arduino without a board** | AVR firmware in `simavr`: timers, interrupts, UART; the check reads the simulated pins and serial | Docker VM; avr-gcc |
| **WebAssembly & WASI** | Compile C or Go to wasm, run it in `wasmtime`, grant and deny file access, find why a module can't open a file | Docker VM |
| **Rust** | Borrow-checker errors to fix, a data race that `unsafe` let through, an async task that never wakes up | Docker VM; Rust image |

**Networking**

| Idea | The lab | Runs on |
|---|---|---|
| **Open vSwitch & OpenFlow** | Program a switch's flows by hand with `ovs-ofctl`: VLANs, a mirror port, a flow rule that drops too much | Docker VM; namespaces |
| **Multicast** | IGMP and PIM on FRR: a video stream (a UDP multicast sender) that never crosses the router | Docker VM; FRR helpers |
| **Flow monitoring** | SNMP counters and NetFlow/sFlow (`softflowd`, `nfdump`): find the host saturating the uplink | Docker VM |
| **IPv6-only with NAT64/DNS64** | An IPv6-only network that still reaches IPv4 sites (Tayga or Jool + DNS64 in Unbound), and an app that breaks on a hard-coded IPv4 literal | Docker VM |
| **QUIC & HTTP/3** | `curl --http3` against a lab server, and why clients fall back to HTTP/2 when UDP 443 is blocked | Docker VM |

**Security**

| Idea | The lab | Runs on |
|---|---|---|
| **Suricata & Zeek** | Write an IDS rule that catches the attack in a capture without alerting on normal traffic; read Zeek's `conn.log` to find the beacon | Offsec VM; ties into packet capture |
| **SSH honeypot (Cowrie)** | Run one, let a scripted attacker in, then reconstruct what it did from the session logs and the files it dropped | Offsec VM |
| **Android APK reversing** | `apktool` and `jadx`, no emulator: find a hard-coded key in a lab-built app, patch a check, re-sign it | Docker VM; JDK |
| **Passkeys (WebAuthn)** | Server-side registration and login, tested with a software authenticator (python-fido2); a relying-party ID mismatch to debug | Docker VM |
| **GraphQL attacks** | Introspection leaks, brute force through batched queries and aliases, then depth and cost limits that stop it | Offsec VM; `web_target` |
| **SOPS & age** | Encrypted secrets in Git, a key rotation, decrypting in a pipeline, the teammate who can't decrypt | Docker VM |

**Cloud-native & data**

| Idea | The lab | Runs on |
|---|---|---|
| **Canary releases with Argo Rollouts** | A rollout that rolls itself back when its error rate rises (analysis from Prometheus); test the nodes' memory first | k8s lab |
| **KEDA** | Scale workers from a Redis queue's length, down to zero when it's empty, and back up when work arrives | k8s lab |
| **PostGIS** | Nearest store, points in polygons, distances in metres vs degrees, and swapped latitude/longitude | Docker VM; PostgreSQL image with PostGIS |
| **Parquet, Arrow & DuckDB** | Partitioned datasets, predicate pushdown, a schema change that breaks reading old files | Docker VM; data-science image |
| **SQLite internals** | WAL mode, "database is locked" under concurrent writers, recovering a corrupt file with `.recover` | Docker VM |
| **TimescaleDB** | Hypertables, continuous aggregates, retention and compression policies on PostgreSQL | Docker VM |

**Just for fun**

| Idea | The lab | Runs on |
|---|---|---|
| **Radio from recordings (SDR)** | Decode a 433 MHz weather station from a lab-generated IQ recording with `rtl_433`, demodulate FM with numpy, find a signal in a waterfall. No radio hardware | Docker VM |
| **Quantum computing in a simulator** | Bell states, superposition and measurement, Grover's search on 3 qubits with Qiskit's simulator | Docker VM; data-science image |
| **Nix** | Reproducible dev shells, pinning nixpkgs, why the same build gives different results | Docker VM; Nix in a container |
| **Terminal fluency** | tmux sessions and panes, vim motions and macros, shell history tricks, graded by the files and sessions you end up with | Any VM |

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
| **Data and storage** | NFS root squash and stale file handles; sparse files and `rsync` |

### Packet capture labs

The defensive set is done in `exercises/defensive/06-network-forensics-and-hardening.yaml`, with
`tshark` pinned on the offsec VM and fetched on first use by `tshark_ready`. The offsec and
networking rows are still to do, and the Docker VM doesn't have `tshark` yet.

| Track | Ideas |
|---|---|
| **Defensive** | Done: `sec-045` to `sec-049` |
| **Offsec** | Sniff plaintext credentials on an isolated lab network; ARP spoofing between namespaces, then the defender's view of it; reverse a custom binary protocol from captures |
| **Networking** | Diagnose faults from captures: retransmissions, zero windows, RST storms and MTU problems |

### Scapy labs

Added 2026-11-21. Not started. A **Packet crafting with Scapy** module for the Networking track
(profile docker, after *Traffic & troubleshooting*, which teaches `tcpdump`): build packets layer
by layer, send them from network namespaces, and read captures from Python. Scapy is on no VM
yet.

| Level | Lab | What it teaches |
|---|---|---|
| Beginner | **Ping by hand** | `sr1(IP()/ICMP()/payload)`, reading a reply's TTL (a per-run `ip_default_ttl` on the target) |
| Beginner | **Dissect a capture** | `rdpcap`, `show()`, `haslayer`, indexing layers: count packets, find a DNS name, a SYN's port, one packet's TTL |
| Beginner | **Write a capture file** | Stack layers with `/`, set fields, DNS queries with `DNSQR`, `wrpcap` to a spec in `/lab/spec.txt` |
| Beginner | **ARP sweep** | `srp(Ether()/ARP())` on a LAN where hosts ignore ping; list every IP and MAC |
| Beginner | **Send exactly this datagram** | IP `tos`/`flags`/`ttl`, UDP ports and a payload, recorded by a sink on the target |
| Intermediate | **A SYN scan by hand** | SYN-ACK vs RST vs silence, so open, closed or filtered; why the kernel sends RSTs |
| Intermediate | **Traceroute with TTL** | `ttl=(1,6)`, ICMP time-exceeded, through a chain of router namespaces with per-run addresses |
| Intermediate | **DNS by hand** | `/lab/dnsq.py NAME TYPE` built on `DNS`/`DNSQR`, which the check runs against fresh dnsmasq records |
| Intermediate | **Sniff the one that matters** | `sniff(filter=…, lfilter=…, prn=…)`: put a message back together from chunks that arrive shuffled and duplicated among noise |
| Intermediate | **Reassemble the fragments** | MF and fragment offsets, `defragment()`, with two datagrams' fragments interleaved out of order |
| Advanced | **Through the firewall** | Read an nftables ruleset and craft what it lets through (a spoofed source port and a DSCP value) |
| Advanced | **ARP spoofing, then defend** | `poison.py TARGET GATEWAY` proven against a test client, then a permanent neighbour entry that resists it |
| Advanced | **A custom protocol layer** | A `Packet` class with `fields_desc`, `bind_layers`, a length filled in by `post_build`, checked against a hidden capture |
| Advanced | **Replay somewhere else** | Rewrite a capture to a new server: drop `Ether`, delete `chksum`/`len` so they're recomputed. A real UDP socket catches bad checksums, which a sniffer would miss |
| Capstone | **The covert channel** | Decode data hidden in ICMP payloads and IP ids, write `detect.py PCAP` that the check runs on a hidden capture, then send the implant's kill packet |

- **Install:** a `scapy_ready` kit helper, like `tshark_ready`, puts a pinned scapy (2.6.x) into
  `/opt/kl-scapy` with `pip --target` run in the pre-pulled `python:3.12-slim` image, adds a `.pth`
  file to the system Python and a `scapy` wrapper (`python3 -m scapy`). There's no bootstrap change,
  so no saved VM copy goes stale. Resets don't touch it.
- **Sending needs root:** learners run `sudo ip netns exec NS python3 script.py` or `scapy`. On
  layer 2 (`srp`, `sendp`), pass `iface="eth0"`.
- **Checks** read a `tcpdump -Z root` sink in the target namespace (a `pkt_sink NS FILE [BPF]`
  helper, with the file under `/tmp` and ending `.pcap` because of tcpdump's AppArmor profile), or
  a real socket where checksums matter. Script deliverables (`dnsq.py`, `poison.py`, `klp.py`,
  `detect.py`) are run by the check on fresh per-run data, so a hard-coded answer fails.
- **Cleanup:** background listeners and generators in namespaces need `on_reset` kills, because
  deleting a namespace leaves its processes running.
- **Later, offsec:** the same tool for the offensive track: sniffing plaintext credentials,
  TCP RST injection, DHCP starvation against a lab dnsmasq, and a defender's view in the
  capture.

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
| **Kubernetes OIDC** | Log in to the API server and map groups to RBAC | Dex, because Keycloak is too heavy for the 3.5GiB and 4GiB nodes |

### Microservices labs

Only `docker-080` and `k8s-300` touch this today, as themed incidents. Use small services and Traefik
or a lightweight proxy, since a service mesh is too heavy for these nodes.

| Lab | What goes wrong |
|---|---|
| **Find the slow service** | Distributed tracing across several services: find the slow span in Jaeger, then fix a trace that's broken into pieces (a service that drops the trace context). OpenTelemetry SDKs and Collector |
| **Cascading failure** | One slow dependency takes everything down, fixed with timeouts and circuit breakers |
| **Retry storms** | Retries turn a blip into an outage |
| **Partial failure** | Degrade gracefully when one service is gone |
| **Saga gone wrong** | An order left half complete across services, then compensate |
| **Message queues** | Poison messages, dead-letter queues and non-idempotent consumers |
| **API gateway** | Routing, rate limits and authentication at the edge |

### Critical-thinking labs

Labs that force the reasoning, not just the right answer. A check sees the answer and not the
thinking, so ask for the evidence behind it, randomise the cause, use several related cases, and
optionally have an LLM grade written reasoning against a rubric, which is less reliable than a
scripted check. Real stakes, pressure and escalation can't be modelled fully; on-call mode gets
partway there.

| Skill | How a lab forces it |
|---|---|
| **Symptom versus cause** | The obvious fix silences the alert, and the check proves the real fault remains |
| **Doubting the report** | The ticket states a confident diagnosis the evidence contradicts |
| **Correlation versus causation** | A deploy came just before the outage, but the logs show it started earlier |
| **Ignoring red herrings** | Alarming errors in the logs that have been there for months and mean nothing |
| **Knowing what you don't know** | The data can't settle it, and the right answer is "can't tell yet, I need X" |
| **Testing a hypothesis** | State a prediction first, and it only counts if the evidence matches |
| **Weighing the cost of checking** | Each diagnostic costs something, so running everything scores lower |
| **Judging trade-offs** | Two valid fixes, and the constraints make only one acceptable |

## Left out for now

- **Service mesh:** Istio would strain the 3.5GiB and 4GiB nodes for relatively little
  payoff.
