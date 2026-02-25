GO ?= go
BIN_DIR ?= bin
SERVER_BIN ?= $(BIN_DIR)/server
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod

.PHONY: build
build:
	mkdir -p $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(SERVER_BIN) ./cmd/server
