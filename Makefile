# Current Operator version
VERSION ?= 0.0.1
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager

# Run tests
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: generate fmt vet manifests
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

install_yaml: manifests kustomize
	$(KUSTOMIZE) build config/crd > install.yaml

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

deploy_yaml: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > ./config/samples/deployment.yaml

# UnDeploy controller from the configured Kubernetes cluster in ~/.kube/config
undeploy:
	$(KUSTOMIZE) build config/default | kubectl delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build:
	docker build -t ${IMG} -f ${DOCKERFILE} . 

# Push the docker image
docker-push:
	docker push ${IMG}

# Download controller-gen locally if necessary
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen:
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# Download kustomize locally if necessary
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize:
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

# Generate bundle manifests and metadata, then validate generated files.
.PHONY: bundle
bundle: manifests kustomize
	operator-sdk generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | operator-sdk generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	operator-sdk bundle validate ./bundle

# Build the bundle image.
.PHONY: bundle-build
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

build-and-deploy:
	make docker-build docker-push IMG=us.gcr.io/rookout/rookout-k8s-operator:1.0 DOCKERFILE=Dockerfile
	kubectl delete deployment.apps/rookout-controller-manager -n rookout #Comment this out if this is the first time running on the cluster
	make install
	make deploy IMG=us.gcr.io/rookout/rookout-k8s-operator:1.0
	kubectl apply -f config/samples/rookout_v1alpha1_rookout.yaml

deployment_yamls:
	make deploy_yaml IMG=us.gcr.io/rookout/rookout-k8s-operator:1.0

log:
	kubectl logs deployment.apps/rookout-controller-manager -n rookout -c manager -f

build_init_container:
	docker build -f InitContainer.Dockerfile . -t us.gcr.io/rookout/rookout-k8s-operator-init-container:${INNER_VERSION}
	docker build -f InitContainer.Dockerfile.ubi . -t us.gcr.io/rookout/rookout-k8s-operator-init-container-ubi:${INNER_VERSION}

push_init_container:
	docker push us.gcr.io/rookout/rookout-k8s-operator-init-container:${INNER_VERSION}
	docker push us.gcr.io/rookout/rookout-k8s-operator-init-container-ubi:${INNER_VERSION}

apply_config:
	kubectl apply -f ./config/samples/rookout_v1alpha1_rookout.yaml

update_rook_jar:
	#git checkout master
	git pull
	# Needed because jenkins.
	git config --global user.email "sonario@rookout.com"
	git config --global user.name "sonariorobot"
	curl -o rook.jar https://get.rookout.com/rook.jar
	git add rook.jar
	git commit -m "Updated rook.jar version to `java -jar ./rook.jar | grep "Rookout version" | awk -F': ' {'print $$2'}` [skip ci]"
	git push


publish-operator:
	## Publishing operator & init container images

	# Pulling image from rookout's bucket
	gcloud docker -- pull us.gcr.io/rookout/rookout-k8s-operator:${INNER_VERSION}
	gcloud docker -- pull us.gcr.io/rookout/rookout-k8s-operator-ubi:${INNER_VERSION}
	gcloud docker -- pull us.gcr.io/rookout/rookout-k8s-operator-init-container:${INNER_VERSION}
	gcloud docker -- pull us.gcr.io/rookout/rookout-k8s-operator-init-container-ubi:${INNER_VERSION}

	# Tagging image with dockerhub name and right version
	docker tag us.gcr.io/rookout/rookout-k8s-operator:${INNER_VERSION} rookout/k8s-operator:${VERSION_TO_PUBLISH}
	docker tag us.gcr.io/rookout/rookout-k8s-operator-ubi:${INNER_VERSION} rookout/k8s-operator-ubi:${VERSION_TO_PUBLISH}
	docker tag us.gcr.io/rookout/rookout-k8s-operator:${INNER_VERSION} rookout/k8s-operator:latest
	docker tag us.gcr.io/rookout/rookout-k8s-operator-ubi:${INNER_VERSION} rookout/k8s-operator-ubi:latest
	docker tag us.gcr.io/rookout/rookout-k8s-operator-init-container:${INNER_VERSION} rookout/k8s-operator-init-container:${VERSION_TO_PUBLISH}
	docker tag us.gcr.io/rookout/rookout-k8s-operator-init-container-ubi:${INNER_VERSION} rookout/k8s-operator-init-container-ubi:${VERSION_TO_PUBLISH}
	docker tag us.gcr.io/rookout/rookout-k8s-operator-init-container:${INNER_VERSION} rookout/k8s-operator-init-container:latest
	docker tag us.gcr.io/rookout/rookout-k8s-operator-init-container-ubi:${INNER_VERSION} rookout/k8s-operator-init-container-ubi:latest

	# Logging into dockerhub and pushing
	docker login -u ${DOCKERHUB_USERNAME} -p ${DOCKERHUB_PASSWORD}
	docker push rookout/k8s-operator:${VERSION_TO_PUBLISH}
	docker push rookout/k8s-operator-ubi:${VERSION_TO_PUBLISH}
	docker push rookout/k8s-operator:latest
	docker push rookout/k8s-operator-ubi:latest
	docker push rookout/k8s-operator-init-container:${VERSION_TO_PUBLISH}
	docker push rookout/k8s-operator-init-container-ubi:${VERSION_TO_PUBLISH}
	docker push rookout/k8s-operator-init-container:latest
	docker push rookout/k8s-operator-init-container-ubi:latest
