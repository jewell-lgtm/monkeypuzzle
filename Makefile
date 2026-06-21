.PHONY: build install test test-tmux vet lint clean all

BINARY := mp
INSTALL_PATH := $(HOME)/.local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/jewell-lgtm/monkeypuzzle/cmd/mp.version=$(VERSION)

all: vet test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	mkdir -p $(INSTALL_PATH)
	cp $(BINARY) $(INSTALL_PATH)/$(BINARY)

test:
	go test ./...

# Tests for the companion tmux plugin (contrib/tmux). Pure bash + jq + fzf;
# integration cases skip cleanly when a dependency is missing.
test-tmux:
	bash contrib/tmux/test/run.sh

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)
