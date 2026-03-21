VERSION := $(shell git describe --tags --always --dirty)
COMMIT  := $(shell git rev-parse --short HEAD)
LDFLAGS := -X github.com/hairglasses-studio/ralphglasses/cmd.version=$(VERSION) -X github.com/hairglasses-studio/ralphglasses/cmd.commit=$(COMMIT)
GO := ./scripts/dev/go.sh

.PHONY: bootstrap doctor test test-verbose test-cover test-integration test-scripts fuzz build build-release install vet lint ci clean release snapshot mcp dev-mcp

bootstrap:
	./scripts/bootstrap-toolchain.sh

doctor:
	./scripts/dev/doctor.sh

# Run all tests with race detector
test:
	$(GO) test -race ./...

# Verbose test output
test-verbose:
	$(GO) test -race -v ./...

# Generate coverage report with threshold enforcement
test-cover:
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: ./scripts/dev/go.sh tool cover -html=coverage.out"

# Run integration tests (requires build tag)
test-integration:
	$(GO) test -tags integration -v ./internal/...

# Run fuzz tests (30s each)
fuzz:
	@echo "Fuzzing config parser (30s)..."
	$(GO) test -fuzz=FuzzLoadConfig -fuzztime=30s ./internal/model/
	@echo "Fuzzing status parser (30s)..."
	$(GO) test -fuzz=FuzzLoadStatus -fuzztime=30s ./internal/model/
	@echo "Fuzzing circuit breaker parser (30s)..."
	$(GO) test -fuzz=FuzzLoadCircuitBreaker -fuzztime=30s ./internal/model/
	@echo "Fuzzing progress parser (30s)..."
	$(GO) test -fuzz=FuzzLoadProgress -fuzztime=30s ./internal/model/
	@echo "Fuzzing MCP string args (30s)..."
	$(GO) test -fuzz=FuzzGetStringArg -fuzztime=30s ./internal/mcpserver/
	@echo "Fuzzing MCP number args (30s)..."
	$(GO) test -fuzz=FuzzGetNumberArg -fuzztime=30s ./internal/mcpserver/

# Run BATS tests for shell scripts
test-scripts:
	@command -v bats >/dev/null 2>&1 || { echo "bats not installed: use .devcontainer or your system package manager"; exit 1; }
	bats scripts/test/

# Build all binaries
build:
	$(GO) build ./...

# Build release binary with version injection
build-release:
	$(GO) build -ldflags "$(LDFLAGS)" -o ralphglasses .

# Install with version injection
install:
	$(GO) install -ldflags "$(LDFLAGS)" .

# Run go vet
vet:
	$(GO) vet ./...

# Run golangci-lint (if installed)
lint:
	@./scripts/bootstrap-toolchain.sh >/dev/null
	@./.tools/bin/golangci-lint run ./... || echo "golangci-lint unavailable; run ./scripts/bootstrap-toolchain.sh"

# CI pipeline: bootstrap-aware vet + test + build
ci:
	./scripts/dev/ci.sh

# Remove build artifacts
clean:
	rm -f coverage.out
	rm -f ralphglasses
	$(GO) clean ./...

# Goreleaser release
release:
	goreleaser release --clean

# Goreleaser snapshot (local testing)
snapshot:
	goreleaser release --snapshot --clean

# Rebuild binary then confirm ready for MCP
mcp: build-release
	@echo "Binary rebuilt: ./ralphglasses ($(VERSION) $(COMMIT))"

# Run MCP server with live compilation (always fresh code)
dev-mcp:
	./scripts/dev/run-mcp.sh --scan-path ~/hairglasses-studio
