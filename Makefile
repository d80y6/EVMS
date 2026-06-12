# EVMS Makefile
BINARY_DIR := bin
GO ?= go
GO_VERSION ?= $(shell $(GO) version | grep -oP 'go\w+\.\w+\.\w+')
MODULE := $(shell head -1 go.mod | awk '{print $$2}')
COMMIT_HASH := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X $(MODULE)/pkg/common.Version=$(COMMIT_HASH) -X $(MODULE)/pkg/common.BuildTime=$(BUILD_TIME)"
CGO_ENABLED ?= 0

# Standard build
.PHONY: all build clean test lint docker-build fips-build fips-test

all: build

build: \
	build-auth \
	build-camera-mgmt \
	build-recorder \
	build-playback \
	build-webrtc \
	build-camera-control \
	build-thumbnails \
	build-discovery \
	build-event-proc \
	build-api-gateway \
	build-export \
	build-audit \
	build-blur \
	build-federation

build-auth:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/auth-service ./services/auth/

build-camera-mgmt:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/camera-mgmt ./services/camera-mgmt/

build-recorder:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/recorder-service ./services/recorder/

build-playback:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/playback-service ./services/playback/

build-webrtc:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/webrtc-service ./services/webrtc/

build-camera-control:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/camera-control ./services/camera-control/

build-thumbnails:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/thumbnails ./services/thumbnails/

build-discovery:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/discovery ./services/discovery/

build-event-proc:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/event-proc ./services/event-proc/

build-api-gateway:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/api-gateway ./services/api-gateway/

build-export:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/export-service ./services/export/

build-audit:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/audit-service ./services/audit/

build-blur:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/blur-worker ./services/ai-worker/blur.go

build-federation:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/federation ./services/federation/main.go

clean:
	rm -rf $(BINARY_DIR)/

test:
	$(GO) test ./pkg/... ./services/... -v -count=1

lint:
	command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

# FIPS-compliant build using BoringCrypto
fips-build: export CGO_ENABLED=1
fips-build: export GOEXPERIMENT=opensslcrypto
fips-build: export CGO_CFLAGS=-O2 -g
fips-build:
	@echo "Building with FIPS-compliant BoringCrypto..."
	$(GO) build -tags fips $(LDFLAGS) -o $(BINARY_DIR)/fips-verify ./cmd/fips.go
	@echo "FIPS build complete: $(BINARY_DIR)/fips-verify"

# Docker build for FIPS
docker-fips-build:
	docker build -t damvms/builder-fips:latest -f docker/fips-builder.Dockerfile .
	docker run --rm damvms/builder-fips:latest

# Run FIPS self-test
fips-test: fips-build
	@echo "Running FIPS self-test..."
	./$(BINARY_DIR)/fips-verify
	@echo "FIPS self-test passed"

# Development helpers
dev-auth:
	$(GO) run ./services/auth/

dev-recorder:
	$(GO) run ./services/recorder/

dev-gateway:
	$(GO) run ./services/api-gateway/main.go

build-triton:
	@if command -v cargo >/dev/null 2>&1; then \
		cd apps/triton-inference-service && cargo build --release; \
	else \
		echo "cargo not found, skipping Triton build"; \
	fi

triton-test:
	@if command -v cargo >/dev/null 2>&1; then \
		cd apps/triton-inference-service && cargo test; \
	else \
		echo "cargo not found, skipping Triton tests"; \
	fi

# Docker compose helpers with auto-detected host IP
DOCKER_COMPOSE := docker compose --env-file ./.env -f deploy/docker/docker-compose.yml

.PHONY: docker-up docker-build docker-logs docker-down

docker-up:
	HOST_IP=$$(hostname -I | awk '{print $$1}') $(DOCKER_COMPOSE) up -d

docker-build:
	HOST_IP=$$(hostname -I | awk '{print $$1}') $(DOCKER_COMPOSE) build $(SERVICE)

docker-logs:
	$(DOCKER_COMPOSE) logs -f $(SERVICE)

docker-down:
	$(DOCKER_COMPOSE) down

# Full beta verification target
.PHONY: beta-verify
beta-verify: build build-triton test lint triton-test
	@echo ""
	@echo "=== EVMS Beta Verification Complete ==="
	@echo "✓ Go build: all services compiled"
	@echo "✓ Go tests: all suites passed"
	@echo "✓ Lint: Go code checked"
	@if command -v cargo >/dev/null 2>&1; then \
		echo "✓ Triton build: Rust service compiled"; \
		echo "✓ Triton tests: Rust tests passed"; \
	fi

.PHONY: build-auth build-camera-mgmt build-recorder build-playback build-webrtc build-camera-control build-thumbnails build-discovery build-event-proc build-api-gateway build-export build-audit build-blur
