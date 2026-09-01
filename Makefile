.PHONY: test build build-ceph lint ci
GO ?= go
LDFLAGS ?= -s -w

test:
	CGO_ENABLED=0 $(GO) test -race -count=1 -timeout 120s ./...

build:
	CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o bin/storage-ceph ./cmd/module

build-ceph:
	CGO_ENABLED=1 $(GO) build -tags ceph -ldflags="$(LDFLAGS)" -o bin/storage-ceph-ceph ./cmd/module

lint:
	golangci-lint run --timeout 120s ./...

ci: lint test build
