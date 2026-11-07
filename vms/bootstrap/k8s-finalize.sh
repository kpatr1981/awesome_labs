#!/bin/bash
# Final step on the control plane (as root): snapshot cluster state.
set -euxo pipefail
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl wait --for=condition=Ready node --all --timeout=300s
kubectl -n kube-system wait --for=condition=Ready pod --all --timeout=300s
/opt/kubelab/k8s-api.sh snapshot
