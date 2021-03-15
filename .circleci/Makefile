HELM_VERSION ?= v3.5.3
HELM2_VERSION ?= v2.17.0
KUSTOMIZE_VERSION ?= v3.8.8
K8S_VERSION ?= v1.13.12
MINIKUBE_VERSION ?= v0.30.0

# ---
CHANGE_MINIKUBE_NONE_USER ?= true
MINIKUBE_WANTUPDATENOTIFICATION ?= false
MINIKUBE_WANTREPORTERRORPROMPT ?= false

tmp := $(shell mktemp -d)
HELM_FILENAME := helm-${HELM_VERSION}-linux-amd64.tar.gz
HELM2_FILENAME := helm-${HELM2_VERSION}-linux-amd64.tar.gz
KUSTOMIZE_FILENAME := kustomize_${KUSTOMIZE_VERSION}_linux_amd64.tar.gz


all: helm kustomize minikube/destroy minikube

helm:
	curl -sSLo $(tmp)/${HELM_FILENAME} "https://get.helm.sh/${HELM_FILENAME}"
	tar zxf $(tmp)/${HELM_FILENAME} --directory ${tmp} linux-amd64/helm
	chmod +x ${tmp}/linux-amd64/helm
	sudo mv ${tmp}/linux-amd64/helm /usr/local/bin/
.PHONY: helm

helm2:
	curl -sSLo $(tmp)/${HELM2_FILENAME} "https://kubernetes-helm.storage.googleapis.com/${HELM2_FILENAME}"
	tar zxf $(tmp)/${HELM2_FILENAME} --directory ${tmp} linux-amd64/helm
	chmod +x ${tmp}/linux-amd64/helm
	sudo mv ${tmp}/linux-amd64/helm /usr/local/bin/
.PHONY: helm2

kustomize:
	curl -sSLo $(tmp)/${KUSTOMIZE_FILENAME} "https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2F${KUSTOMIZE_VERSION}/${KUSTOMIZE_FILENAME}"
	tar zxf $(tmp)/${KUSTOMIZE_FILENAME} --directory ${tmp} kustomize
	chmod +x ${tmp}/kustomize
	sudo mv ${tmp}/kustomize /usr/local/bin/
.PHONY: kustomize

minikube/destroy:
	sudo -E minikube delete || true
	sudo -E rm -rf /etc/kubernetes || true
	sudo -E rm -rf $$HOME/.minikube/* || true
.PHONY: minikube/destroy
.EXPORT_ALL_VARIABLES: minikube/destroy

minikube:
	curl -sSLo ${tmp}/kubectl https://storage.googleapis.com/kubernetes-release/release/${K8S_VERSION}/bin/linux/amd64/kubectl
	chmod +x ${tmp}/kubectl && sudo mv ${tmp}/kubectl /usr/local/bin/
	curl -sSLo ${tmp}/minikube https://storage.googleapis.com/minikube/releases/${MINIKUBE_VERSION}/minikube-linux-amd64
	chmod +x ${tmp}/minikube && sudo mv ${tmp}/minikube /usr/local/bin/
	sudo -E minikube delete || true
	sudo -E rm -rf /etc/kubernetes || true
	sudo -E rm -rf $$HOME/.minikube/* || true
	sudo -E minikube start --vm-driver=none --kubernetes-version=${K8S_VERSION}
	sudo -E minikube update-context
	kubectl wait node/minikube --for=condition=Ready
.PHONY: minikube
.EXPORT_ALL_VARIABLES: minikube
