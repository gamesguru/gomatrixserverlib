# Justfile for gomatrixserverlib development

# Default recipe
default:
    @just --list

# Run golangci-lint
lint:
    golangci-lint run

# Run unit tests
test:
    go test -v ./...

# Run unit tests with race detector
test-race:
    go test -race -v ./...

# Format code
fmt:
    prettier -w .
    gofumpt -w .
    golangci-lint run --fix
    gci write --skip-vendor --skip-generated -s standard -s default -s "prefix(codefloe.com/pat-s/gomatrixserverlib)" --custom-order .

# Run both lint and tests
check: lint test

# Update dependencies
deps-update:
    go get -u ./...
    go mod tidy

# Verify dependencies
deps-verify:
    go mod verify
