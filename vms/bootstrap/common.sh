#!/bin/bash
# Runs on every kubelab VM after base provisioning (as root). Env: LAB_USER
set -euxo pipefail
mkdir -p /var/lib/kubelab /lab
echo "$LAB_USER" >/var/lib/kubelab/user
chown "$LAB_USER": /lab
H=$(getent passwd "$LAB_USER" | cut -d: -f6)
grep -q kubelab "$H/.bashrc" 2>/dev/null || cat >>"$H/.bashrc" <<'RC'
# --- kubelab ---
export PS1='\[\e[1;36m\]\u@\h\[\e[0m\]:\[\e[1;34m\]\w\[\e[0m\]\$ '
[ -d /lab ] && [ "$PWD" = "$HOME" ] && cd /lab
if command -v kubectl >/dev/null; then
  source <(kubectl completion bash)
  alias k=kubectl
  complete -o default -F __start_kubectl k
fi
export EDITOR=vim
RC
cat >/root/.vimrc <<'VRC'
set expandtab tabstop=2 shiftwidth=2 autoindent
VRC
cp /root/.vimrc "$H/.vimrc" && chown "$LAB_USER": "$H/.vimrc"
