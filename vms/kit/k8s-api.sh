#!/bin/bash
# Cluster-level pristine snapshot / reset. Runs on the control plane as root.
# usage: k8s-api.sh snapshot|reset
set -uo pipefail
source /opt/kubelab/lib.sh
export KUBECONFIG=/etc/kubernetes/admin.conf
P=$KL_STATE/pristine/api
CLUSTER_KINDS="namespaces clusterroles clusterrolebindings persistentvolumes storageclasses priorityclasses customresourcedefinitions validatingwebhookconfigurations mutatingwebhookconfigurations validatingadmissionpolicies validatingadmissionpolicybindings ingressclasses runtimeclasses certificatesigningrequests"
SYS_OBJS="configmap/coredns deployment/coredns configmap/kube-proxy daemonset/kube-proxy"
NS_KINDS="deployments,statefulsets,daemonsets,replicasets,jobs,cronjobs,pods,services,configmaps,secrets,serviceaccounts,roles,rolebindings,networkpolicies,ingresses,persistentvolumeclaims,poddisruptionbudgets,horizontalpodautoscalers,resourcequotas,limitranges,leases,endpointslices"

objhash() { kubectl -n kube-system get "$1" -o jsonpath='{.spec.template.spec}{.spec.replicas}{.data}' 2>/dev/null | sha256sum | cut -d' ' -f1; }

snapshot() {
  rm -rf "$P" && mkdir -p "$P" "$KL_STATE/pristine/obj"
  local k o
  for k in $CLUSTER_KINDS; do
    kubectl get "$k" -o name 2>/dev/null | sort >"$P/$k.txt"
  done
  for o in $SYS_OBJS; do
    kubectl -n kube-system get "$o" -o yaml | grep -vE '^\s+(resourceVersion|uid|creationTimestamp|generation):' \
      >"$KL_STATE/pristine/obj/${o%/*}-${o#*/}.yaml"
    objhash "$o" >"$P/$(echo "$o" | tr / -).sum"
  done
  kubectl get nodes -o json | jq -c '[.items[] | {name: .metadata.name, labels: .metadata.labels, taints: (.spec.taints // [])}]' >"$P/nodes.json"
  echo "api snapshot saved"
}

reset() {
  wait_api 240 || { echo "API server not reachable" >&2; exit 1; }
  local k o n
  # kube-system core objects
  for o in $SYS_OBJS; do
    if [ "$(objhash "$o")" != "$(cat "$P/$(echo "$o" | tr / -).sum")" ]; then
      kl_restore_obj "${o%/*}" "${o#*/}" && echo "restored kube-system $o"
    fi
  done
  # exercise clean-up hooks (after core objects are back)
  /opt/kubelab/hooks.sh
  # nodes: labels, taints, schedulability
  jq -c '.[]' "$P/nodes.json" | while read -r node; do
    n=$(jq -r .name <<<"$node")
    kubectl uncordon "$n" >/dev/null 2>&1
    want=$(jq -c .labels <<<"$node")
    have=$(kubectl get node "$n" -o json | jq -c .metadata.labels)
    for key in $(jq -rn --argjson w "$want" --argjson h "$have" '$h | keys[] | select($w[.] == null)'); do
      kubectl label node "$n" "$key-" >/dev/null
    done
    jq -rn --argjson w "$want" --argjson h "$have" '$w | to_entries[] | select($h[.key] != .value) | "\(.key)=\(.value)"' |
      xargs -r kubectl label node "$n" --overwrite >/dev/null
    kubectl patch node "$n" --type=merge -p "{\"spec\":{\"taints\":$(jq -c .taints <<<"$node")}}" >/dev/null
  done
  # exercise pods die immediately (no 30s grace period)
  kubectl get ns -o name | sort >/tmp/kl-now
  for n in $(comm -13 "$P/namespaces.txt" /tmp/kl-now) namespace/default namespace/kube-public; do
    kubectl -n "${n#namespace/}" delete pods --all --grace-period=0 --force --wait=false >/dev/null 2>&1
  done
  # namespaced junk in default + kube-public
  for n in default kube-public; do
    kubectl -n "$n" delete "$NS_KINDS" --all --wait=false --ignore-not-found >/dev/null 2>&1
  done
  # cluster-scoped objects that were not there at snapshot time
  for k in $CLUSTER_KINDS; do
    kubectl get "$k" -o name 2>/dev/null | sort >/tmp/kl-now
    comm -13 "$P/$k.txt" /tmp/kl-now | xargs -r kubectl delete --wait=false >/dev/null 2>&1
  done
  # wait for extra namespaces to vanish; strip finalizers if stuck
  local end=$((SECONDS + 150))
  while true; do
    kubectl get ns -o name | sort >/tmp/kl-now
    extra=$(comm -13 "$P/namespaces.txt" /tmp/kl-now)
    [ -z "$extra" ] && break
    if [ $SECONDS -ge $((end - 60)) ]; then
      for n in $extra; do
        n=${n#namespace/}
        for r in $(kubectl api-resources --verbs=list --namespaced -o name 2>/dev/null); do
          kubectl -n "$n" get "$r" -o name 2>/dev/null | xargs -r -I{} kubectl -n "$n" patch {} --type=merge -p '{"metadata":{"finalizers":null}}' >/dev/null 2>&1
        done
        kubectl get ns "$n" -o json | jq '.spec.finalizers=[]' | kubectl replace --raw "/api/v1/namespaces/$n/finalize" -f - >/dev/null 2>&1
      done
    fi
    [ $SECONDS -ge $end ] && { echo "warning: namespaces still terminating: $extra"; break; }
    sleep 3
  done
  # PVs left behind (Retain policy etc.)
  kubectl get pv -o name | sort >/tmp/kl-now
  comm -13 "$P/persistentvolumes.txt" /tmp/kl-now | xargs -r kubectl patch --type=merge -p '{"metadata":{"finalizers":null}}' >/dev/null 2>&1
  # wait for system pods to settle
  kubectl -n kube-system wait --for=condition=Ready pod --all --timeout=180s >/dev/null 2>&1 || echo "warning: some kube-system pods not ready"
  kubectl wait --for=condition=Ready node --all --timeout=180s >/dev/null 2>&1 || echo "warning: some nodes not ready"
  echo "cluster reset"
}

case ${1:-} in
snapshot) snapshot ;;
reset) reset ;;
*) echo "usage: $0 snapshot|reset" >&2; exit 2 ;;
esac
