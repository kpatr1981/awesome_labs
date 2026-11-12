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
exit 0
