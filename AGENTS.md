# Repository Guidelines

## Project Structure & Module Organization
This repository is a Go + React application (`github.com/ryotarai/hayai`).
- `cmd/server/`: server entrypoint and startup.
- `internal/server/`: HTTP routing and UI asset serving.
- `internal/db/`: DB config, migrations, and store wiring.
- `internal/db/sqlc/`: SQL schema/query source and generated DB access code.
- `web/`: Vite + React frontend source.
- `internal/server/ui/dist/`: built frontend artifacts served by the backend.

Keep backend changes within `internal/` boundaries and prefer focused packages.

## Setup, Build, and Development Commands
Setup prerequisites: Go `1.22`, Node.js/npm, and `sqlc`.
- `go mod download`: install Go dependencies.
- `npm --prefix web ci`: install frontend dependencies.
- `make build`: build frontend and server.
- `make build-frontend`: build only web assets.
- `make build-server`: build server (runs `make sqlc` internally).
- `make sqlc`: regenerate SQL access code.
- `./bin/server`: run the built server.

## Coding Style & Naming Conventions
Use Go 1.22 idioms and keep code `gofmt`-clean.
- Format: `gofmt` or `go fmt ./...`
- Static checks: `go vet ./...`
- Naming: exported `CamelCase`, unexported `camelCase`, short lowercase package names.
- Pass `context.Context` through request and I/O paths.

## Testing Guidelines
- Run tests with `go test ./...` before pushing.
- Keep tests near code as `*_test.go`; prefer table-driven tests for handlers and DB logic.
- Add/update E2E or browser checks for UI/routing behavior; prefer `chrome-headless-shell` for browser verification.

## Generated Files & Regeneration
Do not manually edit generated outputs:
- `internal/db/sqlc/postgresql/*.go`
- `internal/db/sqlc/sqlite/*.go`
- `internal/server/ui/dist/*`

Regenerate from sources instead:
- SQL: edit `internal/db/sqlc/schema.sql` and `internal/db/sqlc/query.sql`, then run `make sqlc`.
- UI: edit `web/src/*`, then run `make build-frontend`.

## Commit & Pull Request Guidelines
Recent commits use concise, imperative subjects (for example, `Add ...`, `docs: ...`).
- Keep subjects specific and action-oriented.
- Split unrelated changes into separate commits.
- In PRs, include purpose, key changes, test results (`go test ./...`), linked issues, and screenshots for UI changes.
- Confirm regenerated artifacts and docs updates when behavior or operations change.

## Agent Workflow
- After completing a clear, self-contained requested change, create a commit proactively without requiring an extra user prompt.
- Keep each proactive commit focused to the completed task only; do not include unrelated or generated local artifacts (for example `*.db`).
- Use concise, imperative commit subjects that describe the delivered outcome.
- If the user explicitly asks not to commit, skip this workflow.
- If environment setup is required to complete requested work and the setup is non-destructive (for example installing missing runtime or browser dependencies), proceed without asking for additional confirmation.
