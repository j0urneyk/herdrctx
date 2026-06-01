GO ?= go
GOCMD := $(GO)
BINARY := herdrctx

.PHONY: fmt test vet lint build snapshot clean

fmt:
	golangci-lint fmt

test:
	$(GOCMD) test ./...

vet:
	$(GOCMD) vet ./...

lint:
	golangci-lint run

build:
	$(GOCMD) build -o bin/$(BINARY) ./cmd/herdrctx

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
