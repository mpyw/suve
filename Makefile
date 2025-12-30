.PHONY: build test lint e2e e2e-ssm up down clean coverage

SUVE_LOCALSTACK_EXTERNAL_PORT ?= 4566
export GOEXPERIMENT := jsonv2

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

# E2E tests (SSM only, SM requires localstack Pro)
e2e: e2e-ssm

# E2E tests for SSM only
e2e-ssm: up
	SUVE_LOCALSTACK_EXTERNAL_PORT=$(SUVE_LOCALSTACK_EXTERNAL_PORT) go test -tags=e2e -v -run TestSSM ./e2e/...

# Clean
clean:
	rm -rf bin/
	docker compose down -v 2>/dev/null || true

# Coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep total
