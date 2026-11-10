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
