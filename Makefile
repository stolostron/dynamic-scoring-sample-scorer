IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_NAME ?= dynamic-scoring-framework-sample-scorer
IMAGE_TAG ?= latest
IMG ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

CONTAINER_TOOL ?= docker
GO ?= go
GO_VERSION ?= $(shell sed -n 's/^go //p' go.mod | cut -d. -f1,2)
GOLANGCI_LINT_VERSION ?= v2.12.2
HELM ?= helm
CHART_PATH ?= charts/sample-scorer
BIN_DIR ?= bin
GOLANGCI_LINT_BIN ?= $(BIN_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Format Go source files.
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet.
	$(GO) vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint.
	$(GOLANGCI_LINT_BIN) run ./...

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT_BIN) ## Install golangci-lint if it is not already available.

$(GOLANGCI_LINT_BIN): | $(BIN_DIR)
	GOBIN="$(abspath $(BIN_DIR))" $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	mv "$(BIN_DIR)/golangci-lint" "$@"

$(BIN_DIR):
	mkdir -p "$(BIN_DIR)"

.PHONY: test
test: ## Run the scorer tests.
	$(GO) test ./...

.PHONY: verify
verify: fmt vet lint test ## Format, vet, lint, and test the scorer.

##@ Build

.PHONY: build
build: ## Build the scorer binary.
	mkdir -p "$(BIN_DIR)"
	$(GO) build -trimpath -o "$(BIN_DIR)/sample-scorer" ./cmd/sample-scorer

.PHONY: run
run: ## Run the scorer locally.
	$(GO) run ./cmd/sample-scorer

.PHONY: docker-build
docker-build: ## Build the scorer container image.
	$(CONTAINER_TOOL) build $(IMAGE_BUILD_EXTRA_FLAGS) --build-arg GO_VERSION="$(GO_VERSION)" -t "$(IMG)" -f Dockerfile .

.PHONY: docker-push
docker-push: ## Push the scorer container image.
	$(CONTAINER_TOOL) push "$(IMG)"

.PHONY: image
image: docker-build ## Build the scorer container image.

.PHONY: image-push
image-push: docker-push ## Push the scorer container image.

##@ Chart

.PHONY: chart-lint
chart-lint: ## Lint the sample scorer Helm chart with its local fixture values.
	$(HELM) lint "$(CHART_PATH)" -f "$(CHART_PATH)/tests/values.yaml"

.PHONY: chart-template
chart-template: ## Render the sample scorer Helm chart with its local fixture values.
	$(HELM) template sample-scorer "$(CHART_PATH)" -f "$(CHART_PATH)/tests/values.yaml"
