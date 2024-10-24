# include the bingo binary variables. This enables the bingo versions to be
# referenced here as make variables. For example: $(GOLANGCI_LINT)
include .bingo/Variables.mk

# set the default target here, because the include above will automatically set
# it to the first defined target
.DEFAULT_GOAL := default
default: all

VERSION ?= v0.0.1

CRD_DIR := $(shell pwd)/deploy/crds

# REGISTRY_BASE
# defines the container registry and organization for the bundle and operator container images.
REGISTRY_BASE_OPENSHIFT = quay.io/rhobs
REGISTRY_BASE ?= $(REGISTRY_BASE_OPENSHIFT)

# Image URL to use all building/pushing image targets
IMG ?= $(REGISTRY_BASE)/multicluster-observability-addon:$(VERSION)

.PHONY: deps
deps: go.mod go.sum
	go mod tidy
	go mod download
	go mod verify

$(CRD_DIR)/observability.openshift.io_clusterlogforwarders.yaml:
	@mkdir -p $(CRD_DIR)
	@curl https://raw.githubusercontent.com/openshift/cluster-logging-operator/release-6.0/bundle/manifests/observability.openshift.io_clusterlogforwarders.yaml > $(CRD_DIR)/observability.openshift.io_clusterlogforwarders.yaml

$(CRD_DIR)/opentelemetry.io_opentelemetrycollectors.yaml:
	@mkdir -p $(CRD_DIR)
	@curl https://raw.githubusercontent.com/open-telemetry/opentelemetry-operator/v0.100.1/bundle/manifests/opentelemetry.io_opentelemetrycollectors.yaml > $(CRD_DIR)/opentelemetry.io_opentelemetrycollectors.yaml

$(CRD_DIR)/opentelemetry.io_instrumentations.yaml:
	@mkdir -p $(CRD_DIR)
	@curl https://raw.githubusercontent.com/open-telemetry/opentelemetry-operator/v0.100.1/bundle/manifests/opentelemetry.io_instrumentations.yaml > $(CRD_DIR)/opentelemetry.io_instrumentations.yaml

.PHONY: install-crds
install-crds: $(CRD_DIR)/observability.openshift.io_clusterlogforwarders.yaml $(CRD_DIR)/opentelemetry.io_opentelemetrycollectors.yaml $(CRD_DIR)/opentelemetry.io_instrumentations.yaml

.PHONY: fmt
fmt: $(GOFUMPT) ## Run gofumpt on source code.
	find . -type f -name '*.go' -not -path '**/fake_*.go' -exec $(GOFUMPT) -w {} \;

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint on source code.
	$(GOLANGCI_LINT) run --timeout=5m ./...

.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Attempt to automatically fix lint issues in source code.
	$(GOLANGCI_LINT) run --fix --timeout=5m ./...

.PHONY: test
test:
	go test -v ./internal/...

.PHONY: integration-test
integration-test:
	go test -timeout 30s ./test/integration/...

.PHONY: integration-env
integration-env:
	@mkdir -p tmp/crds
	@curl -sL -o tmp/crds/addon.open-cluster-management.io_addondeploymentconfigs.crd.yaml https://raw.githubusercontent.com/open-cluster-management-io/api/f6c65820279078afbe536d5a6012e0b3badde3c5/addon/v1alpha1/0000_02_addon.open-cluster-management.io_addondeploymentconfigs.crd.yaml
	@curl -sL -o tmp/crds/addon.open-cluster-management.io_clustermanagementaddons.crd.yaml https://raw.githubusercontent.com/open-cluster-management-io/api/f6c65820279078afbe536d5a6012e0b3badde3c5/addon/v1alpha1/0000_00_addon.open-cluster-management.io_clustermanagementaddons.crd.yaml
	@curl -sL -o tmp/crds/addon.open-cluster-management.io_managedclusteraddons.crd.yaml https://raw.githubusercontent.com/open-cluster-management-io/api/f6c65820279078afbe536d5a6012e0b3badde3c5/addon/v1alpha1/0000_01_addon.open-cluster-management.io_managedclusteraddons.crd.yaml 
	@curl -sL -o tmp/crds/clusters.open-cluster-management.io_managedclusters.crd.yaml https://raw.githubusercontent.com/open-cluster-management-io/api/f6c65820279078afbe536d5a6012e0b3badde3c5/cluster/v1/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml 
	@curl -sL -o tmp/crds/work.open-cluster-management.io_manifestworks.crd.yaml https://raw.githubusercontent.com/open-cluster-management-io/api/f6c65820279078afbe536d5a6012e0b3badde3c5/work/v1/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml 
	@curl -sL -o tmp/crds/opentelemetry.io_instrumentations.yaml https://raw.githubusercontent.com/open-telemetry/opentelemetry-operator/70dff6948ad00a971840b1dce71e8c4d3a52e02c/bundle/community/manifests/opentelemetry.io_instrumentations.yaml 
	@curl -sL -o tmp/crds/opentelemetry.io_opentelemetrycollectors.yaml https://raw.githubusercontent.com/open-telemetry/opentelemetry-operator/70dff6948ad00a971840b1dce71e8c4d3a52e02c/bundle/community/manifests/opentelemetry.io_opentelemetrycollectors.yaml 
	@curl -sL -o tmp/crds/logging.openshift.io_clusterlogforwarders.yaml https://raw.githubusercontent.com/openshift/cluster-logging-operator/bced99a9889abe7714b2c5e558f3056b95701baf/config/crd/bases/observability.openshift.io_clusterlogforwarders.yaml && sed -i 's/observability.openshift.io/logging.openshift.io/g' tmp/crds/logging.openshift.io_clusterlogforwarders.yaml 
	@curl -sL -o tmp/crds/monitoring.coreos.com_prometheusagents.yaml https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/ca4f84f2bb6ce42fd9e3bb81a46e0bb32d042db1/example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusagents.yaml 
	@go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
	@setup-envtest -p env use 1.30.x

.PHONY: addon
addon: deps fmt ## Build addon binary
	go build -o bin/multicluster-observability-addon main.go

.PHONY: oci-build
oci-build: ## Build the image
	podman build -t ${IMG} .

.PHONY: oci-push
oci-push: ## Push the image
	podman push ${IMG}

.PHONY: oci
oci: oci-build oci-push

.PHONY: addon-deploy
addon-deploy: $(KUSTOMIZE) install-crds
	cd deploy && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build ./deploy | kubectl apply -f -

.PHONY: addon-undeploy
addon-undeploy: $(KUSTOMIZE) install-crds
	$(KUSTOMIZE) build ./deploy | kubectl delete -f -
