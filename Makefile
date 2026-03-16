.PHONY: test test-verbose test-cover build vet lint ci clean

# Run all tests with race detector
test:
	go test -race ./...

# Verbose test output
test-verbose:
	go test -race -v ./...

# Generate coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

# Build all binaries
build:
	go build ./...

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
