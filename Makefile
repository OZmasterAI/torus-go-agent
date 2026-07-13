# evolve-harness/search_db*/ holds git-ignored, generated snapshot code that
# does not compile — exclude it so ./... targets work on a fresh checkout.
PKGS = $(shell go list ./... | grep -v 'evolve-harness/search_db')

.DEFAULT_GOAL := check

.PHONY: build vet test test-fast check

build:
	go build $(PKGS)

vet:
	go vet $(PKGS)

# Canonical test path — always runs the race detector, never cached results.
test:
	go test -race -count=1 $(PKGS)

# Quick iteration only; CI and pre-commit should use "test".
test-fast:
	go test $(PKGS)

# The full gate: build + vet + race tests.
check: build vet test
