GO ?= go

.PHONY: build check fmt fmt-check integration snapshot test test-race tidy vet vuln

build:
	$(GO) build -trimpath -o db-tui ./cmd/db-tui

check: fmt-check vet test test-race vuln

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './dist/*')

fmt-check:
	./scripts/check-format.sh

integration:
	TEST_POSTGRES_VERSION=$${TEST_POSTGRES_VERSION:-18} $(GO) test -tags=integration -count=1 ./internal/postgres

snapshot:
	goreleaser release --snapshot --clean
	./scripts/verify-dist.sh

test:
	$(GO) test ./... -coverprofile=coverage.out

test-race:
	$(GO) test -race ./...

tidy:
	$(GO) mod tidy

vet:
	$(GO) vet ./...

vuln:
	$(GO) tool govulncheck ./...
