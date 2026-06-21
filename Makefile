.PHONY: build build-server install test test-tmux vet lint clean all

BIN_DIR := bin
INSTALL_PATH := $(HOME)/.local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
# version lives in package main (apps/mp), so the linker symbol is main.version.
LDFLAGS := -X main.version=$(VERSION)

all: vet test build

# Build the CLI + MCP bridge into a shared ./bin so mp-mcp's findMpBinary()
# resolves the sibling mp binary.
build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/mp ./apps/mp
	go build -o $(BIN_DIR)/mp-mcp ./apps/mp-mcp

build-server:
	go build -o $(BIN_DIR)/mp-server ./apps/mp-server

install: build
	mkdir -p $(INSTALL_PATH)
	cp $(BIN_DIR)/mp $(INSTALL_PATH)/mp
	cp $(BIN_DIR)/mp-mcp $(INSTALL_PATH)/mp-mcp

test:
	go test ./...

# Tests for the companion tmux plugin (apps/tmux). Pure bash + jq + fzf;
# integration cases skip cleanly when a dependency is missing.
test-tmux:
	bash apps/tmux/test/run.sh

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -rf $(BIN_DIR)
