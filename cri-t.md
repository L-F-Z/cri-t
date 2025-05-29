## sysctl params required by setup, params persist across reboots
```bash
echo 'net.ipv4.ip_forward = 1' | sudo tee /etc/sysctl.d/k8s.conf
sysctl --system
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

## Configure a Container Network Interface (CNI) plugin
```bash
mv /etc/cni/net.d/10-crio-bridge.conflist.disabled /etc/cni/net.d/10-crio-bridge.conflist
```

## Start CRI-T
```bash
wget https://go.dev/dl/go1.24.3.linux-amd64.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf go1.24.3.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a /etc/profile
apt install -y git
git clone https://github.com/L-F-Z/cri-t
cd cri-t
go run cmd/crio/main.go
```

## Bootstrap a cluster
```bash
swapoff -a
modprobe br_netfilter
sysctl -w net.ipv4.ip_forward=1
kubeadm config images list
kubeadm config images pull --cri-socket unix:///var/run/crio/crio.sock
kubeadm init --upload-certs --cri-socket unix:///var/run/crio/crio.sock --v=5
export KUBECONFIG=/etc/kubernetes/admin.conf
```

## Use Kubectl
```bash
kubectl get pods -A
kubectl -n kube-system describe pods <name>
kubectl -n kube-system logs <name>
```
https://github.com/cri-o/cri-o/blob/main/tutorials/crictl.md

## Reset a cluster
```bash
kubeadm reset
sudo systemctl stop kubelet
taskc d -A
```