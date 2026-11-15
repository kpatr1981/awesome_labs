#!/bin/bash
# Docker host bootstrap (as root): pre-pull images and take the pristine snapshot.
# Lines starting with "==> " are shown in the app's Output tab as progress.
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive
# Tools for the Linux and Networking tracks. dnsmasq-base has no service, so it can't
# clash with systemd-resolved on port 53.
PKGS=(strace lsof lvm2 dnsmasq-base wireguard-tools traceroute acl)
missing=$(for p in "${PKGS[@]}"; do dpkg -s "$p" >/dev/null 2>&1 || echo "$p"; done)
if [ -n "$missing" ]; then
  echo "==> installing packages: "$missing
  apt-get update
  apt-get install -y $missing
fi
# Pull 4 images at a time; an image that still fails after 3 tries is left for
# the exercise that uses it to pull.
imgs=$(grep -v '^#' /opt/kubelab/images.txt | awk '$2 ~ /d/ {print $1}')
total=$(echo "$imgs" | wc -l)
echo "==> pulling $total images, 4 at a time"
count=$(mktemp)
echo 0 >"$count"
export count total
pull() {
  local st="from cache"
  if ! docker image inspect "$1" >/dev/null 2>&1; then
    for i in 1 2 3; do docker pull -q "$1" && break || sleep 5; done
    st="could not pull"
    docker image inspect "$1" >/dev/null 2>&1 && st="pulled"
  fi
  flock "$count" bash -c 'n=$(($(cat "$count") + 1)); echo $n >"$count"; echo "==> image $n/$total $2 $1"' _ "$1" "$st"
}
export -f pull
echo "$imgs" | xargs -P4 -n1 bash -c 'pull "$1"' _
rm -f "$count"
# Python with the data-science libraries, for the Data Science track.
if ! docker image inspect kl-datascience:1 >/dev/null 2>&1; then
  echo "==> building the data-science image (numpy, pandas, scipy, scikit-learn)"
  docker build -q -t kl-datascience:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN pip install --no-cache-dir numpy==2.2.6 pandas==2.2.3 scipy==1.15.3 scikit-learn==1.6.1 joblib==1.5.1
DOCKERFILE
fi
# Terraform with its providers in a local mirror, for the Terraform track: `terraform
# init` then works offline and never hits the registry's rate limits.
if ! docker image inspect kl-terraform:1 >/dev/null 2>&1; then
  echo "==> building the Terraform image (terraform 1.16.7 and a provider mirror)"
  docker build -q -t kl-terraform:1 - <<'DOCKERFILE'
FROM hashicorp/terraform:1.16.7
RUN set -e; m=/usr/share/terraform/providers; \
    mkdir -p /tmp/a /tmp/b; \
    printf 'terraform {\n  required_providers {\n    local    = { source = "hashicorp/local", version = "2.9.1" }\n    random   = { source = "hashicorp/random", version = "3.9.1" }\n    null     = { source = "hashicorp/null", version = "3.3.2" }\n    tls      = { source = "hashicorp/tls", version = "4.4.2" }\n    time     = { source = "hashicorp/time", version = "0.14.2" }\n    external = { source = "hashicorp/external", version = "2.4.2" }\n    archive  = { source = "hashicorp/archive", version = "2.8.1" }\n    docker   = { source = "kreuzwerker/docker", version = "4.7.0" }\n  }\n}\n' > /tmp/a/main.tf; \
    printf 'terraform {\n  required_providers {\n    local  = { source = "hashicorp/local", version = "2.8.0" }\n    random = { source = "hashicorp/random", version = "3.8.1" }\n  }\n}\n' > /tmp/b/main.tf; \
    cd /tmp/a && terraform providers mirror "$m"; \
    cd /tmp/b && terraform providers mirror "$m"; \
    rm -rf /tmp/a /tmp/b; \
    printf 'provider_installation {\n  filesystem_mirror {\n    path    = "/usr/share/terraform/providers"\n    include = ["*/*/*"]\n  }\n}\n' > /etc/terraform.rc; \
    chmod -R a+rX "$m" /etc/terraform.rc
ENV TF_CLI_CONFIG_FILE=/etc/terraform.rc CHECKPOINT_DISABLE=1
DOCKERFILE
fi
# Ansible for the Ansible track: the controller image (ansible-core, ansible-lint and
# collections) and the managed-node image (Debian with sshd; nginx, tree, jq and cron
# are pre-downloaded so the apt module works offline).
if ! docker image inspect kl-ansible:1 >/dev/null 2>&1; then
  echo "==> building the Ansible image (ansible-core 2.21.6, ansible-lint, collections)"
  docker build -q -t kl-ansible:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN set -e; apt-get update; apt-get install -y --no-install-recommends openssh-client sshpass git; rm -rf /var/lib/apt/lists/*; \
    pip install --no-cache-dir ansible-core==2.21.6 ansible-lint==26.10.1; \
    ansible-galaxy collection install -p /usr/share/ansible/collections ansible.posix:2.2.2 community.general:14.0.1; \
    mkdir -p /etc/ansible; \
    printf '[defaults]\nhost_key_checking = False\nprivate_key_file = /lab/.ssh/ansible\nremote_user = ansible\ninterpreter_python = auto_silent\nretry_files_enabled = False\nnocows = 1\n\n[ssh_connection]\npipelining = True\n' > /etc/ansible/ansible.cfg; \
    printf '#!/bin/sh\n# give the calling uid a passwd entry: ssh refuses to run without one\nif ! getent passwd "$(id -u)" >/dev/null 2>&1; then echo "lab:x:$(id -u):$(id -g):lab:/tmp:/bin/sh" >> /etc/passwd; fi\nexec "$@"\n' > /usr/local/bin/kl-entry; \
    chmod 755 /usr/local/bin/kl-entry; chmod 666 /etc/passwd
ENV ANSIBLE_COLLECTIONS_PATH=/usr/share/ansible/collections
ENTRYPOINT ["/usr/local/bin/kl-entry"]
DOCKERFILE
fi
if ! docker image inspect kl-node:1 >/dev/null 2>&1; then
  echo "==> building the Ansible managed-node image"
  docker build -q -t kl-node:1 - <<'DOCKERFILE'
FROM debian:bookworm-slim
RUN set -e; apt-get update; \
    apt-get install -y --no-install-recommends openssh-server python3 python3-apt sudo init-system-helpers procps iproute2 iputils-ping; \
    apt-get install -y --download-only --no-install-recommends nginx tree jq cron; \
    useradd -m -s /bin/bash ansible; echo 'ansible ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/ansible; chmod 440 /etc/sudoers.d/ansible; \
    install -d -m 700 -o ansible -g ansible /home/ansible/.ssh; mkdir -p /run/sshd; \
    sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config; \
    rm -f /etc/ssh/ssh_host_*; ssh-keygen -A
CMD ["/usr/sbin/sshd", "-D", "-e"]
DOCKERFILE
fi
echo "==> taking the pristine snapshot"
mkdir -p /var/lib/kubelab/pristine
/opt/kubelab/state.sh snapshot docker
docker images -q --no-trunc | sort -u >/var/lib/kubelab/pristine/images.txt
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u >/var/lib/kubelab/pristine/image-refs.txt
