.PHONY: build worker tui test test-fast vet check lint clean run-mcp run-worker run-tui fmt mod-tidy rebuild watch bench

MCP_BINARY := runmylife-mcp
WORKER_BINARY := runmylife-worker
TUI_BINARY := runmylife-tui
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BIN_DIR)/$(MCP_BINARY) ./cmd/runmylife-mcp

worker:
	go build $(LDFLAGS) -o $(BIN_DIR)/$(WORKER_BINARY) ./cmd/runmylife-worker

tui:
	go build $(LDFLAGS) -o $(BIN_DIR)/$(TUI_BINARY) ./cmd/runmylife-tui

test:
	go test -race ./...

test-fast:
	go test -short -race -count=1 ./internal/...

vet:
	go vet ./...

check: build worker tui vet test-fast

lint: vet
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

clean:
	rm -rf $(BIN_DIR)

run-mcp:
	go run ./cmd/runmylife-mcp

run-worker:
	go run ./cmd/runmylife-worker

run-tui:
	go run ./cmd/runmylife-tui

fmt:
	gofmt -w ./...

mod-tidy:
	go mod tidy

rebuild: build
	@echo "Built. Reconnect MCP clients (stdio mode spawns fresh process)."

watch:
	find . -name '*.go' -not -path './vendor/*' | entr -r make build

bench:
	go test -bench=. -benchmem ./internal/mcp/tools/... ./internal/resilience/...

-include $(HOME)/hairglasses-studio/dotfiles/make/pipeline.mk
