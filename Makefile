.DEFAULT_GOAL := help

# Integration tests start a Postgres container via testcontainers, which does not
# read Docker's context and needs DOCKER_HOST set explicitly. Detect Colima, then
# Docker Desktop, and otherwise leave it to the environment.
COLIMA_SOCK := $(HOME)/.colima/default/docker.sock
DESKTOP_SOCK := $(HOME)/.docker/run/docker.sock

ifeq ($(origin DOCKER_HOST), undefined)
  ifneq ($(wildcard $(COLIMA_SOCK)),)
    export DOCKER_HOST := unix://$(COLIMA_SOCK)
    export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE := /var/run/docker.sock
  else ifneq ($(wildcard $(DESKTOP_SOCK)),)
    export DOCKER_HOST := unix://$(DESKTOP_SOCK)
    export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE := /var/run/docker.sock
  endif
endif

GO ?= go
GOLANGCI_LINT ?= golangci-lint
TEMPL ?= templ
SQLC ?= sqlc

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Compile every package
	$(GO) build ./...

.PHONY: test
test: ## Run all tests (starts a Postgres container; needs Docker running)
	$(GO) test ./...

# Scoped by package rather than by -short: no test calls testing.Short(), so the
# flag would promise a distinction the suite does not make. Everything needing a
# container lives under ./service/.
.PHONY: test-unit
test-unit: ## Run only the packages that need no container
	$(GO) test ./cmd/... ./jwtauth/... ./web/...

.PHONY: test-v
test-v: ## Run all tests verbosely, no cache
	$(GO) test -count=1 -v ./...

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint, applying fixes it can make safely
	$(GOLANGCI_LINT) run --fix ./...

.PHONY: fmt
fmt: ## Format Go and templ sources
	$(GO) fmt ./...
	$(TEMPL) fmt .

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: generate
generate: ## Regenerate templ components and sqlc queries
	$(TEMPL) generate
	$(SQLC) generate

.PHONY: tidy
tidy: ## Prune and verify module requirements
	$(GO) mod tidy

# What CI should run, and what to run before pushing.
.PHONY: check
check: build vet lint test ## Build, vet, lint and test

.PHONY: tools
tools: ## Install the toolchain this Makefile expects
	brew install golangci-lint sqlc
	$(GO) install github.com/a-h/templ/cmd/templ@v0.2.793
