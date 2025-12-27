.PHONY: build install test vet lint clean all

BINARY := mp
INSTALL_PATH := $(HOME)/.local/bin

all: vet test build

build:
	go build -o $(BINARY) .

install: build
	mkdir -p $(INSTALL_PATH)
	cp $(BINARY) $(INSTALL_PATH)/$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run

clean:
	rm -f $(BINARY)
