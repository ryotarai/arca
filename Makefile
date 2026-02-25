GO ?= go
NPM ?= npm
WEB_DIR ?= web
BIN_DIR ?= bin
SERVER_BIN ?= $(BIN_DIR)/server
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod

.PHONY: build build-frontend build-server
build: build-frontend build-server

build-frontend:
	@if [ ! -f $(WEB_DIR)/package.json ]; then \
		echo "$(WEB_DIR)/package.json not found"; \
		exit 1; \
	fi
	@if [ -f $(WEB_DIR)/package-lock.json ]; then \
		$(NPM) --prefix $(WEB_DIR) ci; \
	else \
		$(NPM) --prefix $(WEB_DIR) install; \
	fi
	$(NPM) --prefix $(WEB_DIR) run build

build-server:
	mkdir -p $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(SERVER_BIN) ./cmd/server
