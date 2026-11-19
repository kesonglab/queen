BIN        := queen
PKG        := github.com/kesonglab/queen
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS    := -X main.version=$(VERSION)
GOFLAGS    ?=

.PHONY: all build install clean test lint fmt-check dev release

all: build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) .

install:
	go install $(GOFLAGS) -ldflags "$(LDFLAGS)" .

test:
	go test $(GOFLAGS) -race -cover ./...

lint:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l . 2>/dev/null | grep -v '^vendor/' || true); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

dev: fmt-check lint test

release:
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=x.y.z" && exit 1)
	@test "$(VERSION)" != "dev" || (echo "usage: make release VERSION=x.y.z" && exit 1)
	go build $(GOFLAGS) -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/$(BIN)-$(VERSION) .
	@echo "built dist/$(BIN)-$(VERSION)"

clean:
	rm -f $(BIN)
	rm -rf dist coverage.txt
