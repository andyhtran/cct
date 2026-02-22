default:
    @just --list --unsorted

# Run binary cct via go run
[group('dev')]
cct *args:
    go run ./cmd/cct {{args}}

# Build the binary
[group('dev')]
build:
    go build -trimpath -ldflags "-X main.version=dev" -o dist/cct ./cmd/cct

# Remove build artifacts
[group('dev')]
clean:
    rm -rf dist

# Run tests
[group('dev')]
test:
    go test ./...

# Run tests with coverage
[group('dev')]
cover:
    go test ./... -coverprofile=coverage.out
    go tool cover -func=coverage.out

# Run golangci-lint
[group('dev')]
lint:
    golangci-lint run

# Format code
[group('dev')]
fmt:
    gofumpt -w .

# Check formatting (CI)
[group('dev')]
fmt-check:
    test -z "$(gofumpt -l .)"
