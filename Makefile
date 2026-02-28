GO ?= go
NPM ?= npm
SQLC ?= sqlc
BUF ?= buf
WEB_DIR ?= web
BIN_DIR ?= bin
SERVER_BIN ?= $(BIN_DIR)/server
MACHINE_DOCKER_IMAGE ?= arca-machine:dev
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod

.PHONY: build build-docker build-frontend build-server build-server-dev proto sqlc test go/test web/test run watch
build: build-frontend build-server build-docker

build-docker:
	docker build -t $(MACHINE_DOCKER_IMAGE) -f docker/machine/Dockerfile .

build-frontend: proto
	@if [ ! -f $(WEB_DIR)/package.json ]; then \
		echo "$(WEB_DIR)/package.json not found"; \
		exit 1; \
	fi
	$(NPM) --prefix $(WEB_DIR) run build

build-server: build-frontend sqlc
	mkdir -p $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(SERVER_BIN) ./cmd/server

build-server-dev:
	mkdir -p $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) build -o $(SERVER_BIN) ./cmd/server

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

test: go/test web/test

go/test:
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) $(GO) test ./cmd/... ./internal/...

web/test:
	$(NPM) --prefix $(WEB_DIR) run e2e

run: build-server build-docker
	MACHINE_DOCKER_IMAGE=$(MACHINE_DOCKER_IMAGE) ./$(SERVER_BIN)

watch: build-docker
	@if command -v air >/dev/null 2>&1; then \
		MACHINE_DOCKER_IMAGE=$(MACHINE_DOCKER_IMAGE) air -c .air.toml; \
	else \
		MACHINE_DOCKER_IMAGE=$(MACHINE_DOCKER_IMAGE) $(GO) run github.com/air-verse/air@v1.61.7 -c .air.toml; \
	fi
