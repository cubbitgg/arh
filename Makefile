.PHONY: build test vet fmt lint check clean install help
.DEFAULT_GOAL := help

BINARY  := arh
CMD_DIR := ./cmd

build: ## compile the binary
	go build -o $(BINARY) $(CMD_DIR)

test: ## run all tests
	go test ./...

vet: ## run go vet
	go vet ./...

fmt: ## check formatting (fails if any file needs reformatting)
	@test -z "$$(gofmt -l .)" || (echo "gofmt: unformatted files:"; gofmt -l .; exit 1)

lint: ## run golangci-lint (requires golangci-lint in PATH)
	golangci-lint run

check: fmt vet test ## fmt + vet + test (full quality gate)

clean: ## remove the compiled binary
	rm -f $(BINARY)

install: ## install to GOPATH/bin
	go install $(CMD_DIR)

help: ## list available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'
