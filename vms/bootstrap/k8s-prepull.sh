#!/bin/bash
# Pre-pull workload images into containerd (as root).
set -uxo pipefail
for img in $(grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /k/ {print $1}'); do
  for i in 1 2 3; do crictl pull "$img" >/dev/null && break || sleep 5; done
done
exit 0
