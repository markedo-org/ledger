BINARY := ledger
PKG := github.com/markedo-org/ledger
VERSION ?= $(shell cat VERSION 2>/dev/null || git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE := $(shell date -u +%Y-%m-%d)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

.PHONY: build linux-amd64 dist run test lint smoke clean deploy

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/ledger

linux-amd64:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/ledger-linux-amd64 ./cmd/ledger

dist:
	./scripts/dist.sh

smoke:
	./scripts/smoke.sh

run: build
	set -a; \
	if [ -f ../.env ]; then . ../.env; fi; \
	if [ -f .env ]; then . .env; fi; \
	set +a; \
	LEDGER_BOOT_TOKEN=$${LEDGER_BOOT_TOKEN:-lgr_dev} ./$(BINARY) -listen 127.0.0.1:8080 -db ledger.sqlite

test:
	go test ./...

lint:
	go vet ./...

deploy:
	./scripts/deploy.sh

clean:
	rm -f $(BINARY) dist/ledger-linux-amd64 dist/ledger-darwin-arm64 dist/ledger-windows-amd64.exe
