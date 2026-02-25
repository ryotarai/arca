# AGENTS.md

This file provides shared guidelines for humans and agents working in this repository.

## Project Overview
- Module: `github.com/ryotarai/hayai`
- Go version: `1.22`
- Backend: Go (`cmd/server`, `internal/...`)
- Frontend: Vite + React (`web/`)
- DB access: generated code from `sqlc`

## Repository Layout
- `cmd/server/`: server entrypoint
- `internal/server/`: routing and asset serving
- `internal/db/`: DB configuration, migration, and store
- `internal/db/sqlc/`: SQL definitions and generated code
- `web/`: frontend source
- `internal/server/ui/dist/`: built frontend artifacts

## Setup
1. Install Go `1.22`
2. Install Node.js and npm
3. Install `sqlc`
4. Install dependencies
   - Go: `go mod download`
   - Web: `npm --prefix web ci`

## Build / Generate
- Build all: `make build`
- Build frontend only: `make build-frontend`
- Build server only: `make build-server`
- Generate sqlc code: `make sqlc`

`make build-server` runs `make sqlc` internally.

## Run / Test
- Run server (built binary): `./bin/server`
- Run unit tests: `go test ./...`
- For browser-related verification, proactively use `chrome-headless-shell` when possible

## Coding Guidelines (Go)
- Formatting: `gofmt` (or `go fmt ./...` when needed)
- Static analysis: `go vet ./...`
- Prefer context-aware I/O and pass `context.Context` properly
- Respect existing boundaries under `internal/` and minimize cross-boundary changes

## Generated Files
Avoid manual edits to generated files:
- `internal/db/sqlc/postgresql/*.go`
- `internal/db/sqlc/sqlite/*.go`
- `internal/server/ui/dist/*`

If changes are needed, edit source files and regenerate:
- sqlc: edit `internal/db/sqlc/schema.sql` and `internal/db/sqlc/query.sql`, then run `make sqlc`
- UI: edit `web/src/*`, then run `make build-frontend`

## Change Checklist
- Regenerated related artifacts when required
- Verified `go test ./...` passes
- Strongly encouraged writing and running both unit tests and E2E tests
- Excluded unnecessary diffs (cache files, local settings)
- Updated docs when behavior or operation changed
- Committed changes in appropriate, incremental units
