VERSION    := $(shell git describe --tags --always --dirty)
COMMIT     := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w -trimpath \
              -X github.com/hairglasses-studio/ralphglasses/cmd.version=$(VERSION) \
              -X github.com/hairglasses-studio/ralphglasses/cmd.commit=$(COMMIT) \
              -X github.com/hairglasses-studio/ralphglasses/cmd.buildDate=$(BUILD_DATE)
PI_LDFLAGS := -s -w -trimpath -X main.version=$(VERSION)
GO := ./scripts/dev/go.sh

.PHONY: bootstrap doctor test test-verbose test-cover test-cover-strict test-integration test-scripts smoke fuzz bench bench-compare bench-dashboard build build-release install install-local build-prompt-improver install-prompt-improver vet lint ci clean release snapshot changelog mcp dev-mcp plugin-example hooks docker docker-run man install-man coverage-badge coverage-report coverage-treemap skill-doc skill-surface update-tool-counts codex-danger-full-access

# Install pre-commit hook (idempotent)
hooks:
	ln -sf ../../scripts/dev/pre-commit .git/hooks/pre-commit
	@echo "Pre-commit hook installed"

bootstrap:
	./scripts/bootstrap-toolchain.sh

doctor:
	./scripts/dev/doctor.sh

codex-danger-full-access:
	bash ./scripts/dev/set-codex-danger-full-access.sh

# Run all tests with race detector
test:
	$(GO) test -race ./...

# Verbose test output
test-verbose:
	$(GO) test -race -v ./...

# Generate coverage report with threshold enforcement
test-cover:
	@mkdir -p .ralph
	$(GO) test -race -coverprofile=coverage.out ./... | tee .ralph/test-cover-output.txt
	$(GO) tool cover -func=coverage.out
	@$(GO) tool cover -func=coverage.out | tail -1 | awk '{print $$NF}' | tr -d '%' > .ralph/coverage.txt
	@echo "Coverage written to .ralph/coverage.txt: $$(cat .ralph/coverage.txt)%"
	@echo ""
	@echo "=== Per-package coverage summary (sorted) ==="
	@grep -E 'coverage:' .ralph/test-cover-output.txt | \
		sed 's/.*ok[[:space:]]*//' | \
		awk '{pkg=$$1; pct=0; for(i=1;i<=NF;i++){if($$i=="coverage:"){pct=$$(i+1)+0}}} pct>0{printf "%6.1f  %s\n", pct, pkg}' | \
		sort -rn | \
		awk '{pct=$$1+0; warn=""; if (pct < 60) warn=" [NEEDS ATTENTION]"; printf "  %-70s %5.1f%%%s\n", $$2, pct, warn}'
	@echo ""
	@echo "To view HTML report: ./scripts/dev/go.sh tool cover -html=coverage.out"

# Coverage with per-package threshold enforcement
test-cover-strict: test-cover
	./scripts/dev/check-coverage.sh coverage.out

# Run integration tests (requires build tag)
test-integration:
	$(GO) test -tags integration -v ./internal/...

# Smoke test: build + vet + supervisor integration + golden cost extraction
# Fast end-to-end sanity check (~30s). Use before push or after major changes.
smoke:
	@echo "=== smoke: build ==="
	$(GO) build ./...
	@echo "=== smoke: vet ==="
	$(GO) vet ./...
	@echo "=== smoke: supervisor integration ==="
	$(GO) test -race -count=1 -timeout 60s -run "FullCycleLifecycle|BudgetTermination|DecisionOutcomeRecorded|ReflexionLoop" ./internal/session/...
	@echo "=== smoke: cost extraction golden tests ==="
	$(GO) test -race -count=1 -timeout 30s -run "Golden" ./internal/session/...
	@echo "=== smoke: event bus concurrency ==="
	$(GO) test -race -count=3 -timeout 30s -run "ConcurrentPublish" ./internal/events/...
	@echo "=== smoke: all passed ==="

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

# Benchmark dashboard: parse results and emit markdown summary with regression check
bench-dashboard: bench
	@bash scripts/bench-dashboard.sh bench-new.txt

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

# Generate man pages
man:
	$(GO) run ./tools/gendoc/

# Generate docs/SKILLS.md from MCP tool registrations
skill-doc:
	$(GO) run ./tools/genskilldoc -output docs/SKILLS.md

# Generate provider-native checked-in skill surfaces from the live MCP contract
skill-surface:
	$(GO) run ./tools/genskillsurface

check-generated-surfaces:
	bash ./scripts/dev/check-generated-surfaces.sh

# Install man pages to system directory
install-man: man
	@MANDIR=/usr/local/share/man/man1; \
	if [ ! -d "$$MANDIR" ]; then \
		sudo mkdir -p "$$MANDIR"; \
	fi; \
	sudo cp man/man1/*.1 "$$MANDIR/"; \
	echo "Man pages installed to $$MANDIR/"

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

# Generate coverage badge SVG (runs tests if profile missing)
coverage-badge:
	@mkdir -p .ralph
	@if [ ! -f .ralph/coverage.out ]; then \
		echo "Running tests to generate coverage profile..."; \
		$(GO) test -race -coverprofile=.ralph/coverage.out ./...; \
	fi
	$(GO) run ./tools/covbadge -i .ralph/coverage.out -o .ralph/coverage-badge.svg

# Generate HTML coverage report + per-package summary
coverage-report:
	@mkdir -p .ralph
	@if [ ! -f .ralph/coverage.out ]; then \
		echo "Running tests to generate coverage profile..."; \
		$(GO) test -race -coverprofile=.ralph/coverage.out ./...; \
	fi
	$(GO) tool cover -html=.ralph/coverage.out -o .ralph/coverage.html
	@echo "HTML report: .ralph/coverage.html"
	@echo ""
	@echo "=== Per-package coverage summary (sorted by %) ==="
	@$(GO) tool cover -func=.ralph/coverage.out | \
		grep -v 'total:' | \
		awk '{pkg=$$1; sub(/:[^:]+$$/, "", pkg); pct=$$NF+0; stmts[pkg]++; if(pct>0) hit[pkg]++} END{for(p in stmts){pct=hit[p]/stmts[p]*100; printf "%6.1f  %s\n", pct, p}}' | \
		sort -rn | \
		awk '{pct=$$1+0; warn=""; if (pct < 60) warn=" \033[31m[NEEDS ATTENTION]\033[0m"; printf "  %-70s %5.1f%%%s\n", $$2, pct, warn}'
	@echo ""
	@$(GO) tool cover -func=.ralph/coverage.out | tail -1

# Generate coverage treemap (text-based fallback if go-cover-treemap unavailable)
coverage-treemap:
	@mkdir -p .ralph
	@if [ ! -f .ralph/coverage.out ]; then \
		echo "Running tests to generate coverage profile..."; \
		$(GO) test -race -coverprofile=.ralph/coverage.out ./...; \
	fi
	@if command -v go-cover-treemap >/dev/null 2>&1; then \
		echo "Generating SVG treemap with go-cover-treemap..."; \
		go-cover-treemap -coverprofile .ralph/coverage.out > .ralph/coverage-treemap.svg; \
		echo "Treemap: .ralph/coverage-treemap.svg"; \
	else \
		echo "go-cover-treemap not installed, generating text treemap..."; \
		echo ""; \
		echo "=== Coverage Treemap (text) ==="; \
		echo ""; \
		$(GO) tool cover -func=.ralph/coverage.out | \
			grep -v 'total:' | \
			awk '{pkg=$$1; sub(/:[^:]+$$/, "", pkg); pct=$$NF+0; sum[pkg]+=pct; cnt[pkg]++} END{for(p in sum){avg=sum[p]/cnt[p]; printf "%6.1f  %s\n", avg, p}}' | \
			sort -rn | \
			awk '{pct=$$1+0; bar=""; for(i=0;i<pct/2;i++) bar=bar "#"; \
				if(pct>=80) color="\033[32m"; else if(pct>=60) color="\033[33m"; else color="\033[31m"; \
				printf "  %s%-50s %5.1f%% %s\033[0m\n", color, $$2, pct, bar}'; \
		echo ""; \
		echo "Install go-cover-treemap for an SVG visualization:"; \
		echo "  go install github.com/nikolaydubina/go-cover-treemap@latest"; \
	fi

# CI pipeline: bootstrap-aware vet + test + build
ci:
	./scripts/dev/ci.sh

# Remove build artifacts
clean:
	rm -f coverage.out
	rm -f ralphglasses
	rm -f prompt-improver
	rm -f .ralph/coverage.out .ralph/coverage.html .ralph/coverage-badge.svg .ralph/coverage-treemap.svg .ralph/test-cover-output.txt
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

# Build Docker image
docker:
	docker build -t ralphglasses:latest .

# Run ralphglasses in Docker (mounts current directory as /workspace)
docker-run:
	docker run --rm -it -v $(PWD):/workspace ralphglasses:latest --scan-path /workspace

# Regenerate tool_counts_generated.go from the live registry
update-tool-counts:
	$(GO) generate ./internal/mcpserver/...
	@echo "tool_counts_generated.go updated"

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

PIPELINE_GO := ./scripts/dev/go.sh
-include $(HOME)/hairglasses-studio/dotfiles/make/pipeline.mk
