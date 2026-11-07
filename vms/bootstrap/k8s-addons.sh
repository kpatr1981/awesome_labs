#!/bin/bash
# Add-ons that need a worker node (as root): Traefik ingress/gateway controller.
set -euxo pipefail
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl wait --for=condition=Ready node --all --timeout=600s
helm repo add traefik https://traefik.github.io/charts >/dev/null 2>&1 || true
helm repo update traefik
if ! helm -n traefik status traefik >/dev/null 2>&1; then
  helm install traefik traefik/traefik --version 42.3.1 -n traefik --create-namespace --wait --timeout 10m \
    --set service.spec.type=NodePort \
    --set ports.web.nodePort=30080 \
    --set ports.websecure.nodePort=30443 \
    --set providers.kubernetesGateway.enabled=true \
    --set providers.kubernetesIngress.enabled=true \
    --set ingressClass.isDefaultClass=false \
    --set gateway.enabled=false
fi
kubectl -n kube-system rollout status deploy/coredns --timeout=300s
kubectl -n kube-system rollout status deploy/metrics-server --timeout=300s || true
kubectl -n local-path-storage rollout status deploy/local-path-provisioner --timeout=300s
