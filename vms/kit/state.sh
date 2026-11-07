#!/bin/bash
# kubelab node state: snapshot pristine config files / restore them on reset.
# usage: state.sh snapshot|restore docker|cp|worker
set -uo pipefail
source /opt/kubelab/lib.sh

MODE=$1
ROLE=$2
P=$KL_STATE/pristine
USERHOME=$(lab_home)

# group:path  — a group is the systemd unit to restart when its files change.
case $ROLE in
docker)
  ENTRIES=(
    containerd:/etc/containerd/config.toml
    docker:/etc/docker
    docker:/etc/default/docker
    docker:/etc/systemd/system/docker.service.d
    docker:/etc/systemd/system/docker.socket.d
    docker:/usr/lib/systemd/system/docker.service
    docker:/usr/lib/systemd/system/docker.socket
    none:/etc/hosts
    none:/etc/sysctl.d
  )
  UNITS=(containerd docker.socket docker)
  ;;
cp | worker)
  ENTRIES=(
    containerd:/etc/containerd
    containerd:/usr/lib/systemd/system/containerd.service
    containerd:/etc/systemd/system/containerd.service.d
    kubelet:/etc/kubernetes
    kubelet:/var/lib/kubelet/config.yaml
    kubelet:/var/lib/kubelet/kubeadm-flags.env
    kubelet:/usr/lib/systemd/system/kubelet.service
    kubelet:/usr/lib/systemd/system/kubelet.service.d
    kubelet:/etc/systemd/system/kubelet.service.d
    kubelet:/etc/default/kubelet
    kubelet:/usr/bin/kubelet
    kubelet:/usr/bin/kubectl
    kubelet:/usr/bin/kubeadm
    none:/etc/crictl.yaml
    none:/etc/sysctl.d
    none:/etc/modules-load.d
    none:/root/.kube
    "none:$USERHOME/.kube"
  )
  UNITS=(containerd kubelet)
  ;;
*)
  echo "unknown role $ROLE" >&2
  exit 2
  ;;
esac

paths_of() { # paths_of GROUP
  local e
  for e in "${ENTRIES[@]}"; do
    [ "${e%%:*}" = "$1" ] && echo "${e#*:}"
  done
}

fingerprint() { # fingerprint GROUP -> hash of contents + metadata
  local p
  while read -r p; do
    if [ -e "$p" ] || [ -L "$p" ]; then
      find "$p" \( -type f -o -type l -o -type d \) -printf '%p %m %u %g %l\n' 2>/dev/null | sort
      find "$p" -type f -print0 2>/dev/null | sort -z | xargs -0 -r sha256sum
    else
      echo "MISSING $p"
    fi
  done < <(paths_of "$1")
}

groups() { printf '%s\n' "${ENTRIES[@]}" | cut -d: -f1 | sort -u; }

snapshot() {
  rm -rf "$P"
  mkdir -p "$P"
  local existing=()
  local e p
  for e in "${ENTRIES[@]}"; do
    p=${e#*:}
    if [ -e "$p" ] || [ -L "$p" ]; then existing+=("$p"); fi
  done
  tar --acls --xattrs -cpf "$P/files.tar" "${existing[@]}" 2>/dev/null
  local g
  for g in $(groups); do fingerprint "$g" | sha256sum | cut -d' ' -f1 >"$P/$g.sum"; done
  echo "$ROLE" >"$KL_STATE/role"
  echo "snapshot saved (${#existing[@]} paths)"
}

restore() {
  [ -f "$P/files.tar" ] || { echo "no pristine snapshot on this node" >&2; exit 1; }
  local changed=() g p
  for g in $(groups); do
    if [ "$(fingerprint "$g" | sha256sum | cut -d' ' -f1)" != "$(cat "$P/$g.sum")" ]; then
      changed+=("$g")
      while read -r p; do
        rm -rf "$p"
        tar --acls --xattrs -xpf "$P/files.tar" -C / "${p#/}" 2>/dev/null || true
      done < <(paths_of "$g")
    fi
  done
  if [ ${#changed[@]} -gt 0 ]; then
    echo "restored: ${changed[*]}"
    systemctl daemon-reload
    sysctl --system >/dev/null 2>&1 || true
  fi
  local u
  for u in "${UNITS[@]}"; do
    systemctl unmask "$u" >/dev/null 2>&1 || true
    systemctl enable "$u" >/dev/null 2>&1 || true
    if printf '%s\n' "${changed[@]}" | grep -qx "${u%.socket}"; then
      systemctl restart "$u"
    elif ! systemctl is-active -q "$u"; then
      systemctl restart "$u"
    fi
  done
}

case $MODE in
snapshot) snapshot ;;
restore) restore ;;
*) echo "usage: $0 snapshot|restore ROLE" >&2; exit 2 ;;
esac
