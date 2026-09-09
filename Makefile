GO ?= go
GOCMD := $(GO)
BINARY := herdrctx
INTEGRATION_ARGS ?=

.PHONY: fmt test test-integration vet lint build snapshot clean

fmt:
	golangci-lint fmt

test:
	$(GOCMD) test ./...

test-integration: build
	$(GOCMD) test -tags=integration ./integration -count=1 -timeout=10m -v $(value INTEGRATION_ARGS)

vet:
	$(GOCMD) vet -tags=integration ./...

lint:
	golangci-lint run

build:
	$(GOCMD) build -o bin/$(BINARY) ./cmd/herdrctx

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
