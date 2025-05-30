## sysctl params required by setup, params persist across reboots
```bash
echo 'net.ipv4.ip_forward = 1' | sudo tee /etc/sysctl.d/k8s.conf
sysctl --system
sysctl -w net.ipv4.ip_forward=1
modprobe br_netfilter
swapoff -a
```

## Install the dependencies for adding repositories
```bash
apt-get update
apt-get install -y software-properties-common curl apt-transport-https ca-certificates gpg
```

## Add the Kubernetes repository
```bash
curl -fsSL https://pkgs.k8s.io/core:/stable:/v1.32/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v1.32/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list
```

## Add the CRI-O repository
```bash
curl -fsSL https://download.opensuse.org/repositories/isv:/cri-o:/stable:/v1.32/deb/Release.key | gpg --dearmor -o /etc/apt/keyrings/cri-o-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/cri-o-apt-keyring.gpg] https://download.opensuse.org/repositories/isv:/cri-o:/stable:/v1.32/deb/ /" | tee /etc/apt/sources.list.d/cri-o.list
```

## Install the packages
```bash
apt-get update
apt-get install -y cri-o kubelet kubeadm kubectl
apt-mark hold cri-o kubelet kubeadm kubectl
```

## Disable CRI-O
```bash
systemctl stop crio
systemctl disable crio
systemctl mask crio
```

## Configure Container Network Interface (CNI)
```bash
mv /etc/cni/net.d/10-crio-bridge.conflist.disabled /etc/cni/net.d/10-crio-bridge.conflist
```

## Start CRI-T and Bootstrap a Cluster
```bash
./crit
kubeadm config images list
kubeadm config images pull --cri-socket unix:///var/run/crio/crio.sock
export KUBECONFIG=/etc/kubernetes/admin.conf
```

## For Control Plane
```bash
# Control Plane
kubeadm init --upload-certs --cri-socket unix:///var/run/crio/crio.sock --v=5
kubeadm token create --print-join-command
# Node
kubeadm join *** --cri-socket unix:///var/run/crio/crio.sock
# Control Plane
kubectl get nodes
kubectl get pods -A
```

## Run yolo11 Example
```bash
kubectl apply -f k8s-test.yaml
kubectl exec -it yolo11-test -- sh
kubectl delete pod yolo11-test
```
for debug
```bash
kubectl describe pod yolo11-test
kubectl logs yolo11-test --previous
kubectl -n kube-system describe pods <name>
kubectl -n kube-system logs <name>
```

## Reset a cluster
```bash
kubeadm reset --force
sudo systemctl stop kubelet
sudo systemctl disable kubelet
taskc d -A
sudo netstat -tulnp | grep 6443
sudo kill -9 <PID>
```