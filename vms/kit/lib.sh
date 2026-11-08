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
