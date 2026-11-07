# Writing exercises

Exercises live in `exercises/<track>/NN-<module>.yaml`. One file is one module:

```yaml
track: kubernetes          # docker | kubernetes (display grouping)
module: Services & DNS     # module title
profile: k8s               # docker | k8s — which lab environment
order: 3                   # module order within the track
exercises:
- id: k8s-041              # unique
  title: Give the deployment a stable address
  difficulty: 1            # 1 beginner … 5 expert
  kind: build              # build | fix
  tags: [service]
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
