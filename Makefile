.PHONY: docs docs-check build lint test test-unit test-integration test-e2e test-race coverage tools

GO ?= go
GOLANGCI_LINT_VERSION := 2.8.0
ESB_GO_FILES = $(shell rg --files predictor/esb/cmd predictor/esb/internal -g '*.go' -g '!**/*_test.go')

docs:
	cd predictor/esb && go run ./cmd/gendocs -out ../docs/api

docs-check:
	cd predictor/esb && go run ./cmd/gendocs -out ../docs/api -check

build:
	cd predictor/esb && go build ./...

lint:
	@unformatted="$$(gofmt -l $(ESB_GO_FILES))"; \
	if [ -n "$$unformatted" ]; then \
	  printf 'Go files need gofmt:\n%s\n' "$$unformatted"; \
	  exit 1; \
	fi
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint $(GOLANGCI_LINT_VERSION) is missing; run make tools" >&2; exit 1; }
	@test "$$(golangci-lint version --short)" = "$(GOLANGCI_LINT_VERSION)" || { \
	  echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required; run make tools" >&2; exit 1; \
	}
	cd predictor/esb && golangci-lint run --tests=false ./...

test: test-unit test-e2e

test-unit:
	GO="$(GO)" python3 predictor/tests/run-unit.py ./...

test-integration: test-e2e

test-e2e:
	@command -v docker >/dev/null 2>&1 || { echo "Docker is required for the isolated ESB E2E suite." >&2; exit 1; }
	@docker compose version >/dev/null 2>&1 || { echo "Docker Compose v2 is required for the isolated ESB E2E suite." >&2; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "Start the Docker daemon before running the ESB E2E suite." >&2; exit 1; }
	./predictor/tests/run-e2e.sh

test-race:
	GO="$(GO)" python3 predictor/tests/run-unit.py -race ./...

coverage:
	@set -eu; profile="$$(mktemp "$${TMPDIR:-/tmp}/predictor-esb-coverage.XXXXXX")"; \
	trap 'rm -f "$$profile"' EXIT; \
	GO="$(GO)" python3 predictor/tests/run-unit.py -coverprofile="$$profile" -covermode=atomic ./...; \
	cd predictor/esb && $(GO) tool cover -func="$$profile" | awk '/^total:/ { print "Overall ESB Go test coverage: " $$3; found = 1 } END { if (!found) exit 1 }'

tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
