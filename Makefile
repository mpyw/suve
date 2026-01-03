.PHONY: build test lint e2e up down clean coverage coverage-e2e coverage-all

SUVE_LOCALSTACK_EXTERNAL_PORT ?= 4566
COVERPKG = $(shell go list ./... | grep -v testutil | grep -v /e2e | tr '\n' ',')

# Build
build:
	go build -o bin/suve ./cmd/suve

# Unit tests
test:
	go test ./...

# Lint
lint:
	golangci-lint run ./...

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

# Unit test coverage (exclude testutil and e2e)
coverage:
	go test -coverprofile=coverage.out -coverpkg=$(COVERPKG) ./...
	go tool cover -func=coverage.out | grep total

# E2E test coverage (requires localstack running)
coverage-e2e: up
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -coverpkg=$(COVERPKG) ./e2e/...
	go tool cover -func=coverage-e2e.out | grep total

# Combined coverage (unit + E2E)
coverage-all: up
	@echo "Running unit tests with coverage..."
	go test -coverprofile=coverage-unit.out -covermode=atomic -coverpkg=$(COVERPKG) ./...
	@echo "Running E2E tests with coverage..."
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -coverprofile=coverage-e2e.out -covermode=atomic -coverpkg=$(COVERPKG) ./e2e/...
	@echo "Merging coverage profiles..."
	@go run github.com/wadey/gocovmerge@latest coverage-unit.out coverage-e2e.out > coverage-all.out
	go tool cover -func=coverage-all.out | grep total
