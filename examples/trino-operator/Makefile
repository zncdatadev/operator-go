SHELL := /bin/bash
.SHELLFLAGS := -euo pipefail -c
SDK_ROOT := ../..
GO ?= go
IMG ?= trino-operator:dev
CONTAINER_TOOL ?= docker
TARGETOS ?= linux
TARGETARCH ?= $(shell $(GO) env GOARCH)
KUSTOMIZE ?= kustomize
GOLANGCI_LINT ?= $(SDK_ROOT)/bin/golangci-lint
KUBEBUILDER_ASSETS ?= $(abspath $(SDK_ROOT)/bin/k8s/1.35.0-$(shell $(GO) env GOOS)-$(shell $(GO) env GOARCH))

.PHONY: all generate manifests verify-generate fmt vet test lint build build-linux run docker-build docker-push build-installer install uninstall deploy undeploy
all: build

generate manifests:
	$(GO) run -mod=readonly ./cmd/generate

verify-generate:
	$(GO) run -mod=readonly ./cmd/generate -check

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test: verify-generate
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" $(GO) test -mod=readonly ./... -count=1

lint:
	$(GOLANGCI_LINT) run ./...

build: verify-generate
	$(GO) build -mod=readonly -trimpath -buildvcs=false -o bin/manager ./cmd

build-linux: verify-generate
	mkdir -p bin/image
	CGO_ENABLED=0 GOOS=$(TARGETOS) GOARCH=$(TARGETARCH) $(GO) build -mod=readonly -trimpath -buildvcs=false -o bin/image/manager ./cmd

run: verify-generate
	$(GO) run -mod=readonly ./cmd $(ARGS)

docker-build: build-linux
	$(CONTAINER_TOOL) build --platform $(TARGETOS)/$(TARGETARCH) -f Dockerfile -t $(IMG) bin/image

docker-push:
	$(CONTAINER_TOOL) push $(IMG)

build-installer: verify-generate
	mkdir -p dist
	@overlay=$$(mktemp -d config/.installer.XXXXXX); \
	trap 'rm -rf "$$overlay"' EXIT; \
	printf '%s\n' 'resources:' '- ../default' > "$$overlay/kustomization.yaml"; \
	(cd "$$overlay" && $(KUSTOMIZE) edit set image "trino-operator=$(IMG)"); \
	$(KUSTOMIZE) build "$$overlay" > dist/install.yaml

install: verify-generate
	$(KUSTOMIZE) build config/crd | kubectl apply --validate=strict -f -

uninstall:
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: build-installer
	kubectl apply --validate=strict -f dist/install.yaml

undeploy:
	$(KUSTOMIZE) build config/default | kubectl delete -f -
