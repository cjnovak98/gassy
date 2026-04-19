.PHONY: help build build-agent build-all install test validate clean

# Default target - build and install
all: build-agent build install

help:
	@echo "Available targets:"
	@echo "  make          - (default) build + install to PATH"
	@echo "  make build    - Build Go CLI to ./gassy"
	@echo "  make build-agent - Build TypeScript agent"
	@echo "  make build-all - Build both CLI and agent (no install)"
	@echo "  make test     - Run Go unit tests"
	@echo "  make validate - Run end-to-end PoC demo test"
	@echo "  make clean    - Remove built binaries and tidy modules"

build:
	go build -o gassy ./cmd/gassy/

build-agent:
	podman build -t localhost:5000/gassy/agent:latest ./agent

build-all: build build-agent

install: build build-agent
	go install ./cmd/gassy/
	go install ./cmd/gassy-admin/

test:
	go test -v ./internal/...

validate:
	@echo "=== Gassy Validation Test ===" && \
	echo "Running PoC demo end-to-end..." && \
	cd examples/poc/demo && go run . && \
	echo "" && echo "Validation PASSED" || \
	(echo "" && echo "Validation FAILED" && exit 1)

clean:
	go clean
	go mod tidy
	rm -f agent/*.log