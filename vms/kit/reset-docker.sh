#!/bin/bash
# Return the docker host to its pristine state.
set -uo pipefail
source /opt/kubelab/lib.sh
P=$KL_STATE/pristine

docker swarm leave --force >/dev/null 2>&1 || true
docker ps -aq 2>/dev/null | xargs -r docker rm -f >/dev/null 2>&1
/opt/kubelab/state.sh restore docker
eventually 90 'docker info' || { echo "docker did not come back" >&2; exit 1; }
chgrp docker /var/run/docker.sock 2>/dev/null; chmod 660 /var/run/docker.sock 2>/dev/null
/opt/kubelab/hooks.sh
docker ps -aq | xargs -r docker rm -f >/dev/null 2>&1
docker network ls --format '{{.Name}}' | grep -vxE 'bridge|host|none' | xargs -r docker network rm >/dev/null 2>&1
docker volume ls -q | xargs -r docker volume rm -f >/dev/null 2>&1
docker images -q --no-trunc | sort -u >/tmp/kl-imgs
comm -23 /tmp/kl-imgs "$P/images.txt" | xargs -r docker rmi -f >/dev/null 2>&1
# extra tags pointing at pristine images
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u >/tmp/kl-refs
comm -23 /tmp/kl-refs "$P/image-refs.txt" | xargs -r docker rmi >/dev/null 2>&1
docker builder prune -af >/dev/null 2>&1
docker buildx ls --format '{{.Name}}' 2>/dev/null | grep -vxE 'default|desktop-linux' | xargs -r -n1 docker buildx rm >/dev/null 2>&1
# Re-tag pristine images in case an exercise removed/retagged them
while read -r ref; do
  [ -n "$ref" ] && ! docker image inspect "$ref" >/dev/null 2>&1 && docker pull -q "$ref" >/dev/null 2>&1
done <"$P/image-refs.txt"
U=$(lab_user)
usermod -aG docker "$U"
rm -rf "$(lab_home)/.docker"
rm -rf /lab && mkdir -p /lab && chown "$U": /lab
echo "docker host reset"
