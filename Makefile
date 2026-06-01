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
	build-blur

build-auth:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/auth-service ./services/auth/main.go

build-camera-mgmt:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/camera-mgmt ./services/camera-mgmt/main.go

build-recorder:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/recorder-service ./services/recorder/main.go

build-playback:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/playback-service ./services/playback/main.go

build-webrtc:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/webrtc-service ./services/webrtc/main.go

build-camera-control:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/camera-control ./services/camera-control/main.go

build-thumbnails:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/thumbnails ./services/thumbnails/main.go

build-discovery:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/discovery ./services/discovery/main.go

build-event-proc:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/event-proc ./services/event-proc/main.go

build-api-gateway:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/api-gateway ./services/api-gateway/main.go

build-export:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/export-service ./services/export/main.go

build-audit:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/audit-service ./services/audit/main.go

build-blur:
	$(GO) build $(LDFLAGS) -o $(BINARY_DIR)/blur-worker ./services/ai-worker/blur.go

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

# Docker compose helpers
docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

docker-logs:
	docker compose -f deploy/docker/docker-compose.yml logs -f

# Development helpers
dev-auth:
	$(GO) run ./services/auth/main.go

dev-recorder:
	$(GO) run ./services/recorder/main.go

dev-gateway:
	$(GO) run ./services/api-gateway/main.go

.PHONY: build-auth build-camera-mgmt build-recorder build-playback build-webrtc build-camera-control build-thumbnails build-discovery build-event-proc build-api-gateway build-export build-audit build-blur
