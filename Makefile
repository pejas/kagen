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

.PHONY: all build test test-e2e lint clean install runtime-images-lock runtime-images-build-local help

all: build

## build: compile the binary into bin/
build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./$(BUILD_DIR)

## test: run unit and integration tests with race detector (excluding internal/e2e)
test: build
	$(GOTEST) -race -count=1 -v $(TEST_PKGS)

## test-e2e: run the end-to-end suite explicitly
test-e2e: build
	$(GOTEST) -race -count=1 -v ./internal/e2e

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

## help: show this help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //' | column -t -s ':'
