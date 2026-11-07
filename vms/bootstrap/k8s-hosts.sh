#!/bin/bash
# Write controlplane/node01 into /etc/hosts. Env: CP_IP WORKER_IP
set -euxo pipefail
sed -i '/# kubelab-nodes/d' /etc/hosts
echo "$CP_IP controlplane # kubelab-nodes" >>/etc/hosts
echo "$WORKER_IP node01 # kubelab-nodes" >>/etc/hosts
# kubelet should advertise the user-v2 address
IP=$(hostname -I | awk '{print $1}')
echo "KUBELET_EXTRA_ARGS=--node-ip=$IP" >/etc/default/kubelet
