# Writing exercises

Exercises live in `exercises/<track>/NN-<module>.yaml`. One file is one module:

```yaml
track: kubernetes          # docker | kubernetes | offsec | linux | networking | cloud | data | defensive | programming
module: Services & DNS     # module title
profile: k8s               # docker | k8s | offsec — which lab environment
order: 3                   # module order within the track
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
```

All scripts get `/opt/kubelab/lib.sh` sourced (see `vms/kit/lib.sh`). As root on the
control plane, `KUBECONFIG` is `/etc/kubernetes/admin.conf`; the lab user has its own
`~/.kube/config`.

## The contract

1. `setup` runs on a **freshly reset** environment. Make it deterministic; if you
   randomise answers, store the expected value in `$KL_STATE/answer`.
2. `check` must **fail** right after setup and **pass** after `solve`. Run
   `kubelab verify <id>` to prove it.
3. Checks must be **quick to fail**: use `wait_pod`, `wait_pods`, `wait_deploy`
   (they give up early when pods are clearly stuck), and guard long `eventually`
   loops with a precondition.
4. Anything the reset can't restore on its own (new systemd units, files outside the
   snapshotted paths, sysctls, kube-system edits) must register a clean-up with
   `on_reset 'command'` in `setup`.
5. Prefer pre-pulled images (`vms/kit/images.txt`). Don't use `:latest`.

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
| `ing_get HOST PATH`, `ing_code`, `ing_tls` | through the Traefik ingress |
| `on_node01 'cmd'` | run as root on node01 (from the control plane) |
| `lab_file PATH <<EOF` | write a file owned by the lab user |
| `web_target NAME PORT <<PY` | run a stdlib-Python vulnerable target in container `kl-NAME` on 127.0.0.1:PORT (offsec) |
| `rand_flag` | a fresh `KL{…}` flag token (offsec) |
| `netns_new NS...`, `netns_link NS1 IF1 NS2 IF2` | network namespaces and veth links, removed on reset |
| `moto_start` | run the AWS emulator and install an `aws` command that uses it (sees only `/lab`) |
| `go_test <<'GO'` | run a hidden Go test file against the learner's `/lab/*.go` |
| `pg_start`, `pg_sql 'SQL'` | PostgreSQL in container `kl-pg`, and run SQL from a script |

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
