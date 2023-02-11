# Kubernetes

&nbsp;
## Installation

### kubectl

Use the following (Ubuntu) repository to install kubectl 
```
sudo curl -fsSLo /usr/share/keyrings/kubernetes-archive-keyring.gpg https://packages.cloud.google.com/apt/doc/apt-key.gpg
echo "deb [signed-by=/usr/share/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main" | sudo tee /etc/apt/sources.list.d/kubernetes.list
sudo apt update
sudo apt install kubectl
```

### minikube
Use the following (Ubuntu) repository to install minikube 

```
wget https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
sudo cp minikube-linux-amd64 /usr/local/bin/minikube
sudo chmod 755 /usr/local/bin/minikube
```

### helm
Use the following (Ubuntu) repository to install helm

```
curl https://baltocdn.com/helm/signing.asc | gpg --dearmor | sudo tee /usr/share/keyrings/helm.gpg > /dev/null
sudo apt-get install apt-transport-https --yes
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update
sudo apt-get install helm
```

&nbsp;  
## Other requirements

Minikube won't be able to use docker as sudo.  
Therefor, your user should be able to use docker without sudo permissions.  
Run the following command to allow this:

```
sudo usermod -aG docker $USER && newgrp docker
```

Build the docker image inside the minikube environment

```
eval $(minikube docker-env)
cd chisel/src
sudo docker build -t jpillora/chisel .
```

&nbsp;  
## Usage

### The cluster

We'll use Kubernetes (minikube) with the docker driver. This will deploy a minikube docker container.  
A directory, here "/kubernetes" (with all required Kubernetes data), on the host will be mounted to the minikube container, for example:

```
$tree /kubernetes/

/kubernetes/ 						# Permissions rwxrwxrwx (755)
└── chisel
    ├── certs
    │   ├── chisel.crt
    │   ├── chisel.csr
    │   ├── chisel.key
    │   ├── root.crt
    │   ├── root.key
    │   └── root.srl
    └── config
        └── users.json				# Make sure user configuration is complete.
```

This allows to further mount this directory as persistent volume across pods.  
See it as volumes in docker but shared across containers.

```
minikube start --driver=docker --mount-string=/kubernetes:/kubernetes --mount

# Cluster status
minikube status
 
# Stop the minikube container
minikube stop
 
# Delete the minikube container (including all kubernetes objects)
minikube delete
```

&nbsp; 
### The environment


The Kubernetes deployment consist of the following objects:

![Chisel environment](https://www.weave.works/assets/images/blt0ac8a1e3751df7e9/k8s-hpa.png)

* Ingress (aka Nginx) exposing a part of the setup to the outside world
* Chisel environment
    * Kubernetes Service Loadbalancer in IPVS mode (least connections)
    * Deployment
        * ReplicaSet
        * HorizontalPodAutoScaler
        * Pods
        * PersistentVolume
* Prometheus
    * Prometheus Server 
    * Prometheus Adapter
* Kubernetes Dashboard

&nbsp;  
To deploy everything, use the following commands:

```
# Deploy Chisel environment
kubectl apply -f kubernetes/chisel/
 
# Deploy Ingress Controller
minikube addons enable ingress
helm repo add nginx-stable https://helm.nginx.com/stable
helm repo update
helm install ingress nginx-stable/nginx-ingress -n ingress-nginx --create-namespace -f kubernetes/ingress/helm/values.yml
kubectl apply -f kubernetes/ingress
 
# Deploy Prometheus
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/prometheus –namespace prometheus --create-namespace
helm install prometheus-adapter prometheus-community/prometheus-adapter -n prometheus --create-namespace -f kubernetes/prometheus/adapter/helm/values.yml
 
# Deploy Kubernetes Dashboard
kubectl apply -f kubernetes/dashboard/
 
# Connect
./chisel client --auth "user:password" --tls-skip-verify --fingerprint <FINGERPRINT> -v <SERVER> <REMOTES>
```

Enable modules on the host:

```
sudo modprobe -a ip_vs ip_vs_rr ip_vs_lc ip_vs_wrr ip_vs_sh
```

Or, create the file /etc/modules-load.d/ipvs-kube-proxy with the following content (and reboot, for the modules to be loaded):

```
ip_vs
ip_vs_rr
ip_vs_lc
ip_vs_wrr
ip_vs_sh
```

Now, enable IPVS mode on the kube-proxy. Put the scheduler on "lc" (least connections).  
Of course, the existing kube-proxy keeps working with the old configuration. Delete the pod and Kubernetes will automatically re-create it, with the new configuration.  
Verify that the kube-proxy activated IPVS

```
kubectl edit configmap kube-proxy -n kube-system
# ...
# mode: "ipvs"
# ipvs:
#  scheduler: "lc"
# ...
 
kubectl get pods -n kube-system
kubectl delete pod -n kube-system kube-proxy-${ID}
kubectl get pods -n kube-system
kubectl logs kube-proxy-${ID}  -n kube-system
# ...
# "Using ipvs Proxier"
# ...
```

&nbsp; 
### Monitoring

```
# List objects in the default namespace
kubectl get nodes
kubectl get deployments
kubectl get pods
kubectl get pv
kubectl get pvc
kubectl get service
kubectl get ingress
kubectl get rs
kubectl get hpa
kubectl get cm
 
# To list everything use the -A option
# To list a specific namespace use -n ...
# 'get' can be replaced by 'describe' to get more detailed information
 
# Expose Kubernetes Dashboard
kubectl proxy
 
# Generate token
kubectl describe secret admin-user -n kubernetes-dashboard | grep 'token:'
 
# Kubernetes Dashboard URL
# http://localhost:8001/api/v1/namespaces/kubernetes-dashboard/services/https:kubernetes-dashboard:/proxy/#/ingress?namespace=default
 
# Chisel Prometheus metrics
docker ps -a
kubectl get pods -o wide
docker exec -it <MINIKUBE_CONTAINER_ID> curl http://<CHISEL_POD_IP>:9113/metrics
 
# Or easier
kubectl get pods -o wide
minikube ssh
curl http://<CHISEL_POD_IP>:9113/metrics
 
# sessions mentioned match with ipvs connections in minikube proxy
# ipvsadm will have to be installed first
#
# There should be a section with the IP address of the Kubernetes Services, with all pod endpoints listed underneath.
# Number of connections on each pod, should match the 'chisel_number_of_active_sessions' metric. (unless somebody is requesting the metrics (:9113/metrics) from the service ;-))
minikube ssh
sudo ipvsadm
```
