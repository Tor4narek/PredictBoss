# ESB unit tests

These test files are stored outside `predictor/esb`. `make test-unit` supplies
a Go overlay so each file is compiled with its ESB package, including tests
that exercise package-private behavior.
