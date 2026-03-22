GO ?= go
NPM ?= npm
SQLC ?= sqlc
BUF ?= buf
WEB_DIR ?= web
BIN_DIR ?= bin
SERVER_BIN ?= $(BIN_DIR)/server
VERSION ?= dev
VERSION_PKG := github.com/ryotarai/arca/internal/version
LDFLAGS := -X $(VERSION_PKG).Version=$(VERSION)

.PHONY: build build-frontend build-server build-server-dev proto sqlc vet test/backend test/e2e test/e2e-all go/test web/test web/test-fast web/test-slow run watch
build: build-frontend build-server

build-frontend: proto
	@if [ ! -f $(WEB_DIR)/package.json ]; then \
		echo "$(WEB_DIR)/package.json not found"; \
		exit 1; \
	fi
	$(NPM) --prefix $(WEB_DIR) run build

build-server: build-frontend sqlc
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(SERVER_BIN) ./cmd/server

build-server-dev:
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(SERVER_BIN) ./cmd/server

proto:
	@if command -v $(BUF) >/dev/null 2>&1; then \
		$(BUF) generate; \
	else \
		$(GO) run github.com/bufbuild/buf/cmd/buf@v1.56.0 generate; \
	fi

sqlc:
	@if command -v $(SQLC) >/dev/null 2>&1; then \
		$(SQLC) generate; \
	else \
		$(GO) run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate; \
	fi

vet:
	$(GO) vet ./...

test/backend: vet go/test

test/e2e: web/test-fast

test/e2e-all: web/test

go/test:
	$(GO) test ./cmd/... ./internal/...

web/test:
	$(NPM) --prefix $(WEB_DIR) run e2e

web/test-fast:
	$(NPM) --prefix $(WEB_DIR) run e2e -- --project=fast

web/test-slow:
	$(NPM) --prefix $(WEB_DIR) run e2e -- --project=slow

run: build-server
	./$(SERVER_BIN)

watch:
	@if command -v air >/dev/null 2>&1; then \
		air -c .air.toml; \
	else \
		$(GO) run github.com/air-verse/air@v1.61.7 -c .air.toml; \
	fi
