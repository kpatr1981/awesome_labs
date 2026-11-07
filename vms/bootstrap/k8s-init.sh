#!/bin/bash
# Control-plane bootstrap (as root). Env: K8S_VERSION LAB_USER
set -euxo pipefail
export KUBECONFIG=/etc/kubernetes/admin.conf
H=$(getent passwd "$LAB_USER" | cut -d: -f6)
ARCH=$(dpkg --print-architecture)
if [ ! -f /etc/kubernetes/admin.conf ]; then
  IP=$(hostname -I | awk '{print $1}')
  cat >/etc/kubernetes/kubeadm-config.yaml <<CFG
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
localAPIEndpoint:
  advertiseAddress: "$IP"
nodeRegistration:
  name: controlplane
  criSocket: unix:///run/containerd/containerd.sock
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
kubernetesVersion: "v$K8S_VERSION"
clusterName: kubelab
controlPlaneEndpoint: "controlplane:6443"
apiServer:
  certSANs: ["127.0.0.1", "controlplane", "$IP"]
networking:
  podSubnet: "10.244.0.0/16"
  serviceSubnet: "10.96.0.0/12"
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
CFG
  kubeadm init --config /etc/kubernetes/kubeadm-config.yaml --upload-certs
fi
mkdir -p "$H/.kube" /root/.kube
cp -f /etc/kubernetes/admin.conf "$H/.kube/config"
cp -f /etc/kubernetes/admin.conf /root/.kube/config
chown -R "$LAB_USER": "$H/.kube"

# --- CNI: Calico ---
if ! kubectl -n kube-system get ds calico-node >/dev/null 2>&1; then
  curl -fsSL "https://raw.githubusercontent.com/projectcalico/calico/${CALICO_VERSION}/manifests/calico.yaml" -o /tmp/calico.yaml
  sed -i -e 's|# - name: CALICO_IPV4POOL_CIDR|- name: CALICO_IPV4POOL_CIDR|' \
         -e 's|#   value: "192.168.0.0/16"|  value: "10.244.0.0/16"|' /tmp/calico.yaml
  kubectl apply -f /tmp/calico.yaml
fi

# --- Helm ---
if ! command -v helm >/dev/null; then
  curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${ARCH}.tar.gz" | tar -xz -C /tmp
  install -m 0755 "/tmp/linux-${ARCH}/helm" /usr/local/bin/helm
fi

# --- metrics-server ---
kubectl apply -f "https://github.com/kubernetes-sigs/metrics-server/releases/download/${METRICS_VERSION}/components.yaml"
kubectl -n kube-system patch deploy metrics-server --type=json \
  -p '[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]' 2>/dev/null || true
kubectl -n kube-system get deploy metrics-server -o jsonpath='{.spec.template.spec.containers[0].args}' | grep -q insecure || true

# --- local-path storage (default StorageClass) ---
kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/${LOCALPATH_VERSION}/deploy/local-path-storage.yaml"
kubectl patch storageclass local-path -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'

# --- Gateway API CRDs ---
kubectl apply --server-side -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml"
