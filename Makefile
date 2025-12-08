# Detect platform for sed compatibility
SED := $(shell if [ "$(shell uname)" = "Darwin" ]; then echo gsed; else echo sed; fi)


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set).
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

## Detect platform for Kind binary
UNAME := $(shell uname -s | tr '[:upper:]' '[:lower:]')
KIND_BINARY := kind-$(UNAME)-amd64

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KIND = $(LOCALBIN)/kind
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions

ENVTEST_K8S_VERSION ?= 1.30.0
# renovate: datasource=github-releases depName=kubernetes-sigs/controller-runtime
ENVTEST_VERSION ?= release-0.18
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.7.2
# renovate: datasource=github-releases depName=google/yamlfmt
YAMLFMT_VERSION ?= v0.20.0
# renovate: datasource=github-releases depName=kubernetes-sigs/kind
KIND_VERSION ?= 0.30.0
# renovate: datasource=github-releases depName=onsi/ginkgo
GINKGO_VERSION ?= v2.27.3
# renovate: datasource=github-releases depName=kubernetes/autoscaler
VPA_VERSION ?= vertical-pod-autoscaler-1.5.0

.PHONY: all
all: build

##@ General
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Tagging

# Find the latest tag (with prefix filter if defined, default to 0.0.0 if none found)
# Lazy evaluation ensures fresh values on every run
VERSION_PREFIX ?= v
LATEST_TAG = $(shell git tag --list "$(VERSION_PREFIX)*" --sort=-v:refname | head -n 1)
VERSION = $(shell [ -n "$(LATEST_TAG)" ] && echo $(LATEST_TAG) | sed "s/^$(VERSION_PREFIX)//" || echo "0.0.0")

patch: ## Create a new patch release (x.y.Z+1)
	@NEW_VERSION=$$(echo "$(VERSION)" | awk -F. '{printf "%d.%d.%d", $$1, $$2, $$3+1}') && \
	git tag "$(VERSION_PREFIX)$${NEW_VERSION}" && \
	echo "Tagged $(VERSION_PREFIX)$${NEW_VERSION}"

minor: ## Create a new minor release (x.Y+1.0)
	@NEW_VERSION=$$(echo "$(VERSION)" | awk -F. '{printf "%d.%d.0", $$1, $$2+1}') && \
	git tag "$(VERSION_PREFIX)$${NEW_VERSION}" && \
	echo "Tagged $(VERSION_PREFIX)$${NEW_VERSION}"

major: ## Create a new major release (X+1.0.0)
	@NEW_VERSION=$$(echo "$(VERSION)" | awk -F. '{printf "%d.0.0", $$1+1}') && \
	git tag "$(VERSION_PREFIX)$${NEW_VERSION}" && \
	echo "Tagged $(VERSION_PREFIX)$${NEW_VERSION}"

tag: ## Show latest tag
	@echo "Latest version: $(LATEST_TAG)"

push: ## Push tags to remote
	git push --tags


##@ Development

.PHONY: download
download: ## Download go packages and list them.
	go mod download
	go list -m all

.PHONY: verify-deps
verify-deps: ## Verify go.mod and go.sum are tidy
	go mod tidy
	git diff --exit-code go.mod go.sum

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" \
	go test -coverprofile=cover.out -covermode=atomic -count=1 -parallel=4 -timeout=5m ./internal/...

.PHONY: kind
kind: $(KIND) ## Create a Kind cluster.
	@echo "Setting up Kind cluster..."
	@$(KIND) create cluster --name autovpa-test --wait 60s
	@kubectl cluster-info
	@$(MAKE) install-vpa

.PHONY: delete-kind
delete-kind: ## Delete the Kind cluster.
	@echo "Deleting Kind cluster..."
	@$(KIND) delete cluster --name autovpa-test
	@echo "Kind cluster teardown complete."

.PHONY: e2e
e2e: ginkgo ## Run all e2e tests sequentially (Ginkgo procs=1 required due to shared state: LogBuffer, Operator process, Cluster resources)
	@echo "Running e2e tests with Ginkgo..."
	PATH=$(LOCALBIN):$$PATH USE_EXISTING_CLUSTER="true" \
  ginkgo --procs=1 --timeout=30m --tags=e2e -v --focus='${FOCUS}' ./test/e2e/...

.PHONY: e2e-generic
e2e-generic: ## Run only Mixed workload e2e tests
	@$(MAKE) e2e FOCUS=Generic

.PHONY: e2e-argo-disabled
e2e-argo-disabled: ## Run only Argo tracking disabled e2e tests
	@$(MAKE) e2e FOCUS="Argo tracking disabled"

.PHONY: e2e-argo-enabled
e2e-argo-enabled: ## Run only Argo tracking enabled e2e tests
	@$(MAKE) e2e FOCUS="Argo tracking enabled"

.PHONY: e2e-namespaced
e2e-namespaced: ## Run only Namespaced mode e2e tests
	@$(MAKE) e2e FOCUS="Namespaced mode"

.PHONY: e2e-vpa
e2e-vpa: ## Run only VPA e2e tests
	@$(MAKE) e2e FOCUS="VPA Generic"

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

.PHONY: check-header
check-header: ## Verify that all *.go files have the boilerplate header
	@missing_files=0; \
	for file in $(shell find . -type f -name '*.go'); do \
		if ! diff <(head -n $(shell wc -l < hack/boilerplate.go.txt) $$file) hack/boilerplate.go.txt > /dev/null; then \
			echo "Missing or incorrect header in $$file"; \
			missing_files=$$((missing_files + 1)); \
		fi; \
	done; \
	if [ $$missing_files -ne 0 ]; then \
		echo "ERROR: Some files are missing the required boilerplate header."; \
		exit 1; \
	fi; \
	echo "All files have the correct boilerplate header."

.PHONY: check-header-fix
check-header-fix: ## Fix missing or incorrect headers in all *.go files
	@for file in $(shell find . -type f -name '*.go'); do \
		if ! diff <(head -n $(shell wc -l < hack/boilerplate.go.txt) $$file) hack/boilerplate.go.txt > /dev/null; then \
			echo "Fixing header in $$file"; \
			content=$$(cat $$file); \
			cat hack/boilerplate.go.txt > $$file; \
			echo "" >> $$file; \
			echo "$$content" >> $$file; \
		fi; \
	done; \
	echo "Headers have been fixed for all *.go files."

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/autovpa cmd/main.go

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./cmd/main.go $(ARGS)

.PHONY: kustomize
kustomize: ## Render kustomize manifests and save as a single file.
	kustomize build deploy/kubernetes/ > deploy/kubernetes/autovpa.yaml
	yamlfmt deploy/kubernetes/kustomized.yaml

##@ Dependencies

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: yamlfmt
yamlfmt: $(LOCALBIN)/yamlfmt ## Download yamlfmt locally if necessary.
$(LOCALBIN)/yamlfmt: $(LOCALBIN)
	$(call go-install-tool,$(LOCALBIN)/yamlfmt,github.com/google/yamlfmt/cmd/yamlfmt,$(YAMLFMT_VERSION))

.PHONY: kind
$(KIND): $(LOCALBIN)
	@if [ ! -f $(KIND) ]; then \
		echo "Downloading Kind v$(KIND_VERSION) for $(UNAME)..."; \
		curl -L -o $(KIND) https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/$(KIND_BINARY); \
		chmod +x $(KIND); \
		echo "Kind v$(KIND_VERSION) installed at $(KIND)."; \
	fi

.PHONY: install-vpa
install-vpa: ## Install the Vertical Pod Autoscaler CRDs and components into the current cluster.
	@echo "Installing Vertical Pod Autoscaler components..."
	@TMP_DIR=$$(mktemp -d); \
		echo "Downloading VPA release archive $(VPA_VERSION)..."; \
		curl -fsSL https://github.com/kubernetes/autoscaler/archive/refs/tags/$(VPA_VERSION).zip -o $$TMP_DIR/vpa.zip; \
		unzip -q $$TMP_DIR/vpa.zip -d $$TMP_DIR; \
		VPA_DIR=$$TMP_DIR/autoscaler-$(VPA_VERSION)/vertical-pod-autoscaler/deploy; \
			echo "Applying CRDs with validation disabled"; \
			kubectl apply --validate=false -f $$VPA_DIR/vpa-v1-crd-gen.yaml; \
			echo "Applying remaining resources"; \
		for resource in \
				admission-controller-deployment.yaml \
				admission-controller-service.yaml \
				recommender-deployment.yaml \
				recommender-deployment-low.yaml \
				recommender-deployment-high.yaml \
				updater-deployment.yaml \
				vpa-rbac.yaml; do \
			kubectl apply -f "$$VPA_DIR/$$resource"; \
		done; \
		echo $$VPA_DIR; \
		#rm -rf $$TMP_DIR
	@echo "VPA installation complete."

.PHONY: ginkgo
ginkgo: $(LOCALBIN)/ginkgo ## Download ginkgo locally if necessary.
$(LOCALBIN)/ginkgo: $(LOCALBIN)
	$(call go-install-tool,$(LOCALBIN)/ginkgo,github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
tools: ginkgo envtest golangci-lint yamlfmt kind ## Install all tools

define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
