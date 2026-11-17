IMAGE_REGISTRY ?= quay.io/open-cluster-management
IMAGE_NAME ?= dynamic-scoring-sample-scorer
IMAGE_TAG ?= latest
IMG ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

CONTAINER_TOOL ?= docker
PYTHON ?= python3
PIP ?= $(PYTHON) -m pip
HELM ?= helm
CHART_PATH ?= charts/sample-scorer
PORT ?= 8000

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec
.DEFAULT_GOAL := help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: install
install: ## Install the scorer and development dependencies.
	$(PIP) install -r requirements-dev.txt

.PHONY: run
run: ## Run the scorer locally with auto-reload.
	$(PYTHON) -m uvicorn app.main:app --reload --host 0.0.0.0 --port $(PORT)

.PHONY: test
test: ## Run the scorer tests.
	$(PYTHON) -m pytest -q

##@ Build

.PHONY: docker-build
docker-build: ## Build the scorer container image.
	$(CONTAINER_TOOL) build $(IMAGE_BUILD_EXTRA_FLAGS) -t "$(IMG)" -f Dockerfile .

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
