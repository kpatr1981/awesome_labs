# kubelab helper library — sourced by every setup / check / solve script.
# shellcheck shell=bash

export LAB=/lab
export KL_STATE=/var/lib/kubelab
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$PATH
if [ -z "${KUBECONFIG:-}" ]; then
  if [ "$(id -u)" = 0 ] && [ -r /etc/kubernetes/admin.conf ]; then
    export KUBECONFIG=/etc/kubernetes/admin.conf
  elif [ -r "$HOME/.kube/config" ]; then
    export KUBECONFIG=$HOME/.kube/config
  fi
fi

_kl_fails=0
_kl_checks=0

# ---------- reporting ----------
pass() { _kl_checks=$((_kl_checks + 1)); printf '  \033[32m✔\033[0m %s\n' "$*"; }
fail() { _kl_checks=$((_kl_checks + 1)); _kl_fails=$((_kl_fails + 1)); printf '  \033[31m✘\033[0m %s\n' "$*"; }
info() { printf '  \033[36m·\033[0m %s\n' "$*"; }

# check "description" 'shell expression'  -> pass when the expression succeeds
check() {
  local desc=$1; shift
  if ( eval "$*" ) >/dev/null 2>&1; then pass "$desc"; else fail "$desc"; fi
}

# check_not "description" 'shell expression' -> pass when the expression fails
check_not() {
  local desc=$1; shift
  if ( eval "$*" ) >/dev/null 2>&1; then fail "$desc"; else pass "$desc"; fi
}

# check_eq "description" "actual" "expected"
check_eq() {
  if [ "$2" = "$3" ]; then pass "$1"; else fail "$1 (got '${2}', want '${3}')"; fi
}

# check_match "description" "actual" 'regex'
check_match() {
  if [[ $2 =~ $3 ]]; then pass "$1"; else fail "$1 (got '${2}')"; fi
}

# eventually SECONDS 'shell expression' -> retry until true or timeout
eventually() {
  local t=$1; shift
  local end=$((SECONDS + t))
  while true; do
    if ( eval "$*" ) >/dev/null 2>&1; then return 0; fi
    [ $SECONDS -ge $end ] && return 1
    sleep 2
  done
}

kl_finish() {
  echo
  if [ "$_kl_fails" -eq 0 ]; then
    printf '\033[1;32mPASSED\033[0m — %d/%d checks\n' "$_kl_checks" "$_kl_checks"
    exit 0
  fi
  printf '\033[1;31mNOT YET\033[0m — %d of %d checks failing\n' "$_kl_fails" "$_kl_checks"
  exit 1
}

# ---------- setup helpers ----------
# lab_user — the interactive (non-root) user
lab_user() { cat "$KL_STATE/user" 2>/dev/null || echo "${SUDO_USER:-$USER}"; }
lab_home() { getent passwd "$(lab_user)" | cut -d: -f6; }

# as_user cmd... — run a command as the lab user
as_user() { sudo -u "$(lab_user)" -H bash -lc "$*"; }

# lab_file PATH [MODE] <<'EOF' ... EOF — write a file owned by the lab user
lab_file() {
  local p=$1 mode=${2:-0644}
  case "$p" in /*) ;; *) p="$LAB/$p" ;; esac
  mkdir -p "$(dirname "$p")"
  cat >"$p"
  chmod "$mode" "$p"
  chown -R "$(lab_user)": "$LAB" 2>/dev/null || true
  [[ $p == "$LAB"* ]] || chown "$(lab_user)": "$p"
}

# on_reset 'command' — register a cleanup command for the next reset
on_reset() {
  mkdir -p "$KL_STATE/cleanup.d"
  printf '%s\n' "$*" >>"$KL_STATE/cleanup.d/$(date +%s%N).sh"
}

# ---------- network namespace helpers ----------
# netns_new NAME... -> create namespaces with lo up; removed on reset
netns_new() {
  local n
  for n in "$@"; do
    ip netns add "$n"
    ip -n "$n" link set lo up
    on_reset "ip netns del $n 2>/dev/null"
  done
}
# netns_link NS1 IF1 NS2 IF2 -> connect two namespaces with a veth pair, both ends up
netns_link() {
  # build under temporary names: IF1/IF2 may match an interface on the VM itself
  local a="klv$RANDOM" b="klv$RANDOM"
  ip link add "$a" type veth peer name "$b"
  ip link set "$a" netns "$1" name "$2"
  ip link set "$b" netns "$3" name "$4"
  ip -n "$1" link set "$2" up
  ip -n "$3" link set "$4" up
}

# ---------- packet capture helpers ----------
# Keep in step with TsharkVersion in internal/lab/lab.go.
TSHARK_VERSION=4.2.2-1.1build3
# tshark_ready -> make sure tshark exists, also on VMs built before it joined provisioning.
# The VM's reset never touches /usr, so the install lasts.
tshark_ready() {
  command -v tshark >/dev/null && return 0
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq && apt-get install -y -qq "tshark=$TSHARK_VERSION" >/dev/null
}
# lan_up NS=ADDR/LEN... -> namespaces on one bridge (hub namespace kl-lan); any address is reachable
lan_up() {
  local h ns addr a b
  _kl_lan_ns=kl-lan
  ip netns add kl-lan && ip -n kl-lan link set lo up
  ip -n kl-lan link add br0 type bridge && ip -n kl-lan link set br0 up
  on_reset 'ip netns pids kl-lan | xargs -r kill 2>/dev/null; ip netns del kl-lan 2>/dev/null'
  for h in "$@"; do
    ns=${h%%=*}; addr=${h#*=}; a="klv$RANDOM"; b="klv$RANDOM"
    _kl_lan_ns="$_kl_lan_ns $ns"
    netns_new "$ns"
    ip netns exec "$ns" sysctl -qw net.ipv6.conf.all.disable_ipv6=1
    ip link add "$a" type veth peer name "$b"
    ip link set "$a" netns "$ns" name eth0
    ip link set "$b" netns kl-lan
    ip -n kl-lan link set "$b" master br0 up
    ip -n "$ns" addr add "$addr" dev eth0
    ip -n "$ns" link set eth0 up
    ip -n "$ns" route add default dev eth0
    on_reset "ip netns pids $ns | xargs -r kill 2>/dev/null"
  done
}
# cap_start FILE / cap_stop -> capture everything crossing the bridge; the file ends up owned by the lab user
cap_start() {
  ip netns exec kl-lan tcpdump -nn -U -s0 -i br0 -w "$1" >/dev/null 2>&1 </dev/null &
  _kl_cap_pid=$! _kl_cap_file=$1
  eventually 10 "[ -s '$1' ]"
}
cap_stop() {
  kill -INT "$_kl_cap_pid" 2>/dev/null
  wait "$_kl_cap_pid" 2>/dev/null
  chown "$(lab_user)": "$_kl_cap_file"
}
# lan_down -> stop everything running on the bridge and remove it; call after cap_stop
lan_down() {
  local n
  {
    for n in $(ip netns list | awk '{print $1}'); do
      case " $_kl_lan_ns " in *" $n "*) ip netns pids "$n" | xargs -r kill -9; ip netns del "$n" ;; esac
    done
    wait
  } 2>/dev/null
}

# ---------- FRR router helpers ----------
# Routers and hosts are FRR containers with no docker network; veth links join them.
# A restarted container loses its links, so labs must not restart routers.
FRR_IMAGE=quay.io/frrouting/frr:9.1.0
# frr_image -> make sure the image exists and survives resets, also on VMs built before
# the image joined images.txt
frr_image() {
  docker image inspect "$FRR_IMAGE" >/dev/null 2>&1 || docker pull -q "$FRR_IMAGE" >/dev/null || return 1
  local p=$KL_STATE/pristine
  grep -qxF "$FRR_IMAGE" "$p/image-refs.txt" && return 0
  docker image inspect -f '{{.Id}}' "$FRR_IMAGE" >>"$p/images.txt"
  echo "$FRR_IMAGE" >>"$p/image-refs.txt"
  LC_ALL=C sort -u -o "$p/images.txt" "$p/images.txt"
  LC_ALL=C sort -u -o "$p/image-refs.txt" "$p/image-refs.txt"
}
# frr_start NAME -> create one router container
frr_start() {
  docker run -d --name "$1" --hostname "$1" --network none --privileged \
    --restart no --entrypoint sh "$FRR_IMAGE" -c \
    "sed -i 's/^ospfd=no/ospfd=yes/;s/^bgpd=no/bgpd=yes/;s/^vrrpd=no/vrrpd=yes/;s/^ospf6d=no/ospf6d=yes/' /etc/frr/daemons; touch /etc/frr/vtysh.conf; exec /sbin/tini -- /usr/lib/frr/docker-start" >/dev/null
}
# frr_healthy NAME -> startup finished and every daemon is running
frr_healthy() {
  docker logs "$1" 2>&1 | awk '/all daemons up/{u=NR} /state -> down|Forked background/{d=NR} END{exit !(u>d)}' || return 1
  docker exec "$1" sh -c 'ps -o args | grep -q "[w]atchfrr\.sh" && exit 1; for d in zebra mgmtd bgpd ospfd ospf6d staticd vrrpd; do pidof $d >/dev/null || exit 1; done'
}
# frr_up NAME... -> start routers with ospfd, ospf6d, bgpd and vrrpd enabled and forwarding on.
# At boot watchfrr can restart the daemons and lose zebra for good, which leaves a router
# that accepts configuration and can't route. No links exist yet, so such a router is
# simply recreated.
frr_up() {
  local n round bad
  frr_image || return 1
  for n in "$@"; do frr_start "$n"; done
  for round in 1 2 3 4 5; do
    for n in "$@"; do eventually 30 "frr_healthy $n" || true; done
    sleep 5
    bad=
    for n in "$@"; do frr_healthy "$n" || bad="$bad $n"; done
    [ -z "$bad" ] && break
    echo "frr_up: recreating$bad (round $round)" >&2
    for n in $bad; do docker rm -f "$n" >/dev/null; frr_start "$n"; done
  done
  [ -z "$bad" ] || { echo "routers did not start:$bad" >&2; return 1; }
  for n in "$@"; do docker exec "$n" sysctl -qw net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1; done
}
# frr_link A IFA ADDR/LEN B IFB ADDR/LEN -> veth between two containers, addresses optional ("-")
frr_link() {
  local a="klv$RANDOM" b="klv$RANDOM" pa pb
  pa=$(docker inspect -f '{{.State.Pid}}' "$1"); pb=$(docker inspect -f '{{.State.Pid}}' "$4")
  ip link add "$a" type veth peer name "$b"
  ip link set "$a" netns "$pa" name "$2"
  ip link set "$b" netns "$pb" name "$5"
  [ "$3" != - ] && docker exec "$1" ip addr add "$3" dev "$2"
  [ "$6" != - ] && docker exec "$4" ip addr add "$6" dev "$5"
  docker exec "$1" ip link set "$2" up
  docker exec "$4" ip link set "$5" up
}
# frr_gw HOST GATEWAY -> give an end host its default route
frr_gw() { docker exec "$1" ip route replace default via "$2"; }
# frr_conf ROUTER <<EOF -> apply vtysh configuration from stdin ("configure terminal" is implied)
frr_conf() {
  local conf out rc i
  conf=$(cat)
  for i in 1 2 3 4 5; do
    out=$(printf '%s\n' "$conf" | docker exec -i "$1" sh -c 'cat >/tmp/kl.conf; vtysh -f /tmp/kl.conf' 2>&1); rc=$?
    if [ "$rc" = 0 ] && ! grep -qE '^% |failure|not running|Can.t' <<<"$out"; then return 0; fi
    echo "frr_conf $1 attempt $i failed (rc=$rc): $(grep -vE 'sending configuration|\] done|Waiting for children' <<<"$out" | head -5)" >&2
    sleep 1
  done
  return 1
}
# frr_json ROUTER 'show ... json' -> run a show command
frr_json() { docker exec "$1" vtysh -c "$2" 2>/dev/null; }
# frr_route ROUTER PREFIX -> "protocol interface next-hop" of the selected route, empty if none
frr_route() {
  frr_json "$1" "show ip route $2 json" | jq -r --arg p "$2" \
    '.[$p][]? | select(.selected==true) | "\(.protocol) \(.nexthops[0].interfaceName // "-") \(.nexthops[0].ip // "-")"' 2>/dev/null
}
# frr_ping NODE ADDRESS -> one reply within a second
frr_ping() { docker exec "$1" ping -c1 -W1 "$2" >/dev/null 2>&1; }
# frr_gw6, frr_route6, frr_ping6 -> the IPv6 versions of the above
frr_gw6() { docker exec "$1" ip -6 route replace default via "$2"; }
frr_route6() {
  frr_json "$1" "show ipv6 route $2 json" | jq -r --arg p "$2" \
    '.[$p][]? | select(.selected==true) | "\(.protocol) \(.nexthops[0].interfaceName // "-") \(.nexthops[0].ip // "-")"' 2>/dev/null
}
frr_ping6() { docker exec "$1" ping -6 -c1 -W1 "$2" >/dev/null 2>&1; }

# ---------- AWS emulator helper ----------
# moto_start -> run the emulator on 127.0.0.1:5000 and install an `aws` command
# that talks to it. The CLI runs in a container that can only see /lab.
moto_start() {
  docker rm -f kl-moto >/dev/null 2>&1 || true
  docker run -d --name kl-moto --network host motoserver/moto:5.2.4 -H 127.0.0.1 -p 5000 >/dev/null
  cat >/usr/local/bin/aws <<'WRAP'
#!/bin/bash
exec docker run --rm -i --network host \
  -e AWS_ENDPOINT_URL=http://127.0.0.1:5000 -e AWS_ACCESS_KEY_ID=test -e AWS_SECRET_ACCESS_KEY=test \
  -e AWS_DEFAULT_REGION=us-east-1 -e AWS_PAGER= -v /lab:/lab -w /lab amazon/aws-cli:2.39.0 "$@"
WRAP
  chmod 755 /usr/local/bin/aws
  on_reset 'rm -f /usr/local/bin/aws'
  eventually 30 'curl -fsS -m 2 http://127.0.0.1:5000/moto-api/ >/dev/null'
}

# moto_start_iam -> the emulator with IAM enforced. The first three calls create the
# administrator, every later call needs valid credentials. The `aws` command reads its
# identities from /lab/.aws/credentials, where [default] is that administrator.
moto_start_iam() {
  local k s doc='{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"*","Resource":"*"}]}'
  docker rm -f kl-moto >/dev/null 2>&1 || true
  docker run -d --name kl-moto --network host -e INITIAL_NO_AUTH_ACTION_COUNT=3 \
    motoserver/moto:5.2.4 -H 127.0.0.1 -p 5000 >/dev/null
  cat >/usr/local/bin/aws <<'WRAP'
#!/bin/bash
exec docker run --rm -i --network host -e AWS_ENDPOINT_URL=http://127.0.0.1:5000 \
  -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_SESSION_TOKEN -e AWS_PROFILE \
  -e AWS_SHARED_CREDENTIALS_FILE=/lab/.aws/credentials -e AWS_CONFIG_FILE=/lab/.aws/config \
  -e AWS_DEFAULT_REGION=us-east-1 -e AWS_PAGER= -v /lab:/lab -w /lab amazon/aws-cli:2.39.0 "$@"
WRAP
  chmod 755 /usr/local/bin/aws
  on_reset 'rm -f /usr/local/bin/aws'
  eventually 30 'curl -fsS -m 2 http://127.0.0.1:5000/moto-api/ >/dev/null' || return 1
  (
    export AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test
    aws iam create-user --user-name lab-admin >/dev/null
    aws iam put-user-policy --user-name lab-admin --policy-name administrator --policy-document "$doc"
    aws iam create-access-key --user-name lab-admin --query 'AccessKey.[AccessKeyId,SecretAccessKey]' --output text >"$KL_STATE/moto-admin"
  ) </dev/null || return 1
  read -r k s <"$KL_STATE/moto-admin"
  rm -f "$KL_STATE/moto-admin"
  [ -n "$k" ] && [ -n "$s" ] || return 1
  mkdir -p /lab/.aws
  printf '[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n' "$k" "$s" >/lab/.aws/credentials
  chmod 600 /lab/.aws/credentials
  chown -R "$(lab_user)": /lab/.aws
}
# moto_user NAME... -> create IAM users and add a profile for each to /lab/.aws/credentials
moto_user() {
  local u k s
  for u in "$@"; do
    aws iam create-user --user-name "$u" >/dev/null </dev/null || return 1
    read -r k s < <(aws iam create-access-key --user-name "$u" \
      --query 'AccessKey.[AccessKeyId,SecretAccessKey]' --output text </dev/null)
    [ -n "$k" ] && [ -n "$s" ] || return 1
    printf '[%s]\naws_access_key_id = %s\naws_secret_access_key = %s\n' "$u" "$k" "$s" >>/lab/.aws/credentials
  done
}
# moto_denied PROFILE AWS-ARGS... -> true only when IAM refused the call
moto_denied() {
  local p=$1; shift
  aws --profile "$p" "$@" 2>&1 | grep -qE 'AccessDenied|\(403\)'
}

# ---------- Terraform helpers ----------
# tf_start -> install a `terraform` command. It runs the pinned kl-terraform image,
# which installs providers only from a local mirror (no registry access), sees /lab
# and the Docker socket, and runs as the calling user so files stay theirs.
tf_start() {
  cat >/usr/local/bin/terraform <<'WRAP'
#!/bin/bash
# Lab Terraform: the kl-terraform image (providers from a local mirror). It sees /lab
# and the Docker socket; outside /lab it starts in /lab (use -chdir=...).
wd=$PWD
case "$wd/" in /lab/*) ;; *) wd=/lab ;; esac
envs=()
while IFS= read -r v; do envs+=(-e "$v"); done < <(compgen -e | grep -E '^(TF_|AWS_)' | grep -vx TF_CLI_CONFIG_FILE)
tty=()
[ -t 0 ] && [ -t 1 ] && tty=(-t)
exec docker run --rm -i "${tty[@]}" --network host --init \
  -u "$(id -u):$(id -g)" --group-add "$(stat -c %g /var/run/docker.sock)" \
  -e HOME=/tmp "${envs[@]}" \
  -v /lab:/lab -v /var/run/docker.sock:/var/run/docker.sock \
  -w "$wd" kl-terraform:1 "$@"
WRAP
  chmod 755 /usr/local/bin/terraform
  on_reset 'rm -f /usr/local/bin/terraform'
}
# tf_aws_start -> the Moto AWS emulator (moto_start) plus `terraform`, `tofu`, `terragrunt` and
# `conftest` commands from the kl-terraform-aws image (aws provider 6.72.0 in the mirror).
# Every AWS call goes to Moto: the wrappers always set AWS_ENDPOINT_URL to it, so a lab
# can never reach real AWS. Credentials default to Moto's dummies; set AWS_* to override.
# S3 Control puts the account id in front of the host, hence the extra hosts entry.
# Terragrunt's automatic provider cache lives in the container's HOME, which is gone after
# each run, so it's turned off: providers are installed into each unit's .terraform.
tf_aws_start() {
  moto_start || return 1
  local t
  for t in terraform tofu terragrunt conftest; do
    cat >"/usr/local/bin/$t" <<WRAP
#!/bin/bash
# Lab $t (kl-terraform-aws image). AWS calls go to the Moto emulator, never to real AWS.
wd=\$PWD
case "\$wd/" in /lab/*) ;; *) wd=/lab ;; esac
envs=(-e AWS_ENDPOINT_URL=http://localhost:5000 -e AWS_ACCESS_KEY_ID=\${AWS_ACCESS_KEY_ID:-test}
      -e AWS_SECRET_ACCESS_KEY=\${AWS_SECRET_ACCESS_KEY:-test} -e AWS_REGION=\${AWS_REGION:-us-east-1}
      -e TG_NO_AUTO_PROVIDER_CACHE_DIR=true)
while IFS= read -r v; do envs+=(-e "\$v"); done < <(compgen -e | grep -E '^(TF_|TG_|AWS_|PG)' | grep -vxE 'TF_CLI_CONFIG_FILE|AWS_ENDPOINT_URL|AWS_ACCESS_KEY_ID|AWS_SECRET_ACCESS_KEY|AWS_REGION|TG_NO_AUTO_PROVIDER_CACHE_DIR')
tty=()
[ -t 0 ] && [ -t 1 ] && tty=(-t)
exec docker run --rm -i "\${tty[@]}" --network host --init --add-host 123456789012.localhost:127.0.0.1 \\
  -u "\$(id -u):\$(id -g)" --group-add "\$(stat -c %g /var/run/docker.sock)" \\
  -e HOME=/tmp "\${envs[@]}" \\
  -v /lab:/lab -v /var/run/docker.sock:/var/run/docker.sock \\
  -w "\$wd" --entrypoint $t kl-terraform-aws:1 "\$@"
WRAP
    chmod 755 "/usr/local/bin/$t"
    on_reset "rm -f /usr/local/bin/$t"
  done
}
# tf_s3_state BUCKET KEY -> a state file kept in S3 (Moto), for tf_has/tf_get/tf_addrs:
#   st=$(mktemp); tf_s3_state tf-state app.tfstate >"$st"; tf_has "$st" aws_vpc.main
tf_s3_state() { aws s3 cp "s3://$1/$2" - 2>/dev/null; }
# tf_as_user DIR CMD... -> run terraform as the lab user in DIR (for setups)
tf_as_user() { local d=$1; shift; sudo -u "$(lab_user)" -H bash -c 'cd "$1" && shift && terraform "$@"' _ "$d" "$@"; }
# Checks read the local state file directly (fast, and works before `init`).
# STATE is a state file or a directory holding terraform.tfstate.
_tf_state() { if [ -d "$1" ]; then echo "$1/terraform.tfstate"; else echo "$1"; fi; }
_TF_JQ='def inst: .resources[]? as $r | $r.instances[]? | {
    address: ((if $r.module then $r.module + "." else "" end)
      + (if $r.mode == "data" then "data." else "" end) + $r.type + "." + $r.name
      + (if has("index_key") then (if (.index_key | type) == "number" then "[\(.index_key)]" else "[\"\(.index_key)\"]" end) else "" end)),
    mode: $r.mode, attributes: .attributes };'
# tf_addrs STATE -> managed resource addresses in the state, sorted
tf_addrs() { jq -r "$_TF_JQ"' [inst | select(.mode == "managed") | .address] | sort | .[]' "$(_tf_state "$1")" 2>/dev/null; }
# tf_has STATE ADDRESS -> the state holds that resource instance
tf_has() { tf_addrs "$1" | grep -qxF "$2"; }
# tf_get STATE ADDRESS ATTR -> an attribute (strings raw, anything else as JSON)
tf_get() {
  jq -r --arg a "$2" --arg k "$3" "$_TF_JQ"' [inst | select(.address == $a)][0].attributes[$k]
    | if . == null then empty elif type == "string" then . else tojson end' "$(_tf_state "$1")" 2>/dev/null
}
# tf_output STATE NAME -> a root output (strings raw, anything else as JSON)
tf_output() {
  jq -r --arg n "$2" '.outputs[$n].value | if . == null then empty elif type == "string" then . else tojson end' "$(_tf_state "$1")" 2>/dev/null
}
# tf_clean DIR -> `terraform plan` in DIR proposes no changes (needs an initialized DIR)
tf_clean() { (cd "$1" && terraform plan -detailed-exitcode -lock=false -input=false -no-color) >/dev/null 2>&1; }

# ---------- Ansible helpers ----------
# ans_start [NODE...] -> install the ansible commands and start managed nodes.
# The commands run the pinned kl-ansible image as the calling user on the
# kl-ansible-net network, where nodes are reachable by name. Nodes are kl-node
# containers (Debian, sshd, python3, user `ansible` with passwordless sudo) that
# trust the key /lab/.ssh/ansible. /etc/ansible/ansible.cfg in the image sets
# remote_user=ansible, that key, and host_key_checking=False; a project's own
# ansible.cfg replaces those defaults entirely.
ANS_TOOLS="ansible ansible-playbook ansible-inventory ansible-vault ansible-galaxy ansible-doc ansible-config ansible-console ansible-lint"
ans_start() {
  local u g t
  u=$(lab_user); g=$(id -gn "$u")
  for t in kl-ansible:1 kl-node:1; do
    docker image inspect "$t" >/dev/null 2>&1 || { echo "ans_start: image $t is missing; rebuild the Docker environment (awesome_labs env rebuild docker)" >&2; return 1; }
  done
  docker network inspect kl-ansible-net >/dev/null 2>&1 || docker network create kl-ansible-net >/dev/null
  install -d -m 700 -o "$u" -g "$g" /lab/.ssh
  [ -f /lab/.ssh/ansible ] || ssh-keygen -q -t ed25519 -N '' -C ansible@lab -f /lab/.ssh/ansible
  chown "$u:$g" /lab/.ssh/ansible /lab/.ssh/ansible.pub; chmod 600 /lab/.ssh/ansible
  cat >/usr/local/bin/kl-ansible <<'WRAP'
#!/bin/bash
# Lab Ansible: runs the kl-ansible image on the kl-ansible-net network, where the
# managed nodes live. It sees /lab only; outside /lab it starts in /lab.
tool=$(basename "$0")
if [ "$tool" = kl-ansible ]; then tool=$1; shift; fi
wd=$PWD
case "$wd/" in /lab/*) ;; *) wd=/lab ;; esac
envs=()
while IFS= read -r v; do envs+=(-e "$v"); done < <(compgen -e | grep -E '^ANSIBLE_')
tty=()
[ -t 0 ] && [ -t 1 ] && tty=(-t)
exec docker run --rm -i "${tty[@]}" --network kl-ansible-net \
  -u "$(id -u):$(id -g)" -e HOME=/tmp "${envs[@]}" \
  -v /lab:/lab -w "$wd" kl-ansible:1 "$tool" "$@"
WRAP
  chmod 755 /usr/local/bin/kl-ansible
  for t in $ANS_TOOLS; do ln -sf kl-ansible "/usr/local/bin/$t"; done
  on_reset "cd /usr/local/bin && rm -f kl-ansible $ANS_TOOLS"
  for t in "$@"; do ans_node "$t"; done
}
# ans_node NAME [DOCKER-RUN-ARGS...] -> (re)start a managed node trusting /lab/.ssh/ansible
ans_node() {
  local n=$1; shift
  docker rm -f "$n" >/dev/null 2>&1 || true
  docker run -d --name "$n" --hostname "$n" --network kl-ansible-net --init --label kl-ansible=node "$@" kl-node:1 >/dev/null
  docker exec -i "$n" sh -c 'cat >/home/ansible/.ssh/authorized_keys && chown ansible: /home/ansible/.ssh/authorized_keys && chmod 600 /home/ansible/.ssh/authorized_keys' </lab/.ssh/ansible.pub
  eventually 20 "docker exec $n sh -c 'pgrep -x sshd >/dev/null'"
}
# ans_project DIR -> a project with the lab's standard ansible.cfg and an inventory
# of web1, web2 (group web) and db1 (group db)
ans_project() {
  lab_file "$1/ansible.cfg" <<'EOF'
[defaults]
inventory = inventory.ini
remote_user = ansible
private_key_file = /lab/.ssh/ansible
host_key_checking = False
interpreter_python = auto_silent
retry_files_enabled = False

[ssh_connection]
pipelining = True
EOF
  lab_file "$1/inventory.ini" <<'EOF'
[web]
web1
web2

[db]
db1
EOF
}
# ans_play DIR PLAYBOOK-ARGS... -> run ansible-playbook in DIR as the lab user (for setups)
ans_play() { local d=$1; shift; ans_as_user "$d" ansible-playbook "$@"; }
# ans_on NODE CMD... -> run a shell command on a node, as root
ans_on() { local n=$1; shift; docker exec "$n" sh -c "$*"; }
# ans_get NODE URL -> fetch URL from inside NODE (the nodes have no curl)
ans_get() { docker exec "$1" python3 -c 'import sys, urllib.request as u; sys.stdout.write(u.urlopen(sys.argv[1], timeout=3).read().decode())' "$2" 2>/dev/null; }
# ans_as_user DIR CMD... -> run a command as the lab user in DIR (for setups)
ans_as_user() { local d=$1; shift; sudo -u "$(lab_user)" -H env ANSIBLE_NOCOLOR=1 bash -c 'cd "$1" && shift && "$@"' _ "$d" "$@"; }
# ans_idempotent DIR PLAYBOOK-ARGS... -> re-running ansible-playbook in DIR as the
# lab user succeeds and reports no changes on any host
ans_idempotent() {
  local d=$1 out; shift
  out=$(ans_as_user "$d" ansible-playbook "$@" 2>&1) || return 1
  grep -q 'ok=' <<<"$out" && ! grep -Eq '(changed|failed|unreachable)=[1-9]' <<<"$out"
}

# ---------- data-science helper ----------
# ds_start -> install `dspy`, which runs Python with pandas and scikit-learn in /lab.
# The container can only see /lab and runs as the calling user.
ds_start() {
  cat >/usr/local/bin/dspy <<'WRAP'
#!/bin/bash
t=-i
[ -t 0 ] && [ -t 1 ] && t=-it
exec docker run --rm $t --user "$(id -u):$(id -g)" -e HOME=/tmp -e PYTHONDONTWRITEBYTECODE=1 \
  -v /lab:/lab -w /lab kl-datascience:1 python "$@"
WRAP
  chmod 755 /usr/local/bin/dspy
  on_reset 'rm -f /usr/local/bin/dspy'
}

# ---------- notebook helpers (modules with notebook: true) ----------
# A notebook exercise is graded on what its notebook does, not on files left in
# /lab: the check runs the notebook from top to bottom in a fresh kernel, on a
# scratch copy of /lab without the results it must write, and grades what it
# wrote there (and, for code to fix, calls its functions on hidden data).
NB_IMAGE=kl-jupyter:1
NB_OUT=

# nb_run NOTEBOOK [RESULT...] [<<'PY' test code PY]
#   Copies /lab to a scratch folder ($NB_OUT, mounted as /lab in the container),
#   removes RESULT... there, runs /lab/NOTEBOOK cell by cell, then the test code
#   from stdin (if any) in the same kernel, and prints what the test printed.
#   NB_HIDDEN="FILE..." (paths, e.g. "$KL_STATE/test.csv") are given to the test
#   code only, as /tmp/kl-hidden/<name>: the test code and these files are taken
#   out of reach before the notebook's first cell runs, so it can't read them.
#   Prints "NB_FAIL <why>" and returns 1 when a cell fails or times out, and
#   "NB_TESTFAIL <why>" (return 2) when the test code raises: your code ran but
#   broke on the check's data.
nb_run() {
  local nb=$1 f U
  shift
  rm -rf /tmp/kl-nb.* 2>/dev/null
  NB_OUT=$(mktemp -d /tmp/kl-nb.XXXXXX)
  cp -a "$LAB/." "$NB_OUT/" 2>/dev/null
  for f in "$@"; do rm -f "$NB_OUT/$f"; done
  [ -t 0 ] && : >"$NB_OUT/.kl-test.py" || cat >"$NB_OUT/.kl-test.py"
  if [ -n "${NB_HIDDEN:-}" ]; then
    mkdir -p "$NB_OUT/.kl-hidden"
    for f in $NB_HIDDEN; do cp "$f" "$NB_OUT/.kl-hidden/"; done
  fi
  U=$(lab_user)
  chown -R "$U": "$NB_OUT"
  if [ ! -s "$NB_OUT/$nb" ]; then echo "NB_FAIL /lab/$nb is missing: open the exercise's notebook (or start it again)"; return 1; fi
  timeout "${NB_TIMEOUT:-300}" docker run --rm -i --network none --memory 2560m \
    --user "$(id -u "$U"):$(id -g "$U")" -e HOME=/tmp -e MPLBACKEND=Agg -e PYTHONDONTWRITEBYTECODE=1 \
    -v "$NB_OUT":/lab -w /lab "$NB_IMAGE" python - "$nb" "${NB_CELL_TIMEOUT:-120}" 2>/dev/null <<'NBRUN'
import os, re, shutil, sys
import nbformat
from nbclient import NotebookClient
from nbclient.exceptions import CellExecutionError, CellTimeoutError

name, limit = sys.argv[1], int(sys.argv[2])
# the check's own code and data: kept in memory, gone before the notebook runs
test = open("/lab/.kl-test.py").read()
os.remove("/lab/.kl-test.py")
hidden = {}
if os.path.isdir("/lab/.kl-hidden"):
    for f in os.listdir("/lab/.kl-hidden"):
        hidden[f] = open("/lab/.kl-hidden/" + f, "rb").read()
    shutil.rmtree("/lab/.kl-hidden")
nb = nbformat.read("/lab/" + name, as_version=4)
client = NotebookClient(nb, timeout=limit, kernel_name="python3", resources={"metadata": {"path": "/lab"}})
plain = lambda s: re.sub(r"\x1b\[[0-9;]*m", "", s)

def printed(outputs):
    out = []
    for o in outputs:
        if o.get("output_type") == "stream" and o.get("name") == "stdout":  # not warnings
            out.append(o.get("text", ""))
        elif o.get("output_type") in ("execute_result", "display_data"):
            out.append(o.get("data", {}).get("text/plain", "") + "\n")
    return "".join(out)

with client.setup_kernel():
    n = 0
    for i, cell in enumerate(nb.cells):
        if cell.cell_type != "code":
            continue
        n += 1
        try:
            client.execute_cell(cell, i)
        except CellTimeoutError:
            print(f"NB_FAIL code cell {n} ran longer than {limit}s")
            sys.exit(1)
        except CellExecutionError as e:
            print(f"NB_FAIL code cell {n} raised {e.ename}: {plain(str(e.evalue)).strip()[:200]}")
            sys.exit(1)
    if hidden:
        os.makedirs("/tmp/kl-hidden", exist_ok=True)
        for f, data in hidden.items():
            open("/tmp/kl-hidden/" + f, "wb").write(data)
    if test.strip():
        cell = nbformat.v4.new_code_cell(test)
        nb.cells.append(cell)
        try:
            client.execute_cell(cell, len(nb.cells) - 1)
        except (CellExecutionError, CellTimeoutError) as e:
            print(f"NB_TESTFAIL {getattr(e, 'ename', 'a timeout')}: {plain(str(getattr(e, 'evalue', ''))).strip()[:200]}")
            sys.exit(2)
        sys.stdout.write(printed(cell.outputs))
NBRUN
  local rc=$?
  [ $rc -eq 124 ] && { echo "NB_FAIL the notebook took longer than ${NB_TIMEOUT:-300}s"; return 1; }
  return $rc
}

# nb_check NOTEBOOK [RESULT...] [<<'PY' test PY]
#   nb_run, reported as a check of its own ("your notebook runs from top to
#   bottom"); the test code's output is left in $NB_RESULT for further checks.
NB_RESULT=
nb_check() {
  local nb=$1 o
  o=$(mktemp)
  nb_run "$@" >"$o" # not in $(...): nb_run sets NB_OUT for nb_out
  NB_RESULT=$(cat "$o")
  rm -f "$o"
  if grep -q '^NB_FAIL ' <<<"$NB_RESULT"; then
    fail "/lab/$nb runs from top to bottom in a fresh kernel ($(sed -n 's/^NB_FAIL //p' <<<"$NB_RESULT" | head -1))"
    NB_RESULT=
    return 1
  fi
  pass "/lab/$nb runs from top to bottom in a fresh kernel"
  if grep -q '^NB_TESTFAIL ' <<<"$NB_RESULT"; then
    fail "your code works on the check's own data (it raised $(sed -n 's/^NB_TESTFAIL //p' <<<"$NB_RESULT" | head -1))"
    NB_RESULT=
    return 1
  fi
}

# nb_out FILE -> the path of FILE as the notebook wrote it during nb_check
nb_out() { echo "$NB_OUT/$1"; }

# nb_write NOTEBOOK <<'PY' ... PY -> write /lab/NOTEBOOK (for solve scripts): the
# Python on stdin, split into code cells at lines that are just "# %%"
nb_write() {
  docker run --rm -i --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$LAB":/lab "$NB_IMAGE" python -c '
import sys, nbformat
src = sys.stdin.read()
cells, cur = [], []
for line in src.splitlines():
    if line.strip() == "# %%":
        cells.append("\n".join(cur).strip("\n")); cur = []
    else:
        cur.append(line)
cells.append("\n".join(cur).strip("\n"))
nb = nbformat.v4.new_notebook()
nb.cells = [nbformat.v4.new_code_cell(c) for c in cells if c.strip()]
nb.metadata["kernelspec"] = {"display_name": "Python 3 (ipykernel)", "language": "python", "name": "python3"}
nbformat.write(nb, "/lab/" + sys.argv[1])
' "$1"
}

# ---------- Go helper ----------
# go_test <<'GO' ... GO -> run a hidden test file against the learner's /lab/*.go
go_test() {
  local d rc
  d=$(mktemp -d)
  cp /lab/*.go "$d"/ 2>/dev/null
  cat >"$d/zz_hidden_test.go"
  timeout 180 docker run --rm -v "$d:/src" -v kl-gocache:/root/.cache -w /src golang:1.24-alpine \
    sh -c 'go mod init lab >/dev/null 2>&1; go test ./...' 2>&1 | tail -15
  rc=${PIPESTATUS[0]}
  rm -rf "$d"
  return "$rc"
}

# py_test <<'PY' ... PY -> run hidden unittest code (ending in unittest.main()) with /lab on
# PYTHONPATH, from /lab; prints the end of the output when it fails
py_test() {
  local d rc out
  d=$(mktemp -d)
  cat >"$d/t.py"
  out=$(cd /lab && PYTHONPATH=/lab timeout 120 python3 -B "$d/t.py" 2>&1); rc=$?
  rm -rf "$d"
  [ "$rc" -ne 0 ] && printf '%s\n' "$out" | tail -20
  return "$rc"
}

# ---------- algorithms helper ----------
# algo_check <<'EOF' ... EOF -> stdin holds hidden Python tests, a line "---GO---", then
# hidden Go tests. They run against /lab/solution.py and /lab/solution.go; the check
# passes when either language's tests pass, so learners pick the language they like.
algo_check() {
  local all py gosrc d prc=9 grc=9 pout="" gout=""
  all=$(cat)
  py=${all%%---GO---*}
  gosrc=${all#*---GO---}
  if [ -f /lab/solution.py ]; then
    d=$(mktemp -d)
    printf '%s\n' "$py" >"$d/t.py"
    pout=$(cd /lab && PYTHONPATH=/lab timeout 120 python3 -B "$d/t.py" 2>&1); prc=$?
    rm -rf "$d"
  fi
  if [ -f /lab/solution.go ]; then
    gout=$(printf '%s\n' "$gosrc" | go_test); grc=$?
  fi
  if [ "$prc" = 0 ]; then info "Python (/lab/solution.py): hidden tests pass"; else info "Python (/lab/solution.py): not passing yet"; fi
  if [ "$grc" = 0 ]; then info "Go (/lab/solution.go): hidden tests pass"; else info "Go (/lab/solution.go): not passing yet"; fi
  if [ "$prc" != 0 ] && [ "$grc" != 0 ]; then
    [ -n "$pout" ] && { echo "  --- Python test output (last lines) ---"; tail -12 <<<"$pout" | sed 's/^/  /'; }
    [ -n "$gout" ] && { echo "  --- Go test output (last lines) ---"; tail -12 <<<"$gout" | sed 's/^/  /'; }
  fi
  check "hidden tests pass in Python or Go" '[ "$prc" = 0 ] || [ "$grc" = 0 ]'
}

# ---------- PostgreSQL replication and PgBouncer helpers ----------
PGB_IMAGE=edoburu/pgbouncer:v1.24.1-p1
# pgb_image -> make sure the PgBouncer image exists and survives resets, also on VMs built
# before the image joined images.txt
pgb_image() {
  docker image inspect "$PGB_IMAGE" >/dev/null 2>&1 || docker pull -q "$PGB_IMAGE" >/dev/null || return 1
  local p=$KL_STATE/pristine
  grep -qxF "$PGB_IMAGE" "$p/image-refs.txt" && return 0
  docker image inspect -f '{{.Id}}' "$PGB_IMAGE" >>"$p/images.txt"
  echo "$PGB_IMAGE" >>"$p/image-refs.txt"
  LC_ALL=C sort -u -o "$p/images.txt" "$p/images.txt"
  LC_ALL=C sort -u -o "$p/image-refs.txt" "$p/image-refs.txt"
}
# pg_x NAME 'SQL' -> run SQL in the PostgreSQL container NAME and print bare rows
pg_x() { docker exec -i "$1" psql -h 127.0.0.1 -U lab -d lab -v ON_ERROR_STOP=1 -qAt -c "$2"; }
# pg_primary NAME NETWORK [postgres flags...] -> a primary on NETWORK that allows replication,
# with its data in the volume NAME-data
pg_primary() {
  local n=$1 net=$2; shift 2
  docker network create "$net" >/dev/null 2>&1 || true
  docker run -d --name "$n" --network "$net" -e POSTGRES_USER=lab -e POSTGRES_PASSWORD=lab -e POSTGRES_DB=lab \
    -v "$n-data:/var/lib/postgresql/data" postgres:16-alpine -c wal_level=replica "$@" >/dev/null
  eventually 60 "pg_x '$n' 'select 1'" || return 1
  docker exec "$n" sh -c "echo 'host replication lab all scram-sha-256' >> /var/lib/postgresql/data/pg_hba.conf"
  pg_x "$n" 'SELECT pg_reload_conf()' >/dev/null
}
# pg_standby NAME NETWORK PRIMARY [APP_NAME] [SLOT] -> a streaming standby of PRIMARY
pg_standby() {
  local n=$1 net=$2 pri=$3 app=${4:-} slot=${5:-} d=/var/lib/postgresql/data
  docker run --rm --network "$net" -e PGPASSWORD=lab -v "$n-data:$d" postgres:16-alpine \
    su-exec postgres pg_basebackup -h "$pri" -U lab -D "$d" -R -X stream -c fast ${slot:+-C -S "$slot"} || return 1
  if [ -n "$app" ]; then
    docker run --rm -v "$n-data:$d" postgres:16-alpine \
      sed -i "s/^primary_conninfo = '/primary_conninfo = 'application_name=$app /" "$d/postgresql.auto.conf"
  fi
  docker run -d --name "$n" --network "$net" -v "$n-data:$d" postgres:16-alpine >/dev/null
  eventually 60 "[ \"\$(pg_x '$n' 'SELECT pg_is_in_recovery()')\" = t ]"
}
# c_veth NAME -> the host-side veth of a container's eth0, for tc netem on that link
c_veth() {
  local i; i=$(docker exec "$1" cat /sys/class/net/eth0/iflink) || return 1
  grep -lx "$i" /sys/class/net/*/ifindex | cut -d/ -f5
}

# ---------- PostgreSQL helper ----------
# pg_start -> run PostgreSQL as container kl-pg (database and user "lab")
pg_start() {
  docker rm -f kl-pg >/dev/null 2>&1 || true
  docker run -d --name kl-pg -e POSTGRES_USER=lab -e POSTGRES_PASSWORD=lab -e POSTGRES_DB=lab postgres:16-alpine >/dev/null
  # the image restarts the server after init, so require a real TCP connection
  eventually 60 "docker exec kl-pg psql -h 127.0.0.1 -U lab -d lab -c 'select 1'"
}
# pg_sql 'SQL' -> run SQL and print bare rows; pg_sql with no argument reads stdin
pg_sql() {
  if [ $# -gt 0 ]; then
    docker exec -i kl-pg psql -h 127.0.0.1 -U lab -d lab -v ON_ERROR_STOP=1 -qAt -c "$1"
  else
    docker exec -i kl-pg psql -h 127.0.0.1 -U lab -d lab -v ON_ERROR_STOP=1 -qAt
  fi
}

# ---------- docker helpers ----------
# ---------- TLS & PKI helpers ----------
# Keys are EC P-256, or RSA 2048 with TLS_KEY=rsa. An existing PREFIX.key is reused, so
# several certificates can share a key. Certificates are signed with `openssl ca`, so
# TLS_START/TLS_END (YYYYMMDDHHMMSSZ, e.g. from `date -u -d '-1 year' +%Y%m%d%H%M%SZ`) can
# make expired or not-yet-valid ones; otherwise TLS_DAYS (default 365) from now.
tls_key() {
  [ -f "$1" ] && return 0
  if [ "${TLS_KEY:-ec}" = rsa ]; then
    openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out "$1" 2>/dev/null
  else
    openssl genpkey -algorithm EC -pkeyopt ec_paramgen_curve:P-256 -out "$1" 2>/dev/null
  fi
}

# tls_sign CA_PREFIX CSR OUT 'EXT' — sign CSR with CA_PREFIX.pem/.key; EXT is extension
# lines joined by \n (printf %b). The CSR's subject is kept as it is.
tls_sign() {
  local ca=$1 csr=$2 out=$3 ext=$4 t
  t=$(mktemp -d)
  : >"$t/index.txt"
  openssl rand -hex 16 >"$t/serial"
  printf '[ca]\ndefault_ca=d\n[d]\ndatabase=%s/index.txt\nnew_certs_dir=%s\nserial=%s/serial\ndefault_md=sha256\npolicy=p\nunique_subject=no\nemail_in_dn=no\n[p]\ncommonName=supplied\n[x]\n%b\n' \
    "$t" "$t" "$t" "$ext" >"$t/ca.cnf"
  local dates=(-days "${TLS_DAYS:-365}")
  [ -n "${TLS_START:-}" ] && dates=(-startdate "$TLS_START" -enddate "$TLS_END")
  if ! openssl ca -batch -notext -preserveDN -config "$t/ca.cnf" -cert "$ca.pem" -keyfile "$ca.key" \
    -in "$csr" -out "$out" -extensions x "${dates[@]}" >"$t/log" 2>&1; then
    cat "$t/log" >&2
    rm -rf "$t"
    return 1
  fi
  rm -rf "$t"
}

# tls_ca PREFIX "SUBJECT" [DAYS] — a self-signed root CA (PREFIX.pem, PREFIX.key). A
# SUBJECT without a slash is a common name.
tls_ca() {
  local subj=$2
  [[ $subj == /* ]] || subj="/CN=$subj"
  tls_key "$1.key"
  openssl req -x509 -new -key "$1.key" -out "$1.pem" -days "${3:-3650}" -subj "$subj" \
    -addext basicConstraints=critical,CA:TRUE -addext keyUsage=critical,keyCertSign,cRLSign \
    -addext subjectKeyIdentifier=hash 2>/dev/null
}

# tls_int PREFIX "SUBJECT" CA_PREFIX ['EXTRA EXT'] — an intermediate CA (pathlen 0) signed
# by CA_PREFIX, valid TLS_DAYS (default 1825).
tls_int() {
  local subj=$2
  [[ $subj == /* ]] || subj="/CN=$subj"
  tls_key "$1.key"
  openssl req -new -key "$1.key" -subj "$subj" -out "$1.csr" 2>/dev/null
  TLS_DAYS=${TLS_DAYS:-1825} tls_sign "$3" "$1.csr" "$1.pem" \
    "basicConstraints=critical,CA:TRUE,pathlen:0\nkeyUsage=critical,keyCertSign,cRLSign\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid:always${4:+\n$4}"
  local rc=$?
  rm -f "$1.csr"
  return $rc
}

# tls_leaf PREFIX "SUBJECT" CA_PREFIX "SAN" ['EXTRA EXT'] — a server certificate signed by
# CA_PREFIX. SAN is like "DNS:a.lab.test,IP:127.0.0.1"; "" leaves the extension out.
tls_leaf() {
  local subj=$2 ext ku=digitalSignature
  [[ $subj == /* ]] || subj="/CN=$subj"
  tls_key "$1.key"
  openssl pkey -in "$1.key" -noout -text 2>/dev/null | grep -q '^Private-Key.*primes' && ku="$ku,keyEncipherment"
  openssl req -new -key "$1.key" -subj "$subj" -out "$1.csr" 2>/dev/null
  ext="basicConstraints=CA:FALSE\nkeyUsage=critical,$ku\nextendedKeyUsage=${TLS_EKU:-serverAuth}\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid"
  [ -n "$4" ] && ext="$ext\nsubjectAltName=$4"
  tls_sign "$3" "$1.csr" "$1.pem" "$ext${5:+\n$5}"
  local rc=$?
  rm -f "$1.csr"
  return $rc
}

# tls_pin FILE — the base64 SHA-256 of a certificate's or key's public key (SPKI pin)
tls_pin() {
  local pub
  if grep -q 'BEGIN CERTIFICATE' "$1" 2>/dev/null; then
    pub=$(openssl x509 -in "$1" -noout -pubkey 2>/dev/null)
  else
    pub=$(openssl pkey -in "$1" -pubout -passin pass: 2>/dev/null)
  fi
  [ -n "$pub" ] || return 1
  printf '%s\n' "$pub" | openssl pkey -pubin -outform DER 2>/dev/null | openssl dgst -sha256 -binary | base64
}

# tls_ok PORT NAME CAFILE [s_client options...] — true when a client that trusts only
# CAFILE completes a handshake with 127.0.0.1:PORT and verifies it as NAME. (s_client says
# "Verify return code: 0" after a failed handshake too, so the cipher and certificate count.)
tls_ok() {
  local p=$1 n=$2 ca=$3 out
  shift 3
  out=$(echo | timeout 10 openssl s_client -connect "127.0.0.1:$p" -servername "$n" -verify_hostname "$n" \
    -CAfile "$ca" -verify_return_error "$@" 2>&1)
  grep -q 'Verify return code: 0 (ok)' <<<"$out" && grep -q 'BEGIN CERTIFICATE' <<<"$out" &&
    ! grep -q 'Cipher is (NONE)' <<<"$out"
}

# tls_peer PORT [NAME] [s_client options...] — the certificate the server on 127.0.0.1:PORT
# sends, as PEM; with no NAME (or ""), the client sends no SNI
tls_peer() {
  local p=$1 n=${2:-}
  shift 2 2>/dev/null || shift $#
  if [ -n "$n" ]; then set -- -servername "$n" "$@"; else set -- -noservername "$@"; fi
  echo | timeout 10 openssl s_client -connect "127.0.0.1:$p" "$@" 2>/dev/null | openssl x509 2>/dev/null
}

# tls_serve PORT CHAIN KEY [CLIENT_CA] — an HTTPS server answering "ok" on 127.0.0.1:PORT,
# in the background for up to 60s (pid in TLS_PID); fails when it can't load the files.
# Stop it with tls_stop.
tls_serve() {
  python3 - "$@" >/dev/null 2>&1 <<'PY' &
import http.server, ssl, sys, threading
port, chain, key = int(sys.argv[1]), sys.argv[2], sys.argv[3]
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
ctx.load_cert_chain(chain, key)
if len(sys.argv) > 4:
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.load_verify_locations(sys.argv[4])
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"ok\n")
    def log_message(self, *a):
        pass
s = http.server.ThreadingHTTPServer(("127.0.0.1", port), H)
s.socket = ctx.wrap_socket(s.socket, server_side=True, do_handshake_on_connect=False)
threading.Timer(60, s.shutdown).start()
s.serve_forever()
PY
  TLS_PID=$!
  local i
  for i in $(seq 50); do
    kill -0 "$TLS_PID" 2>/dev/null || return 1
    ss -ltnH "sport = :$1" 2>/dev/null | grep -q . && return 0
    sleep 0.1
  done
  return 1
}
tls_stop() {
  [ -n "${TLS_PID:-}" ] || return 0
  kill "$TLS_PID" 2>/dev/null
  wait "$TLS_PID" 2>/dev/null
  TLS_PID=
}

# tls_nginx NAME PORT DIR — nginx in container kl-NAME on 127.0.0.1:PORT, with DIR (its
# *.conf files and certificates) mounted at /etc/nginx/conf.d. Removed on reset.
tls_nginx() {
  docker rm -f "kl-$1" >/dev/null 2>&1
  docker run -d --name "kl-$1" -p "127.0.0.1:$2:443" -v "$3:/etc/nginx/conf.d" nginx:1.27-alpine >/dev/null
}

# tls_wait PORT [SECONDS] — wait until a TLS server on 127.0.0.1:PORT completes a handshake
# (a published Docker port accepts connections before the server in it is ready)
tls_wait() {
  local i
  for i in $(seq $((${2:-10} * 2))); do
    echo | timeout 3 openssl s_client -connect "127.0.0.1:$1" 2>/dev/null | grep -q 'BEGIN CERTIFICATE' && return 0
    sleep 0.5
  done
  return 1
}

# port_wait PORT [SECONDS] — wait until something listens on PORT, without connecting to it
port_wait() {
  local i
  for i in $(seq $((${2:-10} * 5))); do
    ss -ltnH "sport = :$1" 2>/dev/null | grep -q . && return 0
    sleep 0.2
  done
  return 1
}

c_running() { [ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null)" = true ]; }
c_exists() { docker inspect "$1" >/dev/null 2>&1; }
c_get() { docker inspect -f "$2" "$1" 2>/dev/null; } # c_get NAME '{{go template}}'
img_exists() { docker image inspect "$1" >/dev/null 2>&1; }
http_get() { curl -fsS -m 5 "$@"; }
http_code() { curl -s -o /dev/null -m 5 -w '%{http_code}' "$1"; }

# ---------- offensive-security helpers ----------
# web_target NAME PORT   (python source on stdin)
# Run a stdlib-only Python web app (the deliberately vulnerable target) in a
# container named kl-NAME, served on 127.0.0.1:PORT. The app source is read from
# stdin; $PORT is exported into it. Registers its own teardown for the next reset
# and waits until it answers.
web_target() {
  local name=$1 port=$2 dir=/srv/kl/$1
  mkdir -p "$dir"
  cat >"$dir/app.py"
  docker rm -f "kl-$name" >/dev/null 2>&1 || true
  # PORT is set per call; FLAG/SECRET/DBUSER/DBPASS are forwarded from the
  # setup environment when present, so the app source need not embed them.
  local fwd=(-e "PORT=$port")
  local v
  for v in FLAG SECRET DBUSER DBPASS; do
    [ -n "${!v:-}" ] && fwd+=(-e "$v=${!v}")
  done
  docker run -d --name "kl-$name" --restart unless-stopped \
    -p "127.0.0.1:$port:$port" "${fwd[@]}" \
    -v "$dir/app.py:/app/app.py:ro" python:3.12-alpine \
    python /app/app.py >/dev/null
  on_reset "docker rm -f kl-$name >/dev/null 2>&1; rm -rf $dir"
  eventually 20 "curl -fsS -o /dev/null http://127.0.0.1:$port/"
}

# rand_flag — a fresh unguessable flag token, e.g. KL{deadbeef...}
rand_flag() { echo "KL{$(head -c16 /dev/urandom | od -An -tx1 | tr -d ' \n')}"; }

# root_flag — plant a fresh flag in /root/flag.txt (root-only), store it as the
# expected answer, schedule its removal on the next reset, and echo it. For
# privilege-escalation tasks: the learner must become root to read it.
root_flag() {
  local f; f=$(rand_flag)
  echo "$f" > /root/flag.txt; chmod 600 /root/flag.txt; chown root:root /root/flag.txt
  echo "$f" > "$KL_STATE/answer"
  on_reset 'rm -f /root/flag.txt'
  echo "$f"
}

# check_flag [FILE] — pass when FILE (default /lab/flag.txt) holds the stored
# answer (a KL{...} token if present, otherwise the file's trimmed contents).
check_flag() {
  local file=${1:-/lab/flag.txt} want got
  want=$(cat "$KL_STATE/answer" 2>/dev/null)
  got=$(grep -oE 'KL\{[0-9a-f]+\}' "$file" 2>/dev/null | head -1)
  [ -z "$got" ] && got=$(tr -d ' \t\r\n' < "$file" 2>/dev/null)
  check_eq "flag captured in $file" "$got" "$want"
}

# ---------- kubernetes helpers ----------
k() { kubectl "$@"; }
# kj RESOURCE... JSONPATH — e.g. kj -n web deploy api '{.spec.replicas}'
kj() {
  local jp="${*: -1}"
  kubectl get "${@:1:$#-1}" -o jsonpath="$jp" 2>/dev/null
}
# kapply — apply a manifest from stdin
kapply() { kubectl apply -f - >/dev/null; }
k_exists() { kubectl get "$@" >/dev/null 2>&1; }
# ready_pods NS SELECTOR -> number of Ready pods
ready_pods() {
  kubectl get pods -n "$1" -l "$2" -o jsonpath='{range .items[*]}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}' 2>/dev/null | grep -c True
}
# k_ns NAME — (re)create a namespace for the exercise
k_ns() { kubectl create namespace "$1" --dry-run=client -o yaml | kubectl apply -f - >/dev/null; }
# k_curl NS URL -> output of curl from a throw-away pod (retried: new
# services take a moment to be programmed into kube-proxy)
k_curl() {
  local out i
  for i in 1 2 3; do
    out=$(kubectl -n "$1" run "kl-curl-$RANDOM" --rm -i --restart=Never --quiet \
      --image=curlimages/curl:8.11.1 --command -- curl -s -m 4 "$2" 2>/dev/null)
    [ -n "$out" ] && { printf '%s\n' "$out"; return 0; }
    sleep 2
  done
  return 1
}
# svc_get NS SERVICE [PATH] [PORT] -> body of an HTTP GET to the Service's ClusterIP, from this
# node (kube-proxy routes it); retried while the Service's endpoints come up
svc_get() {
  local ip out i
  ip=$(kubectl -n "$1" get svc "$2" -o jsonpath='{.spec.clusterIP}' 2>/dev/null)
  [ -n "$ip" ] || return 1
  for i in 1 2 3 4 5; do
    out=$(curl -fsS -m 4 "http://$ip:${4:-80}${3:-/}" 2>/dev/null) && { printf '%s\n' "$out"; return 0; }
    sleep 2
  done
  return 1
}
# restore a kube-system object saved at snapshot time
kl_restore_obj() { # kl_restore_obj configmap coredns
  local f="$KL_STATE/pristine/obj/$1-$2.yaml"
  [ -f "$f" ] && kubectl -n kube-system replace --force -f "$f" >/dev/null 2>&1
}
# api_up — is the API server answering?
api_up() { kubectl get --raw=/readyz >/dev/null 2>&1; }
wait_api() { eventually "${1:-180}" api_up; }
# pod_ready NS NAME -> pod has condition Ready=True
pod_ready() { [ "$(kj -n "$1" pod "$2" '{.status.conditions[?(@.type=="Ready")].status}')" = True ]; }
# _pods_state NS ARGS... -> "ready", "stuck" or "wait" for the selected pods
_pods_state() {
  local ns=$1 j; shift
  j=$(kubectl -n "$ns" get pods "$@" -o json 2>/dev/null) || { echo stuck; return; }
  jq -r <<<"$j" '
    (if .kind == "List" then .items else [.] end) as $p
    | if ($p | length) == 0 then "stuck"
      elif all($p[]; any(.status.conditions[]?; .type == "Ready" and .status == "True")) then "ready"
      elif any($p[]; (.status.phase == "Failed" or .status.phase == "Succeeded")
          or any(.status.conditions[]?; .type == "PodScheduled" and .status == "False")
          or any(.status.containerStatuses[]?, .status.initContainerStatuses[]?;
               (.state.waiting.reason // "") | IN("ImagePullBackOff","ErrImagePull","CrashLoopBackOff","CreateContainerConfigError","InvalidImageName","RunContainerError","CreateContainerError"))
          or any(.status.containerStatuses[]?; .ready == false and .state.running.startedAt != null
               and (now - (.state.running.startedAt | fromdateiso8601)) > 25)) then "stuck"
      else "wait" end' 2>/dev/null || echo stuck
}
# _wait_state SECONDS NS ARGS... — wait for ready; give up early once pods are clearly stuck
_wait_state() {
  local t=$1 ns=$2; shift 2
  local end=$((SECONDS + t)) grace=$((SECONDS + 8)) st
  while :; do
    st=$(_pods_state "$ns" "$@")
    [ "$st" = ready ] && return 0
    [ "$st" = stuck ] && [ $SECONDS -ge $grace ] && return 1
    [ $SECONDS -ge $end ] && return 1
    sleep 2
  done
}
# wait_pod NS NAME [SECONDS] — wait until the pod is Ready (returns early if it is stuck)
wait_pod() { k_exists -n "$1" pod "$2" && _wait_state "${3:-60}" "$1" "$2"; }
# wait_pods NS SELECTOR [SECONDS]
wait_pods() { _wait_state "${3:-60}" "$1" -l "$2"; }
# restarts NS POD -> total container restarts
restarts() { kj -n "$1" pod "$2" '{range .status.containerStatuses[*]}{.restartCount}{"\n"}{end}' | awk '{s+=$1} END {print s+0}'; }
# on_node01 'cmd' — run a command as root on node01 (from the control plane)
on_node01() { sudo -u "$(lab_user)" ssh -o BatchMode=yes node01 sudo bash -c "$(printf '%q' "$*")"; }
# deploy_ready NS NAME -> the deployment's latest spec is fully rolled out and Ready
deploy_ready() {
  local j; j=$(kubectl -n "$1" get deploy "$2" -o json 2>/dev/null) || return 1
  jq -e '(.status.observedGeneration // 0) >= .metadata.generation
    and (.spec.replicas // 1) == (.status.updatedReplicas // 0)
    and (.spec.replicas // 1) == (.status.readyReplicas // 0)
    and (.spec.replicas // 1) == (.status.replicas // 0)' <<<"$j" >/dev/null
}
# wait_deploy NS NAME [SECONDS] — wait for a complete rollout; give up early if pods are stuck
wait_deploy() {
  k_exists -n "$1" deploy "$2" || return 1
  local end=$((SECONDS + ${3:-90})) grace=$((SECONDS + 10)) sel
  sel=$(kubectl -n "$1" get deploy "$2" -o json | jq -r '.spec.selector.matchLabels // {} | to_entries | map("\(.key)=\(.value)") | join(",")')
  while :; do
    deploy_ready "$1" "$2" && return 0
    [ $SECONDS -ge $grace ] && [ "$(_pods_state "$1" -l "$sel")" = stuck ] && return 1
    [ $SECONDS -ge $end ] && return 1
    sleep 2
  done
}
# deploy_images NS NAME -> images of the pod template
deploy_images() { kj -n "$1" deploy "$2" '{.spec.template.spec.containers[*].image}'; }
# ---------- helm helpers ----------
# helm_chart DIR NAME [VERSION] [MESSAGE] -> write a small standard chart: nginx serving
# .Values.message, with a fullname/labels helper, ConfigMap, Deployment, Service and NOTES
helm_chart() {
  local d=$1 n=$2 v=${3:-0.1.0} m=${4:-Hello from Helm}
  mkdir -p "$d/templates"
  cat >"$d/Chart.yaml" <<EOF
apiVersion: v2
name: $n
description: A small web app that serves one page
type: application
version: $v
appVersion: "1.28"
EOF
  cat >"$d/values.yaml" <<EOF
# How many pods to run
replicaCount: 1

image:
  repository: nginx
  tag: 1.28-alpine

# The page the app serves
message: "$m"

service:
  type: ClusterIP
  port: 80
EOF
  cat >"$d/templates/_helpers.tpl" <<'EOF'
{{/* The name of every object: the release name, plus the chart name unless it's already in it */}}
{{- define "__N__.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/* Labels on every object; the Deployment's selector uses them too */}}
{{- define "__N__.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
EOF
  cat >"$d/templates/configmap.yaml" <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ include "__N__.fullname" . }}
  labels:
    {{- include "__N__.labels" . | nindent 4 }}
data:
  index.html: {{ .Values.message | quote }}
EOF
  cat >"$d/templates/deployment.yaml" <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "__N__.fullname" . }}
  labels:
    {{- include "__N__.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "__N__.labels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "__N__.labels" . | nindent 8 }}
      annotations:
        checksum/page: {{ .Values.message | sha256sum }}
    spec:
      containers:
      - name: web
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        ports:
        - containerPort: 80
        readinessProbe:
          httpGet:
            path: /
            port: 80
        volumeMounts:
        - name: page
          mountPath: /usr/share/nginx/html
      volumes:
      - name: page
        configMap:
          name: {{ include "__N__.fullname" . }}
EOF
  cat >"$d/templates/service.yaml" <<'EOF'
apiVersion: v1
kind: Service
metadata:
  name: {{ include "__N__.fullname" . }}
  labels:
    {{- include "__N__.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  selector:
    {{- include "__N__.labels" . | nindent 4 }}
  ports:
  - port: {{ .Values.service.port }}
    targetPort: 80
EOF
  cat >"$d/templates/NOTES.txt" <<'EOF'
{{ .Release.Name }} is installed in namespace {{ .Release.Namespace }}.

Try it:
  kubectl -n {{ .Release.Namespace }} port-forward svc/{{ include "__N__.fullname" . }} 8080:{{ .Values.service.port }}
  curl http://localhost:8080/
EOF
  sed -i "s/__N__/$n/g" "$d"/templates/*
  case "$d" in "$LAB"/*) chown -R "$(lab_user)": "$LAB" ;; esac
}

# ---------- Gitea helpers (Git track: LFS, credentials, pull requests; k8s: GitOps) ----------
# GITEA_URL is where the lab's Gitea answers: on the Docker host it runs in a container
# (gitea_start), on the Kubernetes lab in the cluster behind NodePort 30300 (kgitea_start).
if [ -z "${GITEA_URL:-}" ]; then
  if [ -d /etc/kubernetes ]; then GITEA_URL=http://node01:30300; else GITEA_URL=http://localhost:3000; fi
fi
# gitea_start -> a Gitea server on http://localhost:3000 (also reachable from the host's
# browser through Lima's port forwarding), with LFS, no self-registration, and the admin
# account lab-admin (password in $KL_STATE/gitea-admin). Containers go with the host reset.
gitea_start() {
  local pw
  docker rm -f kl-gitea >/dev/null 2>&1 || true
  docker run -d --name kl-gitea --network host \
    -e GITEA__security__INSTALL_LOCK=true -e GITEA__database__DB_TYPE=sqlite3 \
    -e GITEA__server__HTTP_PORT=3000 -e GITEA__server__ROOT_URL=http://localhost:3000/ -e GITEA__server__DOMAIN=localhost \
    -e GITEA__server__LFS_START_SERVER=true -e GITEA__server__START_SSH_SERVER=false -e GITEA__server__DISABLE_SSH=true \
    -e GITEA__service__DISABLE_REGISTRATION=true -e GITEA__repository__DEFAULT_BRANCH=main \
    -e GITEA__log__LEVEL=Warn -e GITEA__security__PASSWORD_COMPLEXITY=off \
    gitea/gitea:28.2.0-rootless >/dev/null || return 1
  eventually 90 'curl -fsS -m 2 $GITEA_URL/api/healthz >/dev/null' || return 1
  pw="Adm-$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  docker exec kl-gitea gitea admin user create --username lab-admin --password "$pw" \
    --email admin@lab.test --admin --must-change-password=false >/dev/null || return 1
  printf '%s\n' "$pw" >"$KL_STATE/gitea-admin"; chmod 600 "$KL_STATE/gitea-admin"
}
# git_home_guard -> the next reset puts the lab user's ~/.gitconfig back as it is now and
# removes ~/.git-credentials and any credential-cache daemon (the reset doesn't restore home)
git_home_guard() {
  local h; h=$(lab_home)
  rm -f "$KL_STATE/gitconfig.orig"
  [ -f "$h/.gitconfig" ] && cp -p "$h/.gitconfig" "$KL_STATE/gitconfig.orig"
  on_reset "h=\$(lab_home); if [ -f $KL_STATE/gitconfig.orig ]; then cp -p $KL_STATE/gitconfig.orig \$h/.gitconfig; else rm -f \$h/.gitconfig; fi; rm -rf \$h/.git-credentials \$h/.cache/git/credential \$h/.config/git/credentials; pkill -u $(lab_user) -f git-credential-cache--daemon"
}
# gitea_api METHOD PATH [JSON] -> call the API as lab-admin (PATH after /api/v1), print the body
gitea_api() {
  curl -fsS -m 10 -u "lab-admin:$(cat "$KL_STATE/gitea-admin")" -X "$1" -H 'Content-Type: application/json' \
    "$GITEA_URL/api/v1$2" ${3:+-d "$3"}
}
# gitea_as USER PASS METHOD PATH [JSON] -> the same, as another user
gitea_as() {
  curl -fsS -m 10 -u "$1:$2" -X "$3" -H 'Content-Type: application/json' "$GITEA_URL/api/v1$4" ${5:+-d "$5"}
}
# gitea_user NAME PASSWORD [FULL NAME] -> a user account (email NAME@lab.test)
gitea_user() {
  gitea_api POST /admin/users "$(jq -nc --arg u "$1" --arg p "$2" --arg f "${3:-$1}" \
    '{username: $u, password: $p, email: "\($u)@lab.test", full_name: $f, must_change_password: false}')" >/dev/null
}
# gitea_repo OWNER NAME [private] -> an empty repository owned by a user or an organisation
gitea_repo() {
  local p=false; [ "${3:-}" = private ] && p=true
  gitea_api POST "/admin/users/$1/repos" "{\"name\":\"$2\",\"default_branch\":\"main\",\"private\":$p}" >/dev/null 2>&1 ||
    gitea_api POST "/orgs/$1/repos" "{\"name\":\"$2\",\"default_branch\":\"main\",\"private\":$p}" >/dev/null
}
# gitea_url OWNER/REPO [USER:PASS] -> its clone URL (with credentials only when given, for setups)
gitea_url() { if [ -n "${2:-}" ]; then echo "http://${2}@${GITEA_URL#http://}/$1.git"; else echo "$GITEA_URL/$1.git"; fi; }
# gitea_token USER PASS NAME -> create an access token (scopes: repository and issue write, user read)
gitea_token() {
  gitea_as "$1" "$2" POST "/users/$1/tokens" "{\"name\":\"$3\",\"scopes\":[\"write:repository\",\"write:issue\",\"read:user\"]}" | jq -r .sha1
}

# gitea_dev -> the learner's Gitea account "dev" (password in $KL_STATE/dev-pass), and
# /lab/.gitea-credentials, a credential-store file the lab's clones use (see gitea_clone)
gitea_dev() {
  local pw="dev-$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  gitea_user dev "$pw" "Lab Developer" || return 1
  printf '%s\n' "$pw" >"$KL_STATE/dev-pass"
  printf 'http://dev:%s@%s\n' "$pw" "${GITEA_URL#http://}" | lab_file /lab/.gitea-credentials 0600
}
# gitea_clone OWNER/REPO DIR [git clone options] -> a clone as the lab user that authenticates
# with /lab/.gitea-credentials (repo-local credential.helper), with a lab identity
gitea_clone() {
  local r=$1 d=$2; shift 2
  sudo -u "$(lab_user)" -H git -c credential.helper="store --file=/lab/.gitea-credentials" \
    clone -q "$@" "$GITEA_URL/$r.git" "$d" || return 1
  sudo -u "$(lab_user)" -H git -C "$d" config credential.helper "store --file=/lab/.gitea-credentials"
  sudo -u "$(lab_user)" -H git -C "$d" config user.name "Lab Developer"
  sudo -u "$(lab_user)" -H git -C "$d" config user.email dev@lab.test
}
# gitea_mirror OWNER/REPO DIR -> a bare mirror of the server's repository, as lab-admin (for checks)
gitea_mirror() {
  rm -rf "$2"
  git clone -q --mirror "$(gitea_url "$1" "lab-admin:$(cat "$KL_STATE/gitea-admin")")" "$2" 2>/dev/null
}
# gitea_media OWNER/REPO PATH [REF] -> the file's real content as served by Gitea (LFS resolved)
gitea_media() {
  curl -fsS -m 20 -u "lab-admin:$(cat "$KL_STATE/gitea-admin")" "$GITEA_URL/api/v1/repos/$1/media/$2?ref=${3:-main}"
}
# is_lfs_pointer GITDIR REV:PATH -> the blob is a Git LFS pointer
is_lfs_pointer() { git --git-dir="$1" cat-file -p "$2" 2>/dev/null | head -1 | grep -qx 'version https://git-lfs.github.com/spec/v1'; }
# max_blob GITDIR -> the size in bytes of the biggest blob reachable from any ref
max_blob() {
  git --git-dir="$1" rev-list --objects --all 2>/dev/null |
    git --git-dir="$1" cat-file --batch-check='%(objecttype) %(objectsize)' 2>/dev/null |
    awk '$1 == "blob" && $2 > m {m = $2} END {print m + 0}'
}

# kgitea_start -> Gitea in the cluster (namespace gitea, NodePort 30300 on the nodes, Service
# gitea.gitea.svc:3000 for pods such as Argo CD), with the admin lab-admin like gitea_start
kgitea_start() {
  local pw
  k_ns gitea
  kapply <<'EOF'
apiVersion: apps/v1
kind: Deployment
metadata: {name: gitea, namespace: gitea}
spec:
  selector: {matchLabels: {app: gitea}}
  template:
    metadata: {labels: {app: gitea}}
    spec:
      containers:
      - name: gitea
        image: gitea/gitea:28.2.0-rootless
        env:
        - {name: GITEA__security__INSTALL_LOCK, value: "true"}
        - {name: GITEA__database__DB_TYPE, value: sqlite3}
        - {name: GITEA__server__ROOT_URL, value: "http://node01:30300/"}
        - {name: GITEA__server__HTTP_PORT, value: "3000"}
        - {name: GITEA__server__DISABLE_SSH, value: "true"}
        - {name: GITEA__server__START_SSH_SERVER, value: "false"}
        - {name: GITEA__service__DISABLE_REGISTRATION, value: "true"}
        - {name: GITEA__repository__DEFAULT_BRANCH, value: main}
        - {name: GITEA__security__PASSWORD_COMPLEXITY, value: "off"}
        - {name: GITEA__log__LEVEL, value: Warn}
        ports: [{containerPort: 3000}]
        readinessProbe: {httpGet: {path: /api/healthz, port: 3000}}
---
apiVersion: v1
kind: Service
metadata: {name: gitea, namespace: gitea}
spec:
  type: NodePort
  selector: {app: gitea}
  ports: [{port: 3000, targetPort: 3000, nodePort: 30300}]
EOF
  wait_deploy gitea gitea 300 || return 1
  eventually 60 "curl -fsS -m 2 $GITEA_URL/api/healthz >/dev/null" || return 1
  pw="Adm-$(head -c8 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  kubectl -n gitea exec deploy/gitea -- gitea admin user create --username lab-admin --password "$pw" \
    --email admin@lab.test --admin --must-change-password=false >/dev/null || return 1
  printf '%s\n' "$pw" >"$KL_STATE/gitea-admin"; chmod 600 "$KL_STATE/gitea-admin"
}
# kgitea_seed OWNER/REPO DIR MESSAGE -> create the repository and push DIR's files to its main
kgitea_seed() {
  local t; t=$(mktemp -d)
  gitea_repo "${1%%/*}" "${1#*/}" || return 1
  cp -r "$2"/. "$t"/ && git -C "$t" init -q -b main && git -C "$t" add -A &&
    git -C "$t" -c user.name="Lab Developer" -c user.email=dev@lab.test commit -qm "$3" &&
    git -C "$t" push -q "$(gitea_url "$1" "dev:$(cat "$KL_STATE/dev-pass")")" main 2>/dev/null
  local rc=$?; rm -rf "$t"; return $rc
}
# argo_install -> Argo CD v3.5.7 in namespace argocd (bundled manifest, without Dex and
# notifications), ready, with its default AppProject
argo_install() {
  k_ns argocd
  zcat /opt/kubelab/argocd-v3.5.7.yaml.gz | kubectl -n argocd apply --server-side --force-conflicts -f - >/dev/null || return 1
  # look at Git every 30 seconds instead of the default 3 minutes, so labs don't wait
  kubectl -n argocd patch configmap argocd-cm --type merge -p '{"data":{"timeout.reconciliation":"30s"}}' >/dev/null
  kubectl -n argocd rollout restart statefulset/argocd-application-controller deploy/argocd-repo-server >/dev/null 2>&1
  kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=300s >/dev/null &&
    kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=300s >/dev/null &&
    kubectl -n argocd rollout status deploy/argocd-server --timeout=300s >/dev/null || return 1
  eventually 60 'kubectl -n argocd get appproject default'
}
# argo_status APP -> "SYNC HEALTH" of an Argo CD Application, e.g. "Synced Healthy"
argo_status() { kubectl -n argocd get application "$1" -o jsonpath='{.status.sync.status} {.status.health.status}' 2>/dev/null; }
# argo_ok APP [SECONDS] -> wait until the Application is Synced and Healthy
argo_ok() { eventually "${2:-90}" "[ \"\$(argo_status $1)\" = 'Synced Healthy' ]"; }

# ---------- Kyverno and Prometheus helpers ----------
# kyverno_install -> Kyverno v1.19.2 (bundled manifest) in namespace kyverno, webhooks answering
kyverno_install() {
  # policy exceptions are off by default; the exceptions exercise needs them
  zcat /opt/kubelab/kyverno-v1.19.2.yaml.gz | sed 's/--enablePolicyException=false/--enablePolicyException=true/' |
    kubectl apply --server-side --force-conflicts -f - >/dev/null || return 1
  local d
  for d in kyverno-admission-controller kyverno-background-controller kyverno-cleanup-controller kyverno-reports-controller; do
    kubectl -n kyverno rollout status deploy/$d --timeout=300s >/dev/null || return 1
  done
  # the webhooks answer a little after the pods are Ready
  eventually 120 'kubectl apply --dry-run=server -f - >/dev/null 2>&1 <<EOF
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata: {name: kl-probe}
spec:
  rules:
  - name: probe
    match: {any: [{resources: {kinds: [ConfigMap], names: [kl-probe-none]}}]}
    validate: {message: probe, pattern: {metadata: {name: "?*"}}}
EOF'
}
# prom_install -> the Prometheus Operator v0.94.2 and kube-state-metrics in namespace
# monitoring, plus a Prometheus "lab" (v3.15.1) that picks up ServiceMonitors and
# PrometheusRules labelled release=lab in any namespace, reachable as Service prometheus:9090
prom_install() {
  k_ns monitoring
  zcat /opt/kubelab/prometheus-operator-v0.94.2.yaml.gz | sed -E 's/^( +)namespace: default$/\1namespace: monitoring/' |
    kubectl apply --server-side --force-conflicts -f - >/dev/null || return 1
  zcat /opt/kubelab/kube-state-metrics-v2.20.0.yaml.gz | kubectl apply -f - >/dev/null || return 1
  kubectl -n monitoring rollout status deploy/prometheus-operator --timeout=300s >/dev/null || return 1
  kapply <<'EOF'
apiVersion: v1
kind: ServiceAccount
metadata: {name: prometheus, namespace: monitoring}
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata: {name: lab-prometheus}
rules:
- apiGroups: [""]
  resources: [nodes, nodes/metrics, services, endpoints, pods]
  verbs: [get, list, watch]
- apiGroups: [discovery.k8s.io]
  resources: [endpointslices]
  verbs: [get, list, watch]
- nonResourceURLs: [/metrics]
  verbs: [get]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata: {name: lab-prometheus}
roleRef: {apiGroup: rbac.authorization.k8s.io, kind: ClusterRole, name: lab-prometheus}
subjects: [{kind: ServiceAccount, name: prometheus, namespace: monitoring}]
---
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata: {name: lab, namespace: monitoring}
spec:
  image: quay.io/prometheus/prometheus:v3.15.1
  version: v3.15.1
  replicas: 1
  serviceAccountName: prometheus
  scrapeInterval: 15s
  evaluationInterval: 15s
  retention: 2h
  serviceMonitorSelector: {matchLabels: {release: lab}}
  serviceMonitorNamespaceSelector: {}
  podMonitorSelector: {matchLabels: {release: lab}}
  podMonitorNamespaceSelector: {}
  ruleSelector: {matchLabels: {release: lab}}
  ruleNamespaceSelector: {}
  resources: {requests: {memory: 150Mi}}
---
apiVersion: v1
kind: Service
metadata: {name: prometheus, namespace: monitoring}
spec:
  selector: {app.kubernetes.io/name: prometheus, prometheus: lab}
  ports: [{name: web, port: 9090, targetPort: 9090}]
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: kube-state-metrics, namespace: monitoring, labels: {release: lab}}
spec:
  selector: {matchLabels: {app.kubernetes.io/name: kube-state-metrics}}
  endpoints: [{port: http-metrics, honorLabels: true}]
EOF
  eventually 240 'kubectl -n monitoring get pod prometheus-lab-0 >/dev/null 2>&1' || return 1
  wait_pod monitoring prometheus-lab-0 240 && wait_deploy monitoring kube-state-metrics 180
  eventually 120 '[ "$(prom_query up | jq -r ".data.result | length")" -gt 0 ]'
}
# prom_query 'PROMQL' -> Prometheus's JSON answer (instant query)
prom_query() {
  local ip; ip=$(kubectl -n monitoring get svc prometheus -o jsonpath='{.spec.clusterIP}' 2>/dev/null) || return 1
  curl -fsS -m 10 -G "http://$ip:9090/api/v1/query" --data-urlencode "query=$1" 2>/dev/null
}
# prom_value 'PROMQL' -> the first result's value, or nothing
prom_value() { prom_query "$1" | jq -r '.data.result[0].value[1] // empty'; }
# prom_api PATH -> any Prometheus API path, e.g. /api/v1/alerts or /api/v1/targets
prom_api() {
  local ip; ip=$(kubectl -n monitoring get svc prometheus -o jsonpath='{.spec.clusterIP}' 2>/dev/null) || return 1
  curl -fsS -m 10 "http://$ip:9090$1" 2>/dev/null
}

# ---------- cert-manager helpers ----------
CM_VERSION=v1.21.3
CM_CHART=/opt/kubelab/cert-manager-$CM_VERSION.tgz
# cm_cleanup — remove what a cert-manager install leaves outside its own namespace: the
# leader-election Roles, RoleBindings and Leases in kube-system. The controllers are stopped
# first, or they would take the Leases again. Registered by cm_install (and by exercises
# where the learner installs it).
cm_cleanup() {
  kubectl delete deploy -A -l "helm.sh/chart=cert-manager-$CM_VERSION" --wait=true --timeout=60s >/dev/null 2>&1
  kubectl -n kube-system delete role,rolebinding -l "helm.sh/chart=cert-manager-$CM_VERSION" --ignore-not-found >/dev/null 2>&1
  kubectl -n kube-system delete lease cert-manager-controller cert-manager-cainjector-leader-election --ignore-not-found >/dev/null 2>&1
}
# cm_webhook_ok — does the cert-manager webhook answer? (a server-side dry run goes through it)
cm_webhook_ok() {
  kubectl apply --dry-run=server -f - >/dev/null 2>&1 <<'EOF'
apiVersion: cert-manager.io/v1
kind: Issuer
metadata: {name: kl-probe, namespace: default}
spec: {selfSigned: {}}
EOF
}
# cm_install — install cert-manager (release cert-manager, namespace cert-manager, CRDs
# included) from the bundled chart and wait until its webhook answers
cm_install() {
  on_reset cm_cleanup
  helm install cert-manager "$CM_CHART" -n cert-manager --create-namespace \
    --set crds.enabled=true --set startupapicheck.enabled=false \
    --set global.leaderElection.namespace=cert-manager --wait --timeout 6m >/dev/null || return 1
  eventually 120 cm_webhook_ok
}
# cm_lab_ca — a lab CA: ClusterIssuer "selfsigned", the CA certificate cert-manager/lab-root
# (Secret lab-root) and the ClusterIssuer "lab-ca" that signs with it
cm_lab_ca() {
  kapply <<'EOF'
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: {name: selfsigned}
spec: {selfSigned: {}}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: lab-root, namespace: cert-manager}
spec:
  isCA: true
  commonName: Lab Root CA
  secretName: lab-root
  duration: 87600h
  privateKey: {algorithm: ECDSA, size: 256}
  issuerRef: {name: selfsigned, kind: ClusterIssuer}
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata: {name: lab-ca}
spec: {ca: {secretName: lab-root}}
EOF
  kubectl -n cert-manager wait --for=condition=Ready certificate/lab-root --timeout=90s >/dev/null &&
    kubectl wait --for=condition=Ready clusterissuer/lab-ca --timeout=90s >/dev/null
}
# cm_ready NS NAME — the Certificate is Ready for its current spec
cm_ready() {
  kubectl -n "$1" get certificate "$2" -o json 2>/dev/null | jq -e '.metadata.generation as $g
    | any(.status.conditions[]?; .type == "Ready" and .status == "True" and (.observedGeneration // $g) >= $g)' >/dev/null
}
# cm_issuer_ready clusterissuer NAME | issuer NAME NS
cm_issuer_ready() {
  [ "$(kubectl get "$1" "$2" ${3:+-n "$3"} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null)" = True ]
}
# cm_crt NS SECRET [KEY] -> a PEM from a TLS Secret (tls.crt by default)
cm_crt() { kubectl -n "$1" get secret "$2" -o json 2>/dev/null | jq -r --arg k "${3:-tls.crt}" '.data[$k] // empty' | base64 -d 2>/dev/null; }
# cm_sans NS SECRET -> the certificate's SANs, one per line, sorted ("DNS:a.test", "IP Address:10.0.0.1")
cm_sans() { cm_crt "$1" "$2" | openssl x509 -noout -ext subjectAltName 2>/dev/null | tail -n +2 | tr ',' '\n' | sed 's/^ *//' | grep -v '^$' | sort; }
# cm_serial NS SECRET / cm_pubkey NS SECRET -> serial number / SHA-256 of the public key
cm_serial() { cm_crt "$1" "$2" | openssl x509 -noout -serial 2>/dev/null | cut -d= -f2; }
cm_pubkey() { cm_crt "$1" "$2" | openssl x509 -noout -pubkey 2>/dev/null | sha256sum | cut -d' ' -f1; }
# cm_signed_by NS SECRET CAFILE — the Secret's certificate verifies against the CA in CAFILE
cm_signed_by() {
  local t; t=$(mktemp)
  cm_crt "$1" "$2" >"$t"
  [ -s "$t" ] && openssl verify -CAfile "$3" "$t" >/dev/null 2>&1; local rc=$?
  rm -f "$t"; return $rc
}

# ---------- docker compose helpers ----------
# cc_ids PROJECT SERVICE -> ids of running containers of a compose service
cc_ids() { docker ps -q --filter "label=com.docker.compose.project=$1" --filter "label=com.docker.compose.service=$2"; }
cc_id() { cc_ids "$1" "$2" | head -1; }
cc_running() { [ -n "$(cc_ids "$1" "$2")" ]; }
cc_count() { cc_ids "$1" "$2" | wc -l | tr -d ' '; }
# cc_get PROJECT SERVICE '{{template}}' — inspect the first container of a service
cc_get() { local id; id=$(cc_id "$1" "$2"); [ -n "$id" ] && docker inspect -f "$3" "$id"; }
# cc_exec PROJECT SERVICE cmd... — exec in the first container of a service
cc_exec() { local p=$1 s=$2 id; shift 2; id=$(cc_id "$p" "$s"); [ -n "$id" ] && docker exec "$id" "$@"; }
# ---------- ingress / gateway helpers (Traefik NodePorts: web 30080, websecure 30443) ----------
# ing_get HOST PATH [curl args...] — HTTP request through the ingress controller
ing_get() { local h=$1 p=$2; shift 2; curl -s -m 5 -H "Host: $h" "$@" "http://node01:30080$p"; }
# ing_code HOST PATH -> HTTP status code through the ingress controller
ing_code() { curl -s -o /dev/null -m 5 -w '%{http_code}' -H "Host: $1" "http://node01:30080$2"; }
# ing_tls HOST PATH [curl args...] — HTTPS through the ingress controller (SNI = HOST)
ing_tls() { local h=$1 p=$2; shift 2; curl -s -m 5 --resolve "$h:30443:$(getent hosts node01 | awk '{print $1}')" "$@" "https://$h:30443$p"; }

# ---------- edge router helpers (kubernetes) ----------
# An FRR router as Pod "router" in namespace "edge" on the control plane. calico-node's BIRD
# holds TCP 179 on the node, so the router has its own network namespace. It is cabled to the
# node network with a macvlan interface (net0, .200) so that both nodes reach it directly, and
# its Calico eth0 is brought down: it behaves as a device outside the cluster. A second macvlan
# (kl-shim) lets the control plane itself reach the router, which macvlan otherwise forbids.
# kl-shim must never take part in ARP: it would answer for the node's own address with its own
# MAC, the rest of the network would believe it, and removing the shim then cuts the node off.
# So it ignores ARP, and both ends of that one link use static neighbor entries.
# Sidecar "web" (traefik/whoami) answers on the router on port 80 and reports the client address.
KR_NS=edge
KR_MAC=02:4b:4c:00:00:c8
KR_SHIM_MAC=02:4b:4c:00:00:c9
# kr_ip -> the router's address on the node network
# /etc/hosts also maps the VM's own name to 127.0.1.1, so ask the API for node addresses
kr_node_ip() { kubectl get node "$1" -o jsonpath='{.status.addresses[?(@.type=="InternalIP")].address}'; }
kr_ip() { local cp; cp=$(kr_node_ip controlplane); echo "${cp%.*}.200"; }
# kr_nodes -> the node addresses, control plane first
kr_nodes() { kr_node_ip controlplane; echo; kr_node_ip node01; echo; }
# kr_x CMD... -> run a command in the router (network namespace and all); stdin stays free
kr_x() { kubectl -n "$KR_NS" exec router -c frr -- "$@" </dev/null; }
# kr_json 'show ... json' -> run a vtysh show command on the router
kr_json() { kr_x vtysh -c "$1" 2>/dev/null; }
# kr_healthy -> startup finished and every daemon runs (see frr_healthy for why)
kr_healthy() {
  kubectl -n "$KR_NS" logs router -c frr 2>/dev/null |
    awk '/all daemons up/{u=NR} /state -> down|Forked background/{d=NR} END{exit !(u>d)}' || return 1
  kr_x sh -c 'ps -o args | grep -q "[w]atchfrr\.sh" && exit 1; for d in zebra mgmtd bgpd staticd; do pidof $d >/dev/null || exit 1; done' >/dev/null 2>&1
}
# kr_pod_start -> create the Pod, plug the macvlan into its namespace, then let FRR start.
# FRR waits for /tmp/kl-go so zebra starts with the final set of interfaces.
kr_pod_start() {
  local ip cp dev sandbox cid pid i
  ip=$(kr_ip)
  cp=$(kr_node_ip controlplane)
  dev=$(ip route get "$(kr_node_ip node01)" | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -1)
  kubectl -n "$KR_NS" delete pod router --force --grace-period=0 --ignore-not-found >/dev/null 2>&1
  kubectl -n "$KR_NS" wait --for=delete pod/router --timeout=60s >/dev/null 2>&1
  kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Pod
metadata: {name: router, namespace: $KR_NS, labels: {app: router}}
spec:
  nodeName: controlplane
  restartPolicy: Never
  automountServiceAccountToken: false
  terminationGracePeriodSeconds: 0
  tolerations: [{operator: Exists}]
  containers:
  - name: frr
    image: $FRR_IMAGE
    securityContext: {privileged: true}
    command: ["sh", "-c", "until [ -e /tmp/kl-go ]; do sleep 1; done; sed -i 's/^bgpd=no/bgpd=yes/' /etc/frr/daemons; touch /etc/frr/vtysh.conf; exec /sbin/tini -- /usr/lib/frr/docker-start"]
  - name: web
    image: traefik/whoami:v1.10
YAML
  kubectl -n "$KR_NS" wait --for=condition=Ready pod/router --timeout=180s >/dev/null || return 1
  for i in 1 2 3 4 5 6 7 8 9 10; do
    sandbox=$(crictl pods --namespace "$KR_NS" --name '^router$' --state ready -q | head -1)
    cid=$(crictl ps --pod "$sandbox" --name '^frr$' --state running -q | head -1)
    pid=$(crictl inspect "$cid" 2>/dev/null | jq -r '.info.pid // empty')
    [ -n "$pid" ] && break
    sleep 1
  done
  [ -n "$pid" ] || return 1
  ip link add kr-net0 link "$dev" address "$KR_MAC" type macvlan mode bridge || return 1
  ip link set kr-net0 netns "$pid" || return 1
  nsenter -t "$pid" -n sh -c "ip link set kr-net0 name net0 && ip addr add $ip/24 dev net0 && ip link set net0 up && ip neigh replace $cp lladdr $KR_SHIM_MAC dev net0 nud permanent && ip link set eth0 down && ip addr add 198.51.100.1/24 dev lo && sysctl -qw net.ipv4.ip_forward=1" || return 1
  kr_x touch /tmp/kl-go
}
# kr_up -> run the router and wait for it; recreate it if zebra was lost at boot
kr_up() {
  local round ok ip cp dev
  ip=$(kr_ip)
  cp=$(kr_node_ip controlplane)
  dev=$(ip route get "$(kr_node_ip node01)" | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -1)
  k_ns "$KR_NS"
  ip link del kl-shim 2>/dev/null
  ip link add kl-shim link "$dev" address "$KR_SHIM_MAC" type macvlan mode bridge
  sysctl -qw net.ipv4.conf.kl-shim.arp_ignore=8 net.ipv6.conf.kl-shim.disable_ipv6=1
  ip link set kl-shim up
  ip neigh replace "$ip" lladdr "$KR_MAC" dev kl-shim nud permanent
  ip route replace "$ip/32" dev kl-shim src "$cp"
  on_reset 'ip link del kl-shim 2>/dev/null'
  for round in 1 2 3 4 5; do
    kr_pod_start || { echo "kr_up: pod did not start (round $round)" >&2; continue; }
    ok=
    if eventually 40 kr_healthy; then
      sleep 5
      kr_healthy && ok=1
    fi
    [ -n "$ok" ] && break
    echo "kr_up: recreating the router (round $round)" >&2
  done
  [ -n "$ok" ] || { echo "edge router did not start" >&2; return 1; }
  # ARP settles now rather than during the first BGP connection
  eventually 20 "ping -c1 -W1 $ip" || true
  on_node01 "ping -c2 -W1 $ip" >/dev/null 2>&1 || true
}
# kr_conf <<EOF -> apply vtysh configuration from stdin ("configure terminal" is implied)
kr_conf() {
  local conf out rc i
  conf=$(cat)
  for i in 1 2 3 4 5; do
    out=$(printf '%s\n' "$conf" | kubectl -n "$KR_NS" exec -i router -c frr -- sh -c 'cat >/tmp/kl.conf; vtysh -f /tmp/kl.conf' 2>&1); rc=$?
    if [ "$rc" = 0 ] && ! grep -qE '^% |failure|not running|Can.t' <<<"$out"; then return 0; fi
    echo "kr_conf attempt $i failed (rc=$rc): $(grep -vE 'sending configuration|\] done|Waiting for children' <<<"$out" | head -5)" >&2
    sleep 2
  done
  return 1
}
# kr_estab NEIGHBOR -> the BGP session to that neighbor is Established
kr_estab() { [ "$(kr_json 'show bgp summary json' | jq -r --arg n "$1" '.ipv4Unicast.peers[$n].state // empty')" = Established ]; }
# kr_hops PREFIX -> next-hop addresses of the selected route, sorted, comma-separated
kr_hops() {
  kr_json "show ip route $1 json" | jq -r --arg p "$1" \
    '[.[$p][]? | select(.selected==true) | .nexthops[]? | select(.active==true) | .ip] | sort | join(",")' 2>/dev/null
}
# kr_wget URL -> HTTP from the router itself (its source address is the router's .200)
kr_wget() { kr_x wget -qO- -T 4 "$1" 2>/dev/null; }
# kr_guard_calico -> remember Calico's BGP settings now and remove what a lab adds on the next
# reset. The cluster reset leaves custom resources alone, so this is not optional.
kr_guard_calico() {
  kubectl get bgpconfigurations.crd.projectcalico.org default -o json | jq -c .spec >"$KL_STATE/kr-bgpconf.json"
  kubectl get ippools.crd.projectcalico.org default-ipv4-ippool -o json | jq -c .spec >"$KL_STATE/kr-ippool.json"
  on_reset 'kubectl delete bgppeers.crd.projectcalico.org,bgpfilters.crd.projectcalico.org --all --ignore-not-found --wait=false
kubectl get ippools.crd.projectcalico.org -o name | grep -v /default-ipv4-ippool$ | xargs -r kubectl delete --wait=false
for t in "bgpconfigurations.crd.projectcalico.org default kr-bgpconf" "ippools.crd.projectcalico.org default-ipv4-ippool kr-ippool"; do set -- $t
  [ -s "$KL_STATE/$3.json" ] && kubectl patch "$1" "$2" --type=json -p "[{\"op\":\"replace\",\"path\":\"/spec\",\"value\":$(cat "$KL_STATE/$3.json")}]"
  rm -f "$KL_STATE/$3.json"
done'
}
