.PHONY: build test lint e2e up down clean coverage

SUVE_AWSMOCK_EXTERNAL_PORT ?= 4599

# Build
build:
	go build -o bin/suve ./cmd/suve

# Unit tests
test:
	go test ./...

# Lint
lint:
	golangci-lint run ./...

# Start awsmock container
up:
	SUVE_AWSMOCK_EXTERNAL_PORT=$(SUVE_AWSMOCK_EXTERNAL_PORT) docker compose up -d
	@echo "Waiting for awsmock to be ready on port $(SUVE_AWSMOCK_EXTERNAL_PORT)..."
	@sleep 5
	@until nc -z 127.0.0.1 $(SUVE_AWSMOCK_EXTERNAL_PORT) 2>/dev/null; do sleep 1; done
	@echo "awsmock is ready"

# Stop awsmock container
down:
	docker compose down

# E2E tests against local awsmock
e2e: up
	SUVE_AWSMOCK_PORT=$(SUVE_AWSMOCK_EXTERNAL_PORT) go test -tags=e2e -v ./e2e/...

# Clean
clean:
	rm -rf bin/
	docker compose down -v 2>/dev/null || true

# Coverage
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep total
