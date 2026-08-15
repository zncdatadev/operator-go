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

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/..."

.PHONY: manifests
manifests: controller-gen ## Generate the CRDs backing the test mock cluster resources.
# This SDK ships API types, not CRDs — a product operator generates its own from the types it
# embeds. The only CRDs generated here are the ones envtest installs for pkg/testutil's mock
# cluster resources, and they are generated rather than hand-written on purpose: a hand-written
# CRD drifts from the Go types, and the schema-free version this replaced meant the API server
# performed no defaulting, validation or pruning in ANY test in the repository.
	$(CONTROLLER_GEN) crd paths="./pkg/testutil/..." output:crd:artifacts:config=config/crd/bases

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
# examples/trino-operator is a separate module and is checked too: it embeds the commons API types,
# so a change to pkg/apis leaves its CRD stale, and it is the reference implementation downstream
# operators copy. Its own Makefile pins its own controller-gen, so it is driven through that.
#
# '*/config/rbac/*' is in the pathspec because that module's generated ClusterRole is the canonical
# operator-side permission set docs/security.md §3.3 points adopters at. Without it, an edited
# +kubebuilder:rbac marker whose regenerated role.yaml was never committed passed CI.
#
# The trailing /* is load-bearing, and its absence is why the sibling '*/config/crd/bases' entry had
# been inert since it was written. A git pathspec containing a wildcard is wildmatched against the
# FULL path with no directory-prefix expansion, so '*/config/rbac' matches a path that IS that
# directory and never a file inside it — the guard passed unconditionally. Verified by dirtying
# examples/trino-operator/config/rbac/role.yaml: plain `git status` shows it, the old pathspec
# reported nothing, the new one reports it. The literal `config/crd/bases` (no wildcard) always
# worked, which is why only the root module was ever actually covered.
	$(MAKE) -C examples/trino-operator generate manifests
	@drift="$$(git status --porcelain -- '*zz_generated*.go' '*/config/crd/bases/*' config/crd/bases '*/config/rbac/*')"; \
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
