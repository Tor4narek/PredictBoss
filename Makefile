.PHONY: docs docs-check build

docs:
	cd predictor/esb && go run ./cmd/gendocs -out ../docs/api

docs-check:
	cd predictor/esb && go run ./cmd/gendocs -out ../docs/api -check

build:
	cd predictor/esb && go build ./...
