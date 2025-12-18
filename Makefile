# TOS Pool Makefile

VERSION := 1.0.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build clean test run deps

all: build

# Install dependencies
deps:
	go mod tidy
	go mod download

# Build the pool binary
build: deps
	go build $(LDFLAGS) -o bin/tos-pool ./cmd/tos-pool

# Build for Linux (cross-compile from macOS)
build-linux: deps
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/tos-pool-linux-amd64 ./cmd/tos-pool

# Run tests
test:
	go test -v ./...

# Run with default config
run: build
	./bin/tos-pool --config config/config.example.yaml

# Run in master mode
run-master: build
	./bin/tos-pool --config config/config.example.yaml --mode master

# Run in slave mode
run-slave: build
	./bin/tos-pool --config config/config.example.yaml --mode slave

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Docker build
docker-build:
	docker build -t tos-network/tos-pool:$(VERSION) .

# Docker compose up
docker-up:
	docker-compose -f docker/docker-compose.yaml up -d

# Docker compose down
docker-down:
	docker-compose -f docker/docker-compose.yaml down

# Generate mock data for testing
mock-data:
	@echo "Generating mock data..."
	redis-cli SET "tos:stats" '{"blocksFound":10,"lastBlockFound":1702900000}'

# Show help
help:
	@echo "TOS Pool - Mining pool for TOS Hash V3"
	@echo ""
	@echo "Usage:"
	@echo "  make build       - Build the pool binary"
	@echo "  make build-linux - Cross-compile for Linux"
	@echo "  make run         - Run with default config"
	@echo "  make run-master  - Run in master mode"
	@echo "  make run-slave   - Run in slave mode"
	@echo "  make test        - Run tests"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make docker-build - Build Docker image"
	@echo "  make docker-up   - Start with Docker Compose"
	@echo "  make docker-down - Stop Docker Compose"
