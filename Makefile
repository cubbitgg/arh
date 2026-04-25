.PHONY: build test vet fmt lint install-lint install-ctrf test-report check clean install help
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

install-lint: ## install golangci-lint if not already present
	@which golangci-lint > /dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint: install-lint ## run golangci-lint
	golangci-lint run

install-ctrf: ## install go-ctrf-json-reporter if not already present
	@which go-ctrf-json-reporter > /dev/null 2>&1 || go install github.com/ctrf-io/go-ctrf-json-reporter/cmd/go-ctrf-json-reporter@latest

test-report: install-ctrf ## run tests and generate CTRF report (ctrf-report.json)
	go test -json ./... > test-output.json
	go-ctrf-json-reporter -output ctrf-report.json < test-output.json

check: fmt vet test ## fmt + vet + test (full quality gate)

clean: ## remove the compiled binary
	rm -f $(BINARY)

install: ## install to GOPATH/bin
	go install $(CMD_DIR)

help: ## list available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*## "}; {printf "  %-14s %s\n", $$1, $$2}'
