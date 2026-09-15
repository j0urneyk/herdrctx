GO ?= go
GOCMD := $(GO)
BINARY := herdrctx
INTEGRATION_ARGS ?=

.PHONY: fmt test test-integration test-integration-remote remote-test-images vet lint build snapshot clean

fmt:
	golangci-lint fmt

test:
	$(GOCMD) test ./...

test-integration: build
	$(GOCMD) test -tags=integration ./integration -count=1 -timeout=10m -v $(value INTEGRATION_ARGS)

remote-test-images:
	docker build --build-arg HERDR_VERSION=0.8.2 -t herdrctx-remote-test:0.8.2 integration/remote
	docker build --build-arg HERDR_VERSION=0.9.0 -t herdrctx-remote-test:0.9.0 integration/remote

test-integration-remote: build remote-test-images
	$(GOCMD) test -tags=integration ./integration -run '^TestRemote' -count=1 -timeout=10m -v -integration.remote $(value INTEGRATION_ARGS)

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
