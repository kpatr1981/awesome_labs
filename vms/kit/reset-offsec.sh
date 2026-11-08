#!/bin/bash
# Return the offensive-security lab to its pristine state:
#  - tear down every target container / network / volume,
#  - run cleanup hooks the previous exercise registered (on_reset),
#  - restore host files exercises mutate (users, sudoers, cron, docker, ssh),
#  - wipe /lab.
set -uo pipefail
source /opt/kubelab/lib.sh

# Stop and remove the exercises' target containers first so restores are clean.
docker ps -aq 2>/dev/null | xargs -r docker rm -f >/dev/null 2>&1
docker network ls --format '{{.Name}}' | grep -vxE 'bridge|host|none' | xargs -r docker network rm >/dev/null 2>&1
docker volume ls -q | xargs -r docker volume rm -f >/dev/null 2>&1

# Cleanup hooks (e.g. remove a planted SUID binary, kill a background listener).
/opt/kubelab/hooks.sh

# Kill stray listeners/processes an exercise started outside docker. The lab user's
# login shells and the reset itself are left alone.
pkill -u "$(lab_user)" -f 'python3? .*/srv/kl' >/dev/null 2>&1 || true
pkill -f '/srv/kl-' >/dev/null 2>&1 || true

# Restore the snapshotted host files (passwd/shadow/sudoers/cron/docker/ssh/...).
/opt/kubelab/state.sh restore offsec
eventually 60 'docker info' || { echo "docker did not come back" >&2; exit 1; }
chgrp docker /var/run/docker.sock 2>/dev/null; chmod 660 /var/run/docker.sock 2>/dev/null

# Exercise scratch area for target source, flags, loot, etc.
rm -rf /srv/kl && mkdir -p /srv/kl && chmod 755 /srv/kl

U=$(lab_user)
usermod -aG docker "$U" 2>/dev/null || true
rm -rf "$(lab_home)/.docker"
rm -rf /lab && mkdir -p /lab && chown "$U": /lab
echo "offsec lab reset"
