BIN_DIR    := bin
SERVER_BIN := $(BIN_DIR)/sql-api
CLI_BIN    := $(BIN_DIR)/sql-cli
GOBIN_DIR  := $(shell go env GOPATH)/bin

# Detect OS — append .exe on Windows
ifeq ($(OS),Windows_NT)
	SERVER_BIN := $(SERVER_BIN).exe
	CLI_BIN    := $(CLI_BIN).exe
endif

.PHONY: all build build-server build-cli run dev clean vet

all: build

## build: compile binaries, install to $GOPATH/bin, and add to PATH in ~/.bashrc
build: build-server build-cli
	@mkdir -p $(GOBIN_DIR)
	cp $(SERVER_BIN) $(GOBIN_DIR)/sql-api
	cp $(CLI_BIN)    $(GOBIN_DIR)/sql-cli
	@grep -qxF 'export PATH="$$PATH:$$HOME/go/bin"' ~/.bashrc \
		|| echo 'export PATH="$$PATH:$$HOME/go/bin"' >> ~/.bashrc
	@echo "Installed: sql-api and sql-cli -> $(GOBIN_DIR)"
	@echo "Run 'source ~/.bashrc' or open a new terminal to use them globally."

## build-server: compile only the HTTP server binary
build-server:
	@mkdir -p $(BIN_DIR)
	go build -o $(SERVER_BIN) ./cmd/server

## build-cli: compile only the CLI binary
build-cli:
	@mkdir -p $(BIN_DIR)
	go build -o $(CLI_BIN) ./cmd/cli

## run: build then start the HTTP server
run: build-server
	$(SERVER_BIN)

## dev: run the server from source (no build step)
dev:
	go run ./cmd/server

## clean: remove the bin/ directory
clean:
	rm -rf $(BIN_DIR)

## vet: run go vet on all packages
vet:
	go vet ./...

## help: list available targets
help:
	@grep -E '^##' Makefile | sed 's/## //'
