.PHONY: build test lint install clean

BIN_EXT := $(if $(filter windows,$(shell go env GOOS)),.exe,)

build:
	go build -trimpath -o bin/philonet-pp-cli$(BIN_EXT) ./cmd/philonet-pp-cli

test:
	go test ./...

lint:
	golangci-lint run

install:
	go install ./cmd/philonet-pp-cli

clean:
	rm -rf bin/

build-mcp:
	go build -trimpath -o bin/philonet-pp-mcp$(BIN_EXT) ./cmd/philonet-pp-mcp

install-mcp:
	go install ./cmd/philonet-pp-mcp

build-all: build build-mcp
