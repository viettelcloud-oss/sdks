# Thin wrapper over pipeline/main.py. Configure everything in pipeline.yaml;
# run `make help` for targets.
#
# Environment overrides:
#   SPEC_BASE_URL=<url>     base URL for the backend (or set in .env)
#   SPEC_URL=<url>          fetch the spec from a different backend
#   SERVICES="a b"          build only these services (default: all)
#   FETCH_SPEC=1            fetch the spec instead of reusing openapi.json

.PHONY: help init spec generate all build vet tidy clean clean-all \
        tools fmt fmt-check fix fix-check lint test verify spec-version

PY := python3 pipeline/main.py

GOBIN  := $(shell go env GOPATH)/bin
MODULE := $(shell go list -m)
ifeq ($(strip $(MODULE)),)
$(error cannot determine module path: is the Go toolchain on PATH?)
endif

# tools/ is a second, separate module so the ~200 dependencies of
# golangci-lint, gofumpt and goimports stay out of this module's go.mod and
# never become a transitive requirement of the SDK. The directory is still
# part of the repository, but `go get` downloads only the go/ module, so
# consumers never build it.
TOOLS_DIR := ../tools

# The hand-maintained sources: the two real packages, plus each service's
# doc.go, which the facade generator deliberately never writes
# (pipeline/gen_facade.py). Everything else here is generated and is already
# gofmt'ed by pipeline/main.py, so the stricter gofumpt/goimports pass would
# only fight the generator.
SOURCES := core internal $(wildcard */doc.go)

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-12s\033[0m %s\n", $$1, $$2}'

init: ## Initialize go.mod from pipeline.yaml
	@$(PY) init

spec: ## Fetch (FETCH_SPEC=1) and normalize the spec
	@$(PY) spec $(if $(filter 1,$(FETCH_SPEC)),--fetch)

generate: ## Re-normalize local raw specs, then generate code (SERVICES="..." to narrow)
	@$(PY) generate $(SERVICES)

all: ## Full pipeline: init + spec + generate, then tidy/build/vet
	@$(PY) all $(if $(filter 1,$(FETCH_SPEC)),--fetch)
	@$(MAKE) tidy build vet

build: ## Compile every package in the module
	@go build ./...

vet: ## Run go vet on every package in the module
	@go vet ./...

tidy: ## Synchronize dependencies
	@go mod tidy

clean: ## Remove generated client code and facades
	@$(PY) clean

clean-all: clean ## Also remove the fetched raw spec
	@rm -f openapi.json

# --- Checks ---------------------------------------------------------------
# `verify` is exactly what .jenkins/check_merge_request.yml runs, so a green
# local run means a green pipeline.

tools: ## Install the pinned developer tools
	cd $(TOOLS_DIR) && GOBIN="$(GOBIN)" go install tool

fmt: $(GOBIN)/gofumpt $(GOBIN)/goimports ## Format the hand-maintained packages
	@gofmt -s -w $(SOURCES)
	@$(GOBIN)/gofumpt -l -w $(SOURCES)
	@$(GOBIN)/goimports -w -local $(MODULE) $(SOURCES)

fmt-check: $(GOBIN)/gofumpt $(GOBIN)/goimports ## Fail if anything `make fmt` would rewrite is unformatted
	@out="$$( gofmt -l . ; \
	          gofmt -s -l $(SOURCES) ; \
	          $(GOBIN)/gofumpt -l $(SOURCES) ; \
	          $(GOBIN)/goimports -l -local $(MODULE) $(SOURCES) )"; \
	if [ -n "$$out" ]; then \
		echo "These files need formatting (run 'make fmt'):"; \
		echo "$$out" | sort -u; \
		exit 1; \
	fi

# go fix has no dry-run flag, unlike gofmt's -l, so fix-check runs it in place
# and diffs against git. It requires core/ and internal/ to already be clean
# so the diff it finds afterward is attributable to go fix alone.
fix: ## Rewrite hand-maintained packages using deprecated APIs (go fix)
	@go fix ./core/... ./internal/...

fix-check: ## Fail if `go fix` would rewrite core/ or internal/
	@if ! git diff --quiet -- core internal || ! git diff --cached --quiet -- core internal; then \
		echo "fix-check needs core/ and internal/ committed first (it runs 'go fix' in place and diffs the result)"; \
		exit 1; \
	fi
	@go fix ./core/... ./internal/...
	@if ! git diff --quiet -- core internal; then \
		echo "go fix would rewrite the following (run 'make fix' and commit the result):"; \
		git diff --stat -- core internal; \
		git checkout -- core internal; \
		exit 1; \
	fi

lint: $(GOBIN)/golangci-lint ## Run golangci-lint over the module
	@$(GOBIN)/golangci-lint run ./...

test: ## Run the unit tests with the race detector
	@go test -race -cover ./...

verify: fmt-check fix-check vet lint test build ## Everything CI runs on a merge request

# --- Spec version ---------------------------------------------------------

spec-version: ## Compare the upstream info.version with core.SpecVersion
	@python3 -c "import glob, json; \
	print('upstream info.version:', \
	      sorted({json.load(open(f))['info']['version'] \
	              for f in glob.glob('openapi*.json')}) or '(no spec fetched)')"
	@grep -E '^const (Spec)?Version' core/version.go

# Install on first use so `make lint` works in a fresh checkout.
$(GOBIN)/golangci-lint $(GOBIN)/gofumpt $(GOBIN)/goimports:
	@$(MAKE) tools
