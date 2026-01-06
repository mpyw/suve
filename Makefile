.PHONY: build test lint e2e up down clean coverage coverage-e2e coverage-all gui-dev gui-build gui-bindings linux-gui linux-gui-build linux-gui-test linux-gui-setup linux-desktop help

.DEFAULT_GOAL := help

SUVE_LOCALSTACK_EXTERNAL_PORT ?= 4566
COVERPKG = $(shell go list ./... | grep -v testutil | grep -v /e2e | grep -v internal/gui | grep -v /cmd/ | tr '\n' ',')

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

%:
	@echo "make: *** Unknown target '$@'. See available targets below:" >&2
	@echo "" >&2
	@$(MAKE) -s help >&2
	@exit 1

build: ## Build CLI binary
	go build -o bin/suve ./cmd/suve

test: ## Run unit tests
	go test $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)

lint: ## Run linter
	golangci-lint run ./...
	golangci-lint run --build-tags=production ./...

up: ## Start localstack container
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) docker compose up -d
	@echo "Waiting for localstack to be ready on port $(SUVE_LOCALSTACK_EXTERNAL_PORT)..."
	@until curl -sf http://127.0.0.1:$(SUVE_LOCALSTACK_EXTERNAL_PORT)/_localstack/health > /dev/null 2>&1; do sleep 1; done
	@echo "localstack is ready"

down: ## Stop localstack container
	docker compose down

e2e: up ## Run E2E tests (starts localstack)
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -v ./e2e/...

clean: ## Clean build artifacts and stop containers
	rm -rf bin/ *.out
	docker compose down -v 2>/dev/null || true

coverage: ## Run unit tests with coverage
	go test -coverprofile=coverage.out -coverpkg=$(COVERPKG) $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)
	go tool cover -func=coverage.out | grep total

coverage-e2e: up ## Run E2E tests with coverage
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -coverpkg=$(COVERPKG) ./e2e/...
	go tool cover -func=coverage-e2e.out | grep total

coverage-all: up ## Run all tests with combined coverage
	@echo "Running unit tests with coverage..."
	go test -coverprofile=coverage-unit.out -covermode=atomic -coverpkg=$(COVERPKG) $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)
	@echo "Running E2E tests with coverage..."
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -covermode=atomic -coverpkg=$(COVERPKG) ./e2e/...
	@echo "Merging coverage profiles..."
	@go run github.com/wadey/gocovmerge@latest coverage-unit.out coverage-e2e.out > coverage-all.out
	go tool cover -func=coverage-all.out | grep total

gui-dev: ## Start GUI development server
	cd gui && wails dev -skipbindings -tags dev

gui-build: ## Build GUI for production
	cd gui && wails build -tags production -skipbindings

gui-bindings: ## Regenerate GUI bindings
	@echo "Temporarily removing build constraints..."
	@find gui internal/gui -name '*.go' -exec sed -i.bak 's|^//go:build.*||' {} \;
	@echo "Generating bindings..."
	@cd gui && timeout 30 wails dev -tags dev 2>&1 | head -30 || true
	@echo "Restoring build constraints..."
	@find gui internal/gui -name '*.go.bak' -exec sh -c 'mv "$$1" "$${1%.bak}"' _ {} \;
	@echo "Done. Check internal/gui/frontend/wailsjs/go/ for updated bindings."

# Linux GUI test environment
# Prerequisites (macOS):
#   1. brew install --cask xquartz
#   2. XQuartz preferences -> Security -> "Allow connections from network clients"
#   3. Restart XQuartz after changing the setting
linux-gui-setup:
	@echo "Setting up X11 forwarding for Linux GUI..."
	@bash docker/linux-gui/start.sh

linux-gui: linux-gui-setup ## Start Linux GUI container (requires XQuartz)
	@echo "Starting Linux GUI container..."
	HOST_DISPLAY=host.docker.internal:0 docker compose --profile linux-gui run --rm linux-gui

linux-gui-build: linux-gui-setup ## Build GUI in Linux container
	@echo "Building GUI in Linux container..."
	HOST_DISPLAY=host.docker.internal:0 docker compose --profile linux-gui run --rm linux-gui bash -c "cd gui && wails build -tags production -skipbindings"

linux-gui-test: linux-gui-setup ## Run GUI tests in Linux container
	@echo "Running GUI tests in Linux container..."
	HOST_DISPLAY=host.docker.internal:0 docker compose --profile linux-gui run --rm linux-gui bash -c "go test -tags=production ./internal/gui/..."

linux-desktop: linux-gui-setup ## Start Linux XFCE desktop (requires XQuartz)
	@echo "Starting Linux XFCE desktop..."
	HOST_DISPLAY=host.docker.internal:0 docker compose --profile linux-desktop run --rm linux-desktop
