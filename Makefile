APP_NAME    := kagen
MODULE      := github.com/pejas/kagen
BIN_DIR     := bin
BUILD_DIR   := cmd/kagen

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE  ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X '$(MODULE)/internal/cmd.Version=$(VERSION)' \
	-X '$(MODULE)/internal/cmd.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/cmd.BuildDate=$(BUILD_DATE)'

GO       := go
GOFLAGS  := -trimpath
GOTEST   := $(GO) test
GOLINT   := golangci-lint
TEST_PKGS := $(shell $(GO) list ./... | grep -v '/internal/e2e$$')

# Docker image configuration
REGISTRY    := ghcr.io/pejas
IMAGE_PREFIX := kagen
PLATFORMS   := linux/amd64,linux/arm64

# Runtime image names
BASE_IMAGE      := $(REGISTRY)/$(IMAGE_PREFIX)-base
WORKSPACE_IMAGE := $(REGISTRY)/$(IMAGE_PREFIX)-workspace
TOOLBOX_IMAGE   := $(REGISTRY)/$(IMAGE_PREFIX)-toolbox
PROXY_IMAGE     := $(REGISTRY)/$(IMAGE_PREFIX)-proxy

# Docker buildx configuration
BUILDX_BUILDER  := kagen-builder
RUNTIME_IMAGES_DIR := $(CURDIR)/packaging/runtime-images

.PHONY: all build test test-e2e lint clean install runtime-images-lock runtime-images-build-local runtime-images-build runtime-images-push help

all: build

## build: compile the binary into bin/
build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./$(BUILD_DIR)

## test: run unit and integration tests with race detector (excluding internal/e2e)
test: build
	$(GOTEST) -race -count=1 -v $(TEST_PKGS)

## test-e2e: run the end-to-end suite explicitly
test-e2e: build
	$(GOTEST) -timeout=20m -race -count=1 -v ./internal/e2e

## lint: run golangci-lint
lint:
	$(GOLINT) run ./...

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## install: install binary to GOPATH/bin
install:
	$(GO) install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./$(BUILD_DIR)

## runtime-images-lock: refresh the toolbox mise.lock for Linux runtime platforms
runtime-images-lock:
	env MISE_STATE_DIR=$(CURDIR)/.mise-state MISE_CACHE_DIR=$(CURDIR)/.mise-cache mise trust $(CURDIR)/packaging/runtime-images/toolbox/mise.toml
	env MISE_STATE_DIR=$(CURDIR)/.mise-state MISE_CACHE_DIR=$(CURDIR)/.mise-cache mise lock --yes --platform linux-arm64,linux-x64 --cd $(CURDIR)/packaging/runtime-images/toolbox

## runtime-images-build-local: build local workspace/toolbox/proxy images for the Colima runtime
runtime-images-build-local:
	docker build -f $(CURDIR)/packaging/runtime-images/base/Dockerfile -t ghcr.io/pejas/kagen-base:local $(CURDIR)/packaging/runtime-images/base
	docker build -f $(CURDIR)/packaging/runtime-images/workspace/Dockerfile -t ghcr.io/pejas/kagen-workspace:local --build-arg KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:local $(CURDIR)/packaging/runtime-images
	docker build -f $(CURDIR)/packaging/runtime-images/toolbox/Dockerfile -t ghcr.io/pejas/kagen-toolbox:local --build-arg KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:local $(CURDIR)/packaging/runtime-images
	docker build -f $(CURDIR)/packaging/runtime-images/proxy/Dockerfile -t ghcr.io/pejas/kagen-proxy:local --build-arg KAGEN_BASE_IMAGE=ghcr.io/pejas/kagen-base:local $(CURDIR)/packaging/runtime-images

## runtime-images-build: build multi-platform runtime images for CI (requires docker buildx)
runtime-images-build:
	@echo "Setting up Docker buildx builder..."
	@docker buildx inspect $(BUILDX_BUILDER) >/dev/null 2>&1 || docker buildx create --name $(BUILDX_BUILDER) --driver docker-container --bootstrap
	@docker buildx use $(BUILDX_BUILDER)
	@echo "Building base image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/base/Dockerfile \
		--tag $(BASE_IMAGE):$(VERSION) \
		--tag $(BASE_IMAGE):$(COMMIT) \
		--tag $(BASE_IMAGE):latest \
		$(RUNTIME_IMAGES_DIR)/base
	@echo "Building workspace image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/workspace/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(WORKSPACE_IMAGE):$(VERSION) \
		--tag $(WORKSPACE_IMAGE):$(COMMIT) \
		--tag $(WORKSPACE_IMAGE):latest \
		$(RUNTIME_IMAGES_DIR)
	@echo "Building toolbox image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/toolbox/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(TOOLBOX_IMAGE):$(VERSION) \
		--tag $(TOOLBOX_IMAGE):$(COMMIT) \
		--tag $(TOOLBOX_IMAGE):latest \
		$(RUNTIME_IMAGES_DIR)
	@echo "Building proxy image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/proxy/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(PROXY_IMAGE):$(VERSION) \
		--tag $(PROXY_IMAGE):$(COMMIT) \
		--tag $(PROXY_IMAGE):latest \
		$(RUNTIME_IMAGES_DIR)
	@echo "Multi-platform build complete."
	@docker buildx rm $(BUILDX_BUILDER) 2>/dev/null || true

## runtime-images-push: build and push multi-platform runtime images to GHCR
runtime-images-push:
	@echo "Logging into GHCR..."
	@if [ -z "$$GITHUB_TOKEN" ]; then echo "Error: GITHUB_TOKEN not set"; exit 1; fi
	@echo $$GITHUB_TOKEN | docker login ghcr.io -u $$GITHUB_ACTOR --password-stdin
	@echo "Setting up Docker buildx builder..."
	@docker buildx inspect $(BUILDX_BUILDER) >/dev/null 2>&1 || docker buildx create --name $(BUILDX_BUILDER) --driver docker-container --bootstrap
	@docker buildx use $(BUILDX_BUILDER)
	@echo "Building and pushing base image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/base/Dockerfile \
		--tag $(BASE_IMAGE):$(VERSION) \
		--tag $(BASE_IMAGE):$(COMMIT) \
		--tag $(BASE_IMAGE):latest \
		--push \
		$(RUNTIME_IMAGES_DIR)/base
	@echo "Building and pushing workspace image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/workspace/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(WORKSPACE_IMAGE):$(VERSION) \
		--tag $(WORKSPACE_IMAGE):$(COMMIT) \
		--tag $(WORKSPACE_IMAGE):latest \
		--push \
		$(RUNTIME_IMAGES_DIR)
	@echo "Building and pushing toolbox image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/toolbox/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(TOOLBOX_IMAGE):$(VERSION) \
		--tag $(TOOLBOX_IMAGE):$(COMMIT) \
		--tag $(TOOLBOX_IMAGE):latest \
		--push \
		$(RUNTIME_IMAGES_DIR)
	@echo "Building and pushing proxy image..."
	@docker buildx build \
		--platform $(PLATFORMS) \
		--file $(RUNTIME_IMAGES_DIR)/proxy/Dockerfile \
		--build-arg KAGEN_BASE_IMAGE=$(BASE_IMAGE):$(VERSION) \
		--tag $(PROXY_IMAGE):$(VERSION) \
		--tag $(PROXY_IMAGE):$(COMMIT) \
		--tag $(PROXY_IMAGE):latest \
		--push \
		$(RUNTIME_IMAGES_DIR)
	@echo "Multi-platform build and push complete."
	@echo "Images published to:"
	@echo "  - $(BASE_IMAGE):$(VERSION)"
	@echo "  - $(WORKSPACE_IMAGE):$(VERSION)"
	@echo "  - $(TOOLBOX_IMAGE):$(VERSION)"
	@echo "  - $(PROXY_IMAGE):$(VERSION)"
	@docker buildx rm $(BUILDX_BUILDER) 2>/dev/null || true

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'
