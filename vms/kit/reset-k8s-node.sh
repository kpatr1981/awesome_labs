#!/bin/bash
# Restore a kubernetes node's files and services (control plane or worker).
set -uo pipefail
source /opt/kubelab/lib.sh
ROLE=$(cat "$KL_STATE/role")
/opt/kubelab/state.sh restore "$ROLE"
# control-plane hooks usually need the API; reset-k8s-api.sh runs them there
[ "$ROLE" = worker ] && /opt/kubelab/hooks.sh
swapoff -a 2>/dev/null || true
# remove stray standalone containers created with ctr/nerdctl in the default namespace
ctr -n default containers ls -q 2>/dev/null | xargs -r ctr -n default containers rm >/dev/null 2>&1
# re-pull workload images the kubelet may have garbage-collected (e.g. after disk pressure)
missing=$(for i in $(grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /k/ {print $1}'); do crictl inspecti "$i" >/dev/null 2>&1 || echo "$i"; done)
[ -n "$missing" ] && (for i in $missing; do crictl pull "$i" >/dev/null 2>&1; done) &
rm -rf /lab && mkdir -p /lab && chown "$(lab_user)": /lab
echo "node $(hostname) reset"
