BINARY := ledger
PKG := github.com/markedo-org/ledger
VERSION ?= $(shell cat VERSION 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%d)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build run test lint clean

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/ledger

run: build
	./$(BINARY) -listen 127.0.0.1:8080 -db ledger.sqlite

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -f $(BINARY)
