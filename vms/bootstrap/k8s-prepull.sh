#!/bin/bash
# Pre-pull workload images into containerd (as root).
# Lines starting with "==> " are shown in the app's Output tab as progress.
set -uxo pipefail
imgs=$(grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /k/ {print $1}')
total=$(echo "$imgs" | wc -l)
n=0
for img in $imgs; do
  n=$((n + 1))
  echo "==> image $n/$total $img"
  crictl inspecti "$img" >/dev/null 2>&1 && continue
  for i in 1 2 3; do crictl pull "$img" >/dev/null && break || sleep 5; done
done
# gVisor's runsc and its containerd shim (pinned, SHA-512 checked) for the RuntimeClass labs;
# containerd doesn't know about it until a lab configures it.
echo "==> installing gVisor (runsc)"
bash -c 'source /opt/kubelab/lib.sh; gv_install' || echo "==> could not install gVisor: the RuntimeClass exercises install it when they start"
exit 0
