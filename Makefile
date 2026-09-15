##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php


##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Development

MATERIALIZER_IMG ?= quay.io/zncdatadev/operator-go-materializer:0.0.0-dev
MATERIALIZER_ARCH ?= $(shell go env GOARCH)
MATERIALIZER_CONTEXT = $(LOCALBIN)/materializer-$(MATERIALIZER_ARCH)

.PHONY: materializer-build
materializer-build: ## Compile the formal materializer as a static Linux binary.
	mkdir -p "$(MATERIALIZER_CONTEXT)"
	CGO_ENABLED=0 GOOS=linux GOARCH=$(MATERIALIZER_ARCH) go build -mod=readonly -trimpath -buildvcs=false -o "$(MATERIALIZER_CONTEXT)/materialize" ./cmd/materialize

.PHONY: materializer-image
materializer-image: materializer-build ## Build the materializer image; no registry push.
	docker build --network=none --platform linux/$(MATERIALIZER_ARCH) --build-arg SOURCE_REVISION="$$(git rev-parse HEAD)" -f cmd/materialize/Dockerfile -t "$(MATERIALIZER_IMG)" "$(MATERIALIZER_CONTEXT)"

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/..."

.PHONY: manifests
manifests: controller-gen ## Generate test cluster and independent framework data CRDs.
# Products generate their own cluster CRDs. The framework also owns the independent
# data identity/operation protocol and the mock cluster CRDs used by envtest.
	$(CONTROLLER_GEN) crd paths="./pkg/testutil/..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) crd paths="./pkg/framework/dataops/..." output:crd:artifacts:config=config/framework-data/bases

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: generate manifests fmt vet setup-envtest ## Run tests. Pass extra flags with GOTESTFLAGS, e.g. GOTESTFLAGS=-race.
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $(GOTESTFLAGS) $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: verify-generate
verify-generate: generate manifests ## Fail if the committed generated files are out of date.
# `make test` runs `generate` and `manifests` as prerequisites, so it REPAIRS stale generated files
# before testing rather than reporting them. On its own it can therefore never fail on drift, and
# CI ends up testing a tree that differs from the one that was committed. This target regenerates
# and then insists the generated paths are unchanged.
#
# git status, not git diff: a new package needs a NEW zz_generated.deepcopy.go, which is untracked
# and therefore invisible to git diff.
#
# Scoped to the paths generation writes, so the target stays usable with unrelated work in progress
# — a check that fails on any dirty file is a check nobody runs locally.
#
# The formal Trino example is a separate module. Its own generator checks the
# committed input, CRD and registration companion against the product definition.
# It does not use controller-gen or generate its deployment RBAC. Keep the scoped
# status guard for root generated files and committed example delivery inputs.
	$(MAKE) -C examples/trino-operator verify-generate
	@drift="$$(git status --porcelain -- '*zz_generated*.go' '*/config/crd/bases/*' config/crd/bases config/framework-data/bases '*/config/rbac/*')"; \
	if [ -n "$$drift" ]; then \
		echo "Generated files are out of date. Run 'make generate manifests' and commit the result:"; \
		echo "$$drift"; \
		exit 1; \
	fi
	@echo "Generated files are up to date."

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.19.0
GOLANGCI_LINT_VERSION ?= v2.12.2

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

DATAOPS_IMAGE ?= operator-go-dataops:dev
DATAOPS_ARCH ?= $(shell go env GOARCH)
.PHONY: dataops-build dataops-image
dataops-build: ## Build the independently deployed explicit-data executor.
	mkdir -p bin/dataops-image
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DATAOPS_ARCH) go build -mod=readonly -trimpath -buildvcs=false -o bin/dataops-image/dataops ./cmd/dataops

dataops-image: dataops-build ## Package the data executor separately from product operators.
	docker build --platform linux/$(DATAOPS_ARCH) -f Dockerfile.dataops -t $(DATAOPS_IMAGE) bin/dataops-image
