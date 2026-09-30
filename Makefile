GO ?= go

.PHONY: fmt vet test race build

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

build:
	mkdir -p bin
	$(GO) build -o bin/contextarium ./cmd/contextarium
