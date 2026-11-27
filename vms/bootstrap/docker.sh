#!/bin/bash
# Docker host bootstrap (as root): pre-pull images and take the pristine snapshot.
# Lines starting with "==> " are shown in the app's Output tab as progress.
set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive
# Tools for the Linux and Networking tracks (git-lfs for the Git track). dnsmasq-base has no service, so it can't
# clash with systemd-resolved on port 53.
PKGS=(strace lsof lvm2 dnsmasq-base wireguard-tools traceroute acl git-lfs)
missing=$(for p in "${PKGS[@]}"; do dpkg -s "$p" >/dev/null 2>&1 || echo "$p"; done)
if [ -n "$missing" ]; then
  echo "==> installing packages: "$missing
  apt-get update
  apt-get install -y $missing
fi
# The git-lfs package turns LFS on for every user in /etc/gitconfig. The Git track teaches
# `git lfs install`, so leave it off system-wide.
git lfs uninstall --system >/dev/null 2>&1 || true
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
# JupyterLab on top of it: the Data Science exercises are done in notebooks,
# and their checks run the notebook with nbclient (lib.sh nb_run).
if ! docker image inspect kl-jupyter:1 >/dev/null 2>&1; then
  echo "==> building the notebook image (JupyterLab, ipykernel, matplotlib)"
  docker build -q -t kl-jupyter:1 - <<'DOCKERFILE'
FROM kl-datascience:1
RUN pip install --no-cache-dir jupyterlab==4.6.4 ipykernel==7.4.0 matplotlib==3.11.2 && \
    jupyter labextension disable "@jupyterlab/apputils-extension:announcements"
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
# The same plus the AWS provider (against the Moto emulator), OpenTofu (sharing the mirror),
# Terragrunt and Conftest, for the Terraform track's AWS, backend and tooling modules.
if ! docker image inspect kl-terraform-aws:1 >/dev/null 2>&1; then
  echo "==> building the Terraform AWS image (aws provider 6.72.0, OpenTofu, Terragrunt, Conftest)"
  docker build -q -t kl-terraform-aws:1 - <<'DOCKERFILE'
FROM kl-terraform:1
RUN set -e; m=/usr/share/terraform/providers; mkdir -p /tmp/c; \
    printf 'terraform {\n  required_providers {\n    aws = { source = "hashicorp/aws", version = "6.72.0" }\n  }\n}\n' > /tmp/c/main.tf; \
    cd /tmp/c && terraform providers mirror "$m"; rm -rf /tmp/c; \
    cp -al "$m/registry.terraform.io" "$m/registry.opentofu.org"; \
    case "$(uname -m)" in aarch64) a=arm64; c=arm64 ;; *) a=amd64; c=x86_64 ;; esac; \
    cd /tmp; wget -q "https://github.com/opentofu/opentofu/releases/download/v1.13.8/tofu_1.13.8_linux_$a.zip"; \
    unzip -q "tofu_1.13.8_linux_$a.zip" tofu -d /usr/local/bin; rm -f tofu_*.zip; \
    wget -q -O /usr/local/bin/terragrunt "https://github.com/gruntwork-io/terragrunt/releases/download/v1.2.1/terragrunt_linux_$a"; \
    wget -q -O- "https://github.com/open-policy-agent/conftest/releases/download/v0.71.0/conftest_0.71.0_Linux_$c.tar.gz" | tar -xz -C /usr/local/bin conftest; \
    chmod 755 /usr/local/bin/tofu /usr/local/bin/terragrunt /usr/local/bin/conftest; chmod -R a+rX "$m"
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
# Python with the Model Context Protocol SDK, for the MCP modules of the Programming track.
if ! docker image inspect kl-mcp:1 >/dev/null 2>&1; then
  echo "==> building the MCP image (python 3.12, mcp 2.3.0)"
  docker build -q -t kl-mcp:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN pip install --no-cache-dir mcp==2.3.0
DOCKERFILE
fi
# Go 1.25 with the official Go MCP SDK (v1.8.0) in a module cache baked into the image, for the
# Go MCP labs of the Programming track: builds then work offline (GOPROXY=off).
if ! docker image inspect kl-mcp-go:1 >/dev/null 2>&1; then
  echo "==> building the Go MCP image (go 1.25.14, go-sdk v1.8.0)"
  docker build -q -t kl-mcp-go:1 - <<'DOCKERFILE'
FROM golang:1.25.14-alpine
ENV GOMODCACHE=/opt/gomod GOTOOLCHAIN=local CGO_ENABLED=0 GOFLAGS=-mod=mod
RUN set -e; mkdir -p /opt/mcp-template && cd /opt/mcp-template; \
    go mod init lab/mcpserver; \
    printf 'package main\n\nimport _ "github.com/modelcontextprotocol/go-sdk/mcp"\n\nfunc main() {}\n' >main.go; \
    go get github.com/modelcontextprotocol/go-sdk@v1.8.0; go mod tidy; go build -o /dev/null .; rm main.go; \
    chmod -R a+rX /opt/gomod /opt/mcp-template
ENV GOPROXY=off GOSUMDB=off
DOCKERFILE
fi
# llama.cpp's server with small local models, for the AI engineering track: chat models
# (Qwen2.5 Instruct 0.5B at Q4 and Q8, 1.5B at Q4) and an embedding model (bge-small),
# pinned by Hugging Face revision and SHA-256. ~5.8 GB on disk; the labs then run offline.
if ! docker image inspect kl-llm:1 >/dev/null 2>&1; then
  echo "==> building the LLM image (llama.cpp server b12124 and four small models, ~2.3 GB to download)"
  hf=https://huggingface.co
  if ! docker build -q -t kl-llm:1 - <<DOCKERFILE; then
FROM ghcr.io/ggml-org/llama.cpp:server-b12124
ADD --chmod=644 --checksum=sha256:74a4da8c9fdbcd15bd1f6d01d621410d31c6fc00986f5eb687824e7b93d7a9db $hf/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/9217f5db79a29953eb74d5343926648285ec7e67/qwen2.5-0.5b-instruct-q4_k_m.gguf /models/qwen2.5-0.5b-instruct-q4_k_m.gguf
ADD --chmod=644 --checksum=sha256:ca59ca7f13d0e15a8cfa77bd17e65d24f6844b554a7b6c12e07a5f89ff76844e $hf/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/9217f5db79a29953eb74d5343926648285ec7e67/qwen2.5-0.5b-instruct-q8_0.gguf /models/qwen2.5-0.5b-instruct-q8_0.gguf
ADD --chmod=644 --checksum=sha256:6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e $hf/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/91cad51170dc346986eccefdc2dd33a9da36ead9/qwen2.5-1.5b-instruct-q4_k_m.gguf /models/qwen2.5-1.5b-instruct-q4_k_m.gguf
ADD --chmod=644 --checksum=sha256:ec38e8da142596baa913124ae50550de284b6916bf59577ef2f0cb9660c2f514 $hf/CompendiumLabs/bge-small-en-v1.5-gguf/resolve/d32f8c040ea3b516330eeb75b72bcc2d3a780ab7/bge-small-en-v1.5-q8_0.gguf /models/bge-small-en-v1.5-q8_0.gguf
DOCKERFILE
    echo "==> could not build the LLM image: the AI engineering exercises need it (env rebuild docker retries)"
  fi
fi
# Python for the Retrieval labs of the AI engineering track: psycopg, pgvector, numpy, openai.
if ! docker image inspect kl-rag:1 >/dev/null 2>&1; then
  echo "==> building the retrieval image (python 3.12, psycopg, pgvector, openai)"
  docker build -q -t kl-rag:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN pip install --no-cache-dir "psycopg[binary]==3.3.8" pgvector==0.5.1 numpy==2.5.4 openai==3.48.0
DOCKERFILE
fi
# llama.cpp's server with a cross-encoder re-ranker (bge-reranker-v2-m3, Q4_K_M, ~440 MB),
# pinned by Hugging Face revision and SHA-256; shares its base layers with kl-llm:1.
if ! docker image inspect kl-rerank:1 >/dev/null 2>&1; then
  echo "==> building the re-ranker image (llama.cpp server b12124 and bge-reranker-v2-m3, ~440 MB to download)"
  if ! docker build -q -t kl-rerank:1 - <<DOCKERFILE; then
FROM ghcr.io/ggml-org/llama.cpp:server-b12124
ADD --chmod=644 --checksum=sha256:e186a244ed455b4ab66ec64339ce7427a6ae13f5c0b5e544de96e50f0f8b3673 https://huggingface.co/gpustack/bge-reranker-v2-m3-GGUF/resolve/3093af03b1a635e67b084b1d8c03c5f5e020fd05/bge-reranker-v2-m3-Q4_K_M.gguf /models/bge-reranker-v2-m3-q4_k_m.gguf
DOCKERFILE
    echo "==> could not build the re-ranker image: the Retrieval exercises need it (env rebuild docker retries)"
  fi
fi
# Temporal for the Programming track's durable-workflows module: the Temporal CLI (its dev
# server is a single process with an in-memory store) pinned by SHA-256, and the Python SDK.
if ! docker image inspect kl-temporal:1 >/dev/null 2>&1; then
  echo "==> building the Temporal image (temporal CLI 1.10.1, python 3.12, temporalio 1.36.1)"
  docker build -q -t kl-temporal:1 - <<'DOCKERFILE'
FROM python:3.12-slim
ARG TARGETARCH
ADD https://github.com/temporalio/cli/releases/download/v1.10.1/temporal_cli_1.10.1_linux_${TARGETARCH}.tar.gz /tmp/t.tgz
RUN set -e; case "$TARGETARCH" in \
      arm64) sum=2f748e9411b37c9cabb57f1f08e37765388c12b5534432c7aea28b593df9145d ;; \
      amd64) sum=4c1500e2ce2d4de136e5fe8852d9b7f82eb62f19b6b383b3e0c040519a9d20b1 ;; \
      *) echo "no temporal CLI for $TARGETARCH"; exit 1 ;; esac; \
    echo "$sum  /tmp/t.tgz" | sha256sum -c -; tar -xzf /tmp/t.tgz -C /usr/local/bin temporal; rm /tmp/t.tgz; \
    pip install --no-cache-dir temporalio==1.36.1
ENV TEMPORAL_ADDRESS=127.0.0.1:7233
DOCKERFILE
fi
# Keycloak for the Defensive track's identity module: built once (`kc.sh build`) so each lab's
# server starts in ~20 s instead of ~50 s; dev-file database, health endpoints on :9000.
if ! docker image inspect kl-keycloak:1 >/dev/null 2>&1; then
  echo "==> building the Keycloak image (keycloak 26.8.2, ~270 MB to download)"
  docker build -q -t kl-keycloak:1 - <<'DOCKERFILE'
FROM quay.io/keycloak/keycloak:26.8.2
RUN /opt/keycloak/bin/kc.sh build --health-enabled=true --db=dev-file
DOCKERFILE
fi
# Python with PyJWT and requests, for the identity labs' APIs and clients.
if ! docker image inspect kl-idp:1 >/dev/null 2>&1; then
  echo "==> building the identity-labs Python image (pyjwt 2.15.1, cryptography, requests)"
  docker build -q -t kl-idp:1 - <<'DOCKERFILE'
FROM python:3.12-slim
RUN pip install --no-cache-dir "pyjwt[crypto]==2.15.1" cryptography==50.0.2 requests==2.34.2
DOCKERFILE
fi
# gVisor's runsc (pinned, SHA-512 checked) and the job image kl-job:1, for the Defensive
# track's sandboxing module. runsc is not registered with Docker: the labs do that.
if ! /usr/local/bin/runsc --version 2>/dev/null | grep -q release- || ! docker image inspect kl-job:1 >/dev/null 2>&1; then
  echo "==> installing gVisor (runsc) and building the sandbox job image"
  bash -c 'source /opt/kubelab/lib.sh; gv_ready' ||
    echo "==> could not install gVisor: the sandboxing exercises install it when they start"
fi
docker builder prune -af >/dev/null 2>&1 || true
echo "==> taking the pristine snapshot"
mkdir -p /var/lib/kubelab/pristine
/opt/kubelab/state.sh snapshot docker
docker images -q --no-trunc | sort -u >/var/lib/kubelab/pristine/images.txt
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v '<none>' | sort -u >/var/lib/kubelab/pristine/image-refs.txt
