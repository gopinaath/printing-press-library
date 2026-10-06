.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/oneword-domains-pp-cli$(BIN_EXT) ./cmd/oneword-domains-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/oneword-domains-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/oneword-domains-pp-mcp$(BIN_EXT) ./cmd/oneword-domains-pp-mcp

install-mcp:
	go install ./cmd/oneword-domains-pp-mcp

build-all: build build-mcp
