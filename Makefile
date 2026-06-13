GO ?= go
# The race detector needs cgo, but mise.toml pins CGO_ENABLED=0 (static release
# builds) and the mise go shim re-injects the pin over any ambient env var — so
# test-race resolves the real toolchain binary and bypasses the shim.
GO_REAL ?= $(shell mise which go 2>/dev/null || command -v go)

.PHONY: test test-race cover cover-func cover-html test-web check

test: ## run Go unit tests
	$(GO) test ./... -count=1

test-race: ## run Go unit tests under the race detector (the concurrency regression tests need this)
	env CGO_ENABLED=1 $(GO_REAL) test -race -count=1 ./cmd/... ./internal/... ./web/...

cover: ## run tests with coverage and append a snapshot to coverage/history.csv
	./scripts/coverage.sh

cover-func: ## per-function coverage from the last `make cover` run
	$(GO) tool cover -func=coverage/coverage.out

cover-html: ## open per-line HTML report from the last `make cover` run
	$(GO) tool cover -html=coverage/coverage.out

test-web: ## frontend unit tests (node strip-types)
	cd web && pnpm run test:unit

check: test ## alias kept for muscle memory
