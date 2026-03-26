VERSION    := $(shell git describe --tags --always --dirty)
COMMIT     := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X github.com/hairglasses-studio/ralphglasses/cmd.version=$(VERSION) \
              -X github.com/hairglasses-studio/ralphglasses/cmd.commit=$(COMMIT) \
              -X github.com/hairglasses-studio/ralphglasses/cmd.buildDate=$(BUILD_DATE)
PI_LDFLAGS := -X main.version=$(VERSION)
GO := ./scripts/dev/go.sh

.PHONY: bootstrap doctor test test-verbose test-cover test-integration test-scripts fuzz bench bench-compare build build-release install install-local build-prompt-improver install-prompt-improver vet lint ci clean release snapshot changelog mcp dev-mcp plugin-example hooks

# Install pre-commit hook (idempotent)
hooks:
	ln -sf ../../scripts/dev/pre-commit .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

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

# Run benchmarks (count=3 for statistical significance)
bench:
	$(GO) test -bench=. -benchmem -count=3 -run=^$$ ./... | tee bench-new.txt

# Compare benchmarks against previous run (requires bench-old.txt)
bench-compare: bench
	@if [ -f bench-old.txt ]; then \
		benchstat bench-old.txt bench-new.txt; \
	else \
		echo "No bench-old.txt found. Run 'make bench' on the baseline first, then 'cp bench-new.txt bench-old.txt'."; \
	fi

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

# Install to GOBIN (Go dev workflow — use install-local for system PATH)
install:
	$(GO) install -ldflags "$(LDFLAGS)" .

# Build release binary and install to system PATH
install-local: build-release
ifeq ($(shell uname),Darwin)
	@codesign -s - ralphglasses 2>/dev/null || true
endif
	@INSTALL_DIR=""; \
	if [ -w /usr/local/bin ]; then \
		INSTALL_DIR=/usr/local/bin; \
	elif [ -d $(HOME)/.local/bin ]; then \
		INSTALL_DIR=$(HOME)/.local/bin; \
	else \
		mkdir -p $(HOME)/.local/bin; \
		INSTALL_DIR=$(HOME)/.local/bin; \
	fi; \
	cp ralphglasses "$$INSTALL_DIR/ralphglasses"; \
	chmod 755 "$$INSTALL_DIR/ralphglasses"; \
	echo "Installed ralphglasses $(VERSION) to $$INSTALL_DIR/ralphglasses"; \
	case ":$$PATH:" in \
		*":$$INSTALL_DIR:"*) ;; \
		*) echo "WARNING: $$INSTALL_DIR is not on your PATH. Add it to your shell profile." ;; \
	esac

# Build prompt-improver binary
build-prompt-improver:
	$(GO) build -ldflags "$(PI_LDFLAGS)" -o prompt-improver ./cmd/prompt-improver

# Install prompt-improver binary
install-prompt-improver: build-prompt-improver
	sudo cp prompt-improver /usr/local/bin/prompt-improver
	@if [ "$$(uname)" = "Darwin" ]; then sudo codesign -f -s - /usr/local/bin/prompt-improver; fi
	@echo "prompt-improver installed to /usr/local/bin/prompt-improver"

# Run go vet
vet:
	$(GO) vet ./...

# Run golangci-lint (if installed)
lint:
	@./scripts/bootstrap-toolchain.sh >/dev/null
	@if [ -x ./.tools/bin/golangci-lint ]; then \
		./.tools/bin/golangci-lint run ./...; \
	elif command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint unavailable; run ./scripts/bootstrap-toolchain.sh"; \
		exit 1; \
	fi

# CI pipeline: bootstrap-aware vet + test + build
ci:
	./scripts/dev/ci.sh

# Remove build artifacts
clean:
	rm -f coverage.out
	rm -f ralphglasses
	rm -f prompt-improver
	$(GO) clean ./...

# Generate CHANGELOG.md from git log grouped by version tags
changelog:
	@echo "# Changelog\n" > CHANGELOG.md
	@echo "All notable changes to this project will be documented in this file.\n" >> CHANGELOG.md
	@TAGS=$$(git tag --sort=-version:refname); \
	if [ -z "$$TAGS" ]; then \
		echo "## Unreleased\n" >> CHANGELOG.md; \
		git log --oneline --no-merges --format="- %s ([%h](../../commit/%H))" >> CHANGELOG.md; \
	else \
		PREV="HEAD"; \
		for TAG in $$TAGS; do \
			echo "## $$TAG\n" >> CHANGELOG.md; \
			git log --oneline --no-merges --format="- %s ([%h](../../commit/%H))" "$$TAG..$$PREV" >> CHANGELOG.md; \
			echo "" >> CHANGELOG.md; \
			PREV="$$TAG"; \
		done; \
		echo "## Initial Release\n" >> CHANGELOG.md; \
		git log --oneline --no-merges --format="- %s ([%h](../../commit/%H))" "$$PREV" >> CHANGELOG.md; \
	fi
	@echo "Generated CHANGELOG.md"

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

# Plugin example target (placeholder — see internal/plugin/loader.go TODO)
# Actual .so plugin compilation requires: go build -buildmode=plugin -o my-plugin.so ./my-plugin/
# For production use, hashicorp/go-plugin is recommended over Go's native plugin package.
# See: https://github.com/hashicorp/go-plugin
plugin-example:
	@echo "Plugin system scaffolded. To build a plugin as a .so:"
	@echo "  $(GO) build -buildmode=plugin -o my-plugin.so ./path/to/plugin/"
	@echo ""
	@echo "Built-in logger plugin: internal/plugin/builtin/logger.go"
	@echo "For production plugins, see the TODO in internal/plugin/loader.go"
