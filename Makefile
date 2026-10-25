.PHONY: docs docs-check build lint test tools

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

test:
	cd predictor/esb && go test ./...
	./predictor/tests/run-e2e.sh

tools:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)
