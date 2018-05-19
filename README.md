# Kubernetes Initializer

# Diagram flow :

![alt text](https://raw.githubusercontent.com/suker200/kube_initializer/master/k8s-dns-local-caching.png)

# Requirement:
 - k8s 1.8+

# Reason:
 - We want inject on fly some data to kubernetes object (deployment, service, ingress etc...) without touching to origin manifest
 - We allow developer deploy manifest to k8s cluster but don't allow they create loadbalancer, storage etc.. 

# Target:
- DNS: We apply dns-local-caching on every node, pod's resolv.conf must point to node's dns port before go to kube-dns, so when kube-dns failed/network issue or sometime Global dns failed, dns-local-caching support dns failover to dns tale records (1day).
- Developer: We don't allow developer create loadbalancer from k8s or ingress class, service type etc..

# How to:
DNS:<br>
 - deploy dns-daemonset: support dns failover
 - Every deployment will be injected:
     + initcontainer: get node's ip to be nameserver and put on top of resolv.conf --> {emptyDir}/resolv.conf
     + volumes + volumeMount: mount {emptyDir}/resolv.conf /etc/resolv.conf <br>

Developer:<br>
 - ingress
 - service
 - nodeAffinity

# Config
```
---
developer:
  enable: false
  namespacePattern: ".*-dev$"
  nodeSelectorTerms:
    - matchExpressions:
      - key: spot.instance.reserve
        operator: Exists
  ingress:
  	class: nginx-internal
  service:
  	type:
  		- ClusterIP
  		- None
local_dns:
  enable: true
  namespacePattern: ".*-dev$"
```

- Developer: we control developer resource
  + enable: enable/disable apply
  + namespacePattern: we control developer base on namespace
  + nodeSelectorTerms: force all developer resource to speicific spotInstance
  + ingress: only allow one dev ingress class
  + service: only allow ["ClusterIP", "None"] type
- local_dns: we apply inject DNS nameserver on every deployment (currently support deployment)
  + enable: enable/disable apply
  + namespacePattern: we try to apply base on namespace to limit the big effection at a time

# Usage:
- Build image
- help upgrade -i --namespace=kube-system initializer helm

# Build

- Build kube_initializer

```
glide up --strip-vendor

CGO_ENABLED=0 env GOOS=linux go build

```

- Build kube_initializer docker image

```
docker build -t kube_initializer -f Dockerfile
```

- Update helm chart (We using helm chart for deploying application). You can find it in charts folder

# Test
- Requirement:
	+ virtualbox
	+ minikube
	+ kubectl
	+ helm
	
- we can test with minikube :) 
