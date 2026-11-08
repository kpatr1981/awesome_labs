#!/bin/bash
# kubelab node state: snapshot pristine config files / restore them on reset.
# usage: state.sh snapshot|restore docker|cp|worker
set -uo pipefail
source /opt/kubelab/lib.sh

MODE=$1
ROLE=$2
P=$KL_STATE/pristine
USERHOME=$(lab_home)

# group:path  — a group is the systemd unit to restart when its files change
# ("none" and "static" restart nothing; the kubelet picks up static pod manifests itself).
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
    static:/etc/kubernetes
    kubelet:/etc/kubernetes/kubelet.conf
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

fingerprint() { # fingerprint GROUP [ROOT] -> hash of contents + metadata (paths relative to ROOT)
  local p root=${2:-/}
  while read -r p; do
    (
      cd "$root" || exit
      if [ -e ".$p" ] || [ -L ".$p" ]; then
        find ".$p" \( -type f -o -type l -o -type d \) -printf '%p %m %u %g %l\n' 2>/dev/null | sort
        find ".$p" -type f -print0 2>/dev/null | sort -z | xargs -0 -r sha256sum
      else
        echo "MISSING $p"
      fi
    )
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
  touch "$P/sums.v2"
  echo "$ROLE" >"$KL_STATE/role"
  echo "snapshot saved (${#existing[@]} paths)"
}

# restore_path PATH — make PATH identical to the snapshot, touching only what differs
# (static pod manifests that didn't change must keep their inode, or the kubelet
# restarts every control-plane pod).
restore_path() {
  local p=$1 src=$T$1
  if [ ! -e "$src" ] && [ ! -L "$src" ]; then
    rm -rf "$p"
  elif [ -d "$src" ] && [ ! -L "$src" ]; then
    [ -d "$p" ] && [ ! -L "$p" ] || { rm -rf "$p"; mkdir -p "$p"; }
    rsync -a --delete --checksum "$src/" "$p/"
  elif [ -L "$src" ] || [ ! -f "$p" ] || ! cmp -s "$src" "$p" ||
       [ "$(stat -c '%a %u %g' "$src")" != "$(stat -c '%a %u %g' "$p")" ]; then
    rm -rf "$p"
    mkdir -p "$(dirname "$p")"
    cp -a "$src" "$p"
  fi
}

restore() {
  [ -f "$P/files.tar" ] || { echo "no pristine snapshot on this node" >&2; exit 1; }
  local changed=() g p
  T=$(mktemp -d)
  if [ ! -f "$P/sums.v2" ]; then # fingerprints from an older kit: recompute from the snapshot itself
    tar --acls --xattrs -xpf "$P/files.tar" -C "$T" 2>/dev/null
    for g in $(groups); do fingerprint "$g" "$T" | sha256sum | cut -d' ' -f1 >"$P/$g.sum"; done
    touch "$P/sums.v2"
  fi
  for g in $(groups); do
    if [ "$(fingerprint "$g" | sha256sum | cut -d' ' -f1)" != "$(cat "$P/$g.sum")" ]; then
      changed+=("$g")
      [ -n "$(ls -A "$T")" ] || tar --acls --xattrs -xpf "$P/files.tar" -C "$T" 2>/dev/null
      while read -r p; do restore_path "$p"; done < <(paths_of "$g")
    fi
  done
  rm -rf "$T"
  if [ ${#changed[@]} -gt 0 ]; then
    echo "restored: ${changed[*]}"
    systemctl daemon-reload
    sysctl --system >/dev/null 2>&1 || true
  fi
  local u
  for u in "${UNITS[@]}"; do
    systemctl unmask "$u" >/dev/null 2>&1 || true
    systemctl enable "$u" >/dev/null 2>&1 || true
    if printf '%s\n' "${changed[@]}" | grep -qx "${u%.socket}" || ! systemctl is-active -q "$u"; then
      systemctl reset-failed "$u" >/dev/null 2>&1 || true
      systemctl restart "$u" || { sleep 3; systemctl reset-failed "$u" >/dev/null 2>&1; systemctl restart "$u"; }
    fi
  done
}

case $MODE in
snapshot) snapshot ;;
restore) restore ;;
*) echo "usage: $0 snapshot|restore ROLE" >&2; exit 2 ;;
esac
