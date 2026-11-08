#!/bin/bash
# Docker host bootstrap (as root): pre-pull images and take the pristine snapshot.
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive
# Tools for the Linux and Networking tracks. dnsmasq-base has no service, so it can't
# clash with systemd-resolved on port 53.
PKGS=(strace lsof lvm2 dnsmasq-base wireguard-tools traceroute acl)
missing=$(for p in "${PKGS[@]}"; do dpkg -s "$p" >/dev/null 2>&1 || echo "$p"; done)
if [ -n "$missing" ]; then
  apt-get update
  apt-get install -y $missing
fi
for img in $(grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /d/ {print $1}'); do
  for i in 1 2 3; do docker pull -q "$img" && break || sleep 5; done
done
# Python with the data-science libraries, for the Data Science track.
if ! docker image inspect kl-datascience:1 >/dev/null 2>&1; then
  docker build -q -t kl-datascience:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN pip install --no-cache-dir numpy==2.2.6 pandas==2.2.3 scipy==1.15.3 scikit-learn==1.6.1 joblib==1.5.1
DOCKERFILE
fi
mkdir -p /var/lib/kubelab/pristine
/opt/kubelab/state.sh snapshot docker
docker images -q --no-trunc | sort -u >/var/lib/kubelab/pristine/images.txt
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u >/var/lib/kubelab/pristine/image-refs.txt
