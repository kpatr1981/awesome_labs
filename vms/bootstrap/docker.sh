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
mkdir -p /var/lib/kubelab/pristine
/opt/kubelab/state.sh snapshot docker
docker images -q --no-trunc | sort -u >/var/lib/kubelab/pristine/images.txt
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u >/var/lib/kubelab/pristine/image-refs.txt
