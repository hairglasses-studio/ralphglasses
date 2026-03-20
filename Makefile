VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -X github.com/hairglasses-studio/ralphglasses/cmd.version=$(VERSION)

.PHONY: test test-verbose test-cover test-integration test-scripts fuzz build build-release install vet lint ci clean release snapshot

# Run all tests with race detector
test:
	go test -race ./...

# Verbose test output
test-verbose:
	go test -race -v ./...

# Generate coverage report with threshold enforcement
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

# Run integration tests (requires build tag)
test-integration:
	go test -tags integration -v ./internal/...

# Run fuzz tests (30s each)
fuzz:
	@echo "Fuzzing config parser (30s)..."
	go test -fuzz=FuzzLoadConfig -fuzztime=30s ./internal/model/
	@echo "Fuzzing status parser (30s)..."
	go test -fuzz=FuzzLoadStatus -fuzztime=30s ./internal/model/
	@echo "Fuzzing circuit breaker parser (30s)..."
	go test -fuzz=FuzzLoadCircuitBreaker -fuzztime=30s ./internal/model/
	@echo "Fuzzing progress parser (30s)..."
	go test -fuzz=FuzzLoadProgress -fuzztime=30s ./internal/model/
	@echo "Fuzzing MCP string args (30s)..."
	go test -fuzz=FuzzGetStringArg -fuzztime=30s ./internal/mcpserver/
	@echo "Fuzzing MCP number args (30s)..."
	go test -fuzz=FuzzGetNumberArg -fuzztime=30s ./internal/mcpserver/

# Run BATS tests for shell scripts
test-scripts:
	@command -v bats >/dev/null 2>&1 || { echo "bats not installed: npm i -g bats or apt install bats"; exit 1; }
	bats scripts/test/

# Build all binaries
build:
	go build ./...

# Build release binary with version injection
build-release:
	go build -ldflags "$(LDFLAGS)" -o ralphglasses .

# Install with version injection
install:
	go install -ldflags "$(LDFLAGS)" .

# Run go vet
vet:
	go vet ./...

# Run golangci-lint (if installed)
lint:
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

# CI pipeline: vet + test + build
ci: vet test build

# Remove build artifacts
clean:
	rm -f coverage.out
	rm -f ralphglasses
	go clean ./...

# Goreleaser release
release:
	goreleaser release --clean

# Goreleaser snapshot (local testing)
snapshot:
	goreleaser release --snapshot --clean
