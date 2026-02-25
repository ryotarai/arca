GO ?= go
NPM ?= npm
SQLC ?= sqlc
WEB_DIR ?= web
BIN_DIR ?= bin
SERVER_BIN ?= $(BIN_DIR)/server
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod

.PHONY: build build-frontend build-server sqlc run watch
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

build-server: sqlc
	mkdir -p $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(SERVER_BIN) ./cmd/server

sqlc:
	@if command -v $(SQLC) >/dev/null 2>&1; then \
		$(SQLC) generate; \
	else \
		$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate; \
	fi

run: build-server
	./$(SERVER_BIN)

watch:
	@if command -v air >/dev/null 2>&1; then \
		air -c .air.toml; \
	else \
		$(GO) run github.com/air-verse/air@v1.61.7 -c .air.toml; \
	fi
