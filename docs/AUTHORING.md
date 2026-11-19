# Writing exercises

Exercises live in `exercises/<track>/NN-<module>.yaml`. One file is one module:

```yaml
track: kubernetes          # docker | kubernetes | offsec | linux | networking | cloud | data | defensive | programming | datascience | git | terraform | ansible | algorithms
module: Services & DNS     # module title
profile: k8s               # docker | k8s | offsec — which lab environment
order: 3                   # module order within the track
notebook: false            # true = done in a Jupyter notebook, which the check runs (docker profile only)
exercises:
- id: k8s-041              # unique
  title: Give the deployment a stable address
  difficulty: 1            # 1 beginner … 5 expert
  kind: build              # build | fix
  tags: [service]
  tutorial: |              # markdown: the concept and commands to read first; no spoilers
  scenario: |              # markdown, shown in the UI and written to /lab/TASK.md
  hints: [...]             # revealed one at a time
  solution: |              # markdown
  setup: |                 # bash, root, primary node (docker host / controlplane)
  setup_worker: |          # bash, root, node01 (k8s only) — runs BEFORE setup
  check: |                 # bash, root, primary node; uses the check helpers below
  check_worker: |          # bash, root, node01 (k8s only)
  solve: |                 # bash, as the lab user in /lab — the reference solution
  destructive: false       # true = env is rebuilt before the next exercise
  deliverables: [answer.txt] # files in /lab the check grades (with notebook: true: the files the notebook writes)
  starter: |               # notebook modules only: code for the starter notebook, cells split at "# %%"
```

All scripts get `/opt/kubelab/lib.sh` sourced (see `vms/kit/lib.sh`). As root on the
control plane, `KUBECONFIG` is `/etc/kubernetes/admin.conf`; the lab user has its own
`~/.kube/config`.

### Notebooks (`notebook: true`)

A module with `notebook: true` (the Data Science track) is done in a Jupyter notebook and
**graded on the notebook**. `start` writes `/lab/<id>.ipynb` (`awesome_labs notebook <id>`
prints it): a "how it's graded" header, an imports cell, the exercise's `starter:` code
(cells split at lines that are just `# %%`) and a save cell per deliverable. The web
interface opens it in JupyterLab.

- **check** starts with `nb_check <id>.ipynb [FILE...]` (lib.sh): it copies `/lab` to a
  scratch folder without `FILE...`, runs the notebook top to bottom in a fresh kernel there,
  and reports "runs from top to bottom" as a check of its own. Grade the files it wrote
  through `"$(nb_out FILE)"`, never `/lab/FILE`. To call the notebook's functions, pass
  test code on stdin (`<<'PY' ... PY`): it runs in the same kernel afterwards, its stdout
  is in `$NB_RESULT` (print tagged lines such as `part1: ok` and read them back with
  `sed`), and if it raises, a "works on the check's own data" check fails.
- **Hidden data** (test labels, expected tables) stays in `$KL_STATE`; pass it with
  `NB_HIDDEN="$KL_STATE/test.csv ..."` and read it in the test code at
  `/tmp/kl-hidden/<name>`. The test code and these files are out of the notebook's reach
  while it runs.
- **solve** is `nb_write <id>.ipynb <<'PY' ... PY`: the reference notebook.
- **deliverables** lists the files the notebook must write (no `.py`/`.ipynb`); leave it
  out when the check only calls functions. Each must be named in the scenario.
- **starter:** holds code to fix, or a skeleton, rather than a script in `/lab`: the check
  calls the functions by name, so the scenario says to keep their names and arguments.

`awesome_labs lint` enforces `nb_check`/`nb_write` on the exercise's own notebook. See
`ds-001` (a result file), `ds-003` (functions to fix) and `ds-013` (a model graded on
hidden data).

Every script may use `$KL_STATE` for its own files, except `$KL_STATE/user`: that holds the
lab user's name for every later script, and lint rejects any use of it.

## The contract

1. `setup` runs on a **freshly reset** environment. Make it deterministic; if you
   randomise answers, store the expected value in `$KL_STATE/answer`.
2. `check` must **fail** right after setup and **pass** after `solve`. Run
   `awesome_labs verify -record verified.json <id>` to prove it (see
   [Verifying and releasing](#verifying-and-releasing)).
3. Checks must be **quick to fail**: use `wait_pod`, `wait_pods`, `wait_deploy`
   (they give up early when pods are clearly stuck), and guard long `eventually`
   loops with a precondition.
4. Anything the reset can't restore on its own (new systemd units, files outside the
   snapshotted paths, sysctls, kube-system edits) must register a clean-up with
   `on_reset 'command'` in `setup`.
5. Prefer pre-pulled images (`vms/kit/images.txt`). Don't use `:latest`.

## Verifying and releasing

**`verified.json`** (repo root, committed) records which exercises passed `verify` in
their current form: one line per exercise id with a hash of what the run proved, the date
and the awesome_labs version. The hash covers the scripts (`setup`, `setup_worker`,
`check`, `check_worker`, `solve`), `deliverables`, `starter`, the profile, `notebook` and
`destructive`. It leaves out what a learner reads (title, tutorial, scenario, hints,
solution, tags, difficulty, kind) and where the exercise sits, so rewording an exercise or
moving it within or between module files keeps its entry. Changing a script makes it
stale. The shared kit (`vms/kit/lib.sh`, the bootstrap scripts) is in no hash: after
changing it, verify the tracks that use what you changed.

```bash
awesome_labs verify -record verified.json tf-086 tf-087   # run them; each pass updates its entry
awesome_labs verified                 # the gate: new or changed exercises; fails if any
awesome_labs verified -ids            # just their ids, one per line (for scripts)
awesome_labs verified -accept -note "verified on macOS" ans-090   # record a pass made elsewhere
awesome_labs verified -prune          # drop entries for exercises that no longer exist
awesome_labs counts                   # rewrite the exercise counts in README.md and docs/PENDING.md
make docs                             # the same, after a build
```

Entries noted `baseline` were accepted without a new run: every exercise up to v0.20.0
except tf-086..tf-092 had passed `verify` on real VMs before releasing. Use `-accept` only
for a pass that really happened somewhere `-record` couldn't see.

The counts live between markers, `<!-- count:docker -->184<!-- /count -->` (a track id, or
`total`); `counts` rewrites every marker, keeps a thousands separator where the old number
had one, and complains about a README track without one. Add a marker when you add a track.

**`make release`** refuses to build while `awesome_labs verified` lists anything or the
counts are stale (`counts -check`). `make release FORCE=1` builds anyway, with a loud
warning about the unverified exercises; say so in the release notes.

**CI** (`.github/workflows/ci.yml`, every push and pull request): gofmt, `go vet`, `go test`,
build, `lint` and `counts -check`, all with a throwaway HOME. The verified gate only
reports there, as a warning: an exercise can be merged before it has run on a VM.
`.github/workflows/verify.yml` is **experimental**: started by hand (optionally with ids) or
nightly, it enables KVM on a GitHub-hosted runner, installs Lima and QEMU with
`awesome_labs install --yes` and runs `verify -record` on the unverified exercises. The
runner is small (4 vCPUs, 16 GB, little disk, 6 hours), so the Kubernetes environment may
not fit. It commits nothing: download the `verify-results` artifact (the updated
`verified.json` and the log), check it and commit the record yourself.

## What the reset restores

- **Docker host**: removes containers, networks, volumes, non-pristine images and tags,
  build cache; restores `/etc/docker`, docker/containerd unit files and drop-ins,
  `/etc/hosts`, `/etc/sysctl.d`, socket permissions; wipes `/lab` and `~/.docker`.
- **Kubernetes nodes**: restores `/etc/kubernetes` (manifests, pki, kubeconfigs),
  kubelet config/flags/units, containerd config, kubectl/kubeadm/kubelet binaries,
  `~/.kube`; re-enables and restarts kubelet/containerd when needed.
- **Cluster**: deletes namespaces and cluster-scoped objects (CRDs, ClusterRoles,
  PVs, StorageClasses, webhooks, PriorityClasses, CSRs, …) created after the
  pristine snapshot; cleans `default`; restores node labels/taints/schedulability and
  CoreDNS / kube-proxy objects.

## Check helpers (excerpt)

| helper | meaning |
|--------|---------|
| `check "desc" 'expr'` / `check_not` | pass when the shell expression succeeds / fails |
| `check_eq "desc" "$actual" "expected"` / `check_match` | equality / regex |
| `eventually SECONDS 'expr'` | retry until true |
| `c_running NAME`, `c_get NAME '{{tmpl}}'`, `http_get URL` | docker |
| `cc_running PROJECT SERVICE`, `cc_get`, `cc_exec` | docker compose |
| `kj ARGS... JSONPATH`, `k_exists`, `pod_ready`, `deploy_ready`, `restarts` | kubectl |
| `k_curl NS URL` | curl from a throw-away pod |
| `svc_get NS SERVICE [PATH] [PORT]` | HTTP GET to a Service's ClusterIP from the node (retried) |
| `helm_chart DIR NAME [VERSION] [MESSAGE]` | write a small standard chart (nginx serving `.Values.message`, fullname/labels helpers, ConfigMap, Deployment, Service, NOTES) |
| `ing_get HOST PATH`, `ing_code`, `ing_tls` | through the Traefik ingress |
| `on_node01 'cmd'` | run as root on node01 (from the control plane) |
| `lab_file PATH <<EOF` | write a file owned by the lab user |
| `web_target NAME PORT <<PY` | run a stdlib-Python vulnerable target in container `kl-NAME` on 127.0.0.1:PORT (offsec) |
| `rand_flag` | a fresh `KL{…}` flag token (offsec) |
| `netns_new NS...`, `netns_link NS1 IF1 NS2 IF2` | network namespaces and veth links, removed on reset |
| `tshark_ready`, `lan_up NS=ADDR/LEN...`, `cap_start FILE`, `cap_stop`, `lan_down` | install the pinned tshark on older VMs; hosts on one bridge; capture all their traffic; remove the bridge and everything on it (offsec) |
| `frr_up NAME...`, `frr_link A IFA ADDR B IFB ADDR`, `frr_gw HOST GW` | FRR routers and hosts in containers, joined by veth links (docker host) |
| `frr_conf R <<EOF`, `frr_json R 'show ...'`, `frr_route R PREFIX`, `frr_ping NODE ADDR` | configure a router, read its state, test reachability |
| `frr_gw6 HOST GW`, `frr_route6 R PREFIX`, `frr_ping6 NODE ADDR` | the IPv6 versions |
| `moto_start` | run the AWS emulator and install an `aws` command that uses it (sees only `/lab`) |
| `moto_start_iam`, `moto_user NAME...`, `moto_denied PROFILE ARGS...` | the emulator with IAM enforced: `aws` is the administrator and `aws --profile NAME` another user; true only if IAM refused the call |
| `ds_start` | install `dspy`, which runs Python with pandas and scikit-learn in `/lab` |
| `go_test <<'GO'` | run a hidden Go test file against the learner's `/lab/*.go` |
| `algo_check <<'EOF'` | hidden Python tests, a `---GO---` line, then hidden Go tests, run against `/lab/solution.py` and `/lab/solution.go`; passes when either language passes (algorithms) |
| `pg_start`, `pg_sql 'SQL'` | PostgreSQL in container `kl-pg`, and run SQL from a script |
| `pg_primary NAME NET [flags]`, `pg_standby NAME NET PRIMARY [APP] [SLOT]`, `pg_x NAME 'SQL'`, `c_veth NAME`, `pgb_image` | Replicating PostgreSQL containers on a network, SQL on a named one, a container's host veth for `tc`, the PgBouncer image kept across resets (docker host) |
| `kr_up`, `kr_guard_calico`, `kr_conf <<EOF`, `kr_json 'show ...'`, `kr_ip`, `kr_nodes` | an FRR router outside the pod network, Pod `router` in `edge`, reachable from both k8s nodes; `kr_guard_calico` restores Calico's BGP settings on reset |
| `kr_estab IP`, `kr_hops PREFIX`, `kr_wget URL`, `kr_x CMD...` | read a BGP session, the next hops of a route, fetch a URL from the router, run a command in it |

## Offensive-security exercises (`profile: offsec`)

The `offsec` VM is a single Ubuntu host that is both the attacker box (nmap, ncat,
hydra, john, curl, python3) and the home for the targets. Keep everything
**self-contained** — targets run on this VM, nothing reaches the outside network —
and **deterministic**.

The standard pattern for a web task:

1. `setup` mints a flag with `FLAG=$(rand_flag); export FLAG`, stores the expected
   answer in `$KL_STATE/answer`, and starts the vulnerable app with
   `web_target NAME PORT <<'PY' … PY`. `web_target` forwards `FLAG`/`SECRET`/
   `DBUSER`/`DBPASS` into the container and registers its own teardown, so no
   `on_reset` is needed for the target.
2. The task asks the learner to capture the flag into a file under `/lab`.
3. `check` compares that file to `$KL_STATE/answer` (fails before the attack, passes
   after). `solve` is the reference exploit, run as the lab user in `/lab`.

For a **fix** exercise, ship the vulnerable source with `lab_file /lab/app.py`; the
`check` starts the learner's version in a throwaway container (register its removal
with `on_reset`) and probes that the bug is gone *and* the app still works.

## Trying exercises without a VM (`verify -local`)

`awesome_labs verify -local [-j N] [-from id] [-rebuild] [-force] [ids or prefixes...]`
runs the same setup → check fails → solve → check passes as `verify`, but each exercise
gets a **fresh privileged container on this machine's Docker** instead of the lab VM:
Ubuntu 24.04 with systemd as PID 1, the Docker engine at the versions the VM pins, the
packages of `vms/docker.yaml` and `vms/bootstrap/docker.sh`, a lab user (`lab`, sudo, in
the docker group), the real kit in `/opt/kubelab`, and the docker bootstrap's images
(including `kl-terraform`, `kl-ansible`, `kl-node`, `kl-jupyter`). Scripts run the way
the lab runner runs them: root through `sudo -H` from `/`, `solve` as the lab user in
`/lab`. The container is thrown away afterwards, so there is no reset.

```bash
awesome_labs verify -local tf-0                # every Terraform exercise from tf-0…
awesome_labs verify -local -j 3 docker-00 ans- # three at a time
```

- **First run** builds the harness image `kl-local-host:<hash>` and the **image store**,
  a Docker volume `kl-local-store` holding the inner engine's images (seeded from the
  lab's image cache, `-cache DIR`, then the bootstrap pulls the rest). That takes a few
  minutes once; it is rebuilt when the Dockerfile, `docker.sh`, `common.sh` or
  `images.txt` change, or with `-rebuild`. Each exercise then copies the store into its
  own volume (a reflink copy on btrfs/XFS, a full copy elsewhere) and starts in seconds.
- **Only the docker profile** runs here. Kubernetes and offsec exercises, ones with a
  worker script, and destructive ones are listed as skipped.
- **Host safety:** the container shares the host's kernel, devices and clock, so
  exercises whose scripts make loop devices, LVM or filesystems, use swap, apply kernel
  sysctls, set the clock or unload modules are skipped (`-force` runs them anyway, at
  your own risk). Network namespaces, veths, netem and nftables stay inside the
  container.
- Everything it creates is named `kl-local-*` and removed when the run ends or is
  interrupted; leftovers of a killed run are removed by the next one.
- It is a quick check, not a replacement: run `awesome_labs verify` on the real VM before
  a release. Differences from the VM: no Lima user/SSH, container DNS, the host's kernel.
- `AWESOME_LABS_STATE_DIR=DIR` keeps the run's logs (and any progress/config) away from
  `~/.awesome_labs`; it doesn't move Lima's VMs.
