#!/bin/bash
# Offensive-security lab: install attacker tooling and pre-pull target images.
# Runs as root; idempotent (re-run on every `env up`). Env: LAB_USER
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

# Attacker CLI tooling. All of it is standard, authorized-pentest / CTF tooling
# that operates only against the targets this same VM hosts.
# Core tools the exercises rely on; must install. Optional tools are nice-to-have.
# gcc/libc6-dev: compile vulnerable SUID binaries for the privesc/binary modules.
# libcap2-bin: setcap, for the Linux capabilities exercises. cron: scheduled-task privesc.
CORE=(nmap ncat hydra john hashid python3-pip gcc libc6-dev libcap2-bin cron)
OPT=(whatweb nikto)
need() { local p; for p in "$@"; do dpkg -s "$p" >/dev/null 2>&1 || echo "$p"; done; }
missing_core=$(need "${CORE[@]}")
missing_opt=$(need "${OPT[@]}")
if [ -n "$missing_core$missing_opt" ]; then
  apt-get update
fi
if [ -n "$missing_core" ]; then
  apt-get install -y $missing_core
fi
# Optional extras must never fail the whole lab (absent on some mirrors).
if [ -n "$missing_opt" ]; then
  apt-get install -y $missing_opt || true
fi

# A small, predictable wordlist for the enumeration / brute-force exercises, plus
# a pointer to rockyou (shipped gzipped by the wordlists package).
install -d /usr/share/kl/wordlists
if [ ! -f /usr/share/kl/wordlists/common.txt ]; then
  cat >/usr/share/kl/wordlists/common.txt <<'EOF'
admin
administrator
login
password
passwd
secret
config
backup
test
dev
api
users
user
robots.txt
.git
.env
uploads
images
assets
hidden
private
flag
EOF
fi
# A tiny password list that is quick to brute-force in grading.
cat >/usr/share/kl/wordlists/top.txt <<'EOF'
123456
password
password123
qwerty
letmein
dragon
monkey
admin
hunter2
sunshine
football
iloveyou
welcome
ninja
master
EOF

# Pre-pull the base images the target services run from (stdlib-only apps, no pip
# at exercise time). Keep versions pinned; never :latest.
for img in python:3.12-alpine nginx:1.27-alpine httpd:2.4-alpine owasp/modsecurity-crs:4.25.2-nginx-alpine-202610180210-lts; do
  docker image inspect "$img" >/dev/null 2>&1 || docker pull -q "$img" >/dev/null
done

# cron drives the scheduled-task privesc exercises; make sure it's running.
systemctl enable --now cron >/dev/null 2>&1 || true

# Make the wordlists and a scratch dir readable to the lab user.
U=${LAB_USER:?}
chown -R "$U": /usr/share/kl 2>/dev/null || true

echo "offsec lab tooling ready"
