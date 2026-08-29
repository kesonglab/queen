BIN        := queen
PKG        := github.com/kesonglab/queen
VERSION    ?= dev
LDFLAGS    := -X main.version=$(VERSION)
GOFLAGS    ?=

.PHONY: all build install clean test lint dev release

all: build

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) .

install:
	go install $(GOFLAGS) -ldflags "$(LDFLAGS)" .

test:
	go test $(GOFLAGS) -race -cover ./...

lint:
	go vet ./...

dev: lint test

release:
	@test -n "$(VERSION)" || (echo "usage: make release VERSION=x.y.z" && exit 1)
	go build $(GOFLAGS) -ldflags "-X main.version=$(VERSION)" -o dist/$(BIN)-$(VERSION) .
	@echo "built dist/$(BIN)-$(VERSION)"

clean:
	rm -f $(BIN)
	rm -rf dist coverage.txt
