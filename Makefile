.PHONY: build test lint e2e up down clean coverage coverage-e2e coverage-all gui-dev gui-build gui-bindings linux-gui linux-gui-build linux-gui-test linux-gui-setup

SUVE_LOCALSTACK_EXTERNAL_PORT ?= 4566
COVERPKG = $(shell go list ./... | grep -v testutil | grep -v /e2e | grep -v internal/gui | grep -v /cmd/ | tr '\n' ',')

# Build
build:
	go build -o bin/suve ./cmd/suve

# Unit tests (exclude internal/gui and cmd/)
test:
	go test $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)

# Lint (both default and production builds)
lint:
	golangci-lint run ./...
	golangci-lint run --build-tags=production ./...

# Start localstack container
up:
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) docker compose up -d
	@echo "Waiting for localstack to be ready on port $(SUVE_LOCALSTACK_EXTERNAL_PORT)..."
	@until curl -sf http://127.0.0.1:$(SUVE_LOCALSTACK_EXTERNAL_PORT)/_localstack/health > /dev/null 2>&1; do sleep 1; done
	@echo "localstack is ready"

# Stop localstack container
down:
	docker compose down

# E2E tests (SSM + SM)
e2e: up
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -v ./e2e/...

# Clean
clean:
	rm -rf bin/ *.out
	docker compose down -v 2>/dev/null || true

# Unit test coverage (exclude testutil, e2e, internal/gui, and cmd/)
coverage:
	go test -coverprofile=coverage.out -coverpkg=$(COVERPKG) $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)
	go tool cover -func=coverage.out | grep total

# E2E test coverage (requires localstack running)
coverage-e2e: up
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -coverpkg=$(COVERPKG) ./e2e/...
	go tool cover -func=coverage-e2e.out | grep total

# Combined coverage (unit + E2E)
coverage-all: up
	@echo "Running unit tests with coverage..."
	go test -coverprofile=coverage-unit.out -covermode=atomic -coverpkg=$(COVERPKG) $(shell go list ./... | grep -v internal/gui | grep -v /cmd/)
	@echo "Running E2E tests with coverage..."
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -covermode=atomic -coverpkg=$(COVERPKG) ./e2e/...
	@echo "Merging coverage profiles..."
	@go run github.com/wadey/gocovmerge@latest coverage-unit.out coverage-e2e.out > coverage-all.out
	go tool cover -func=coverage-all.out | grep total

# GUI development server
gui-dev:
	cd gui && wails dev -skipbindings -tags dev

# GUI production build
gui-build:
	cd gui && wails build -tags production -skipbindings

# GUI bindings regeneration (temporarily removes build constraints)
gui-bindings:
	@echo "Temporarily removing build constraints..."
	@find gui internal/gui -name '*.go' -exec sed -i.bak 's|^//go:build.*||' {} \;
	@echo "Generating bindings..."
	@cd gui && timeout 30 wails dev -tags dev 2>&1 | head -30 || true
	@echo "Restoring build constraints..."
	@find gui internal/gui -name '*.go.bak' -exec sh -c 'mv "$$1" "$${1%.bak}"' _ {} \;
	@echo "Done. Check internal/gui/frontend/wailsjs/go/ for updated bindings."

# Linux GUI test environment (requires XQuartz on macOS)
linux-gui-setup:
	@echo "Setting up X11 forwarding for Linux GUI..."
	@bash docker/linux-gui/start.sh

linux-gui: linux-gui-setup
	@echo "Starting Linux GUI container..."
	docker compose --profile linux-gui run --rm linux-gui

linux-gui-build:
	@echo "Building GUI in Linux container..."
	docker compose --profile linux-gui run --rm linux-gui bash -c "cd gui && wails build -tags production -skipbindings"

linux-gui-test:
	@echo "Running GUI tests in Linux container..."
	docker compose --profile linux-gui run --rm linux-gui bash -c "go test -tags=production ./internal/gui/..."
