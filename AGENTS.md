# Repository Guidelines

## Project Structure & Module Organization
This repository is a Go + React application (`github.com/ryotarai/arca`).
- `cmd/server/`: server entrypoint and startup.
- `internal/server/`: HTTP routing and UI asset serving.
- `internal/db/`: DB config, migrations, and store wiring.
- `internal/db/sqlc/`: SQL schema/query source and generated DB access code.
- `web/`: Vite + React frontend source.
- `internal/server/ui/dist/`: built frontend artifacts served by the backend.

Keep backend changes within `internal/` boundaries and prefer focused packages.

## API Conventions
- All API endpoints should be defined in `proto/` and served via ConnectRPC handlers.
- Avoid adding new REST-style `/api/*` routes; migrate existing APIs to Connect when touching them.

## Setup, Build, and Development Commands
Setup prerequisites: Go `1.22`, Node.js/npm, and `sqlc`.
- `go mod download`: install Go dependencies.
- `npm --prefix web ci`: install frontend dependencies.
- `make build`: build frontend and server.
- `make build-frontend`: generate proto code and build web assets.
- `make build-server`: run frontend build + sqlc, then build `./cmd/server` to `bin/server`.
- `make proto`: regenerate protobuf/Connect generated code via `buf generate`.
- `make sqlc`: regenerate SQL access code.
- `make run`: build server and run `./bin/server`.
- `make watch`: start hot-reload development server using `air`.
- `./bin/server`: run the already built server binary.

## Coding Style & Naming Conventions
Use Go 1.22 idioms and keep code `gofmt`-clean.
- Format: `gofmt` or `go fmt ./...`
- Static checks: `go vet ./...`
- Naming: exported `CamelCase`, unexported `camelCase`, short lowercase package names.
- Pass `context.Context` through request and I/O paths.

## Frontend UI Guidelines
- For UI changes, prefer `shadcn/ui` components with Tailwind CSS utilities.
- Add components via CLI (for example, `npx shadcn@latest add button`) instead of hand-copying component source.
- Keep a modern, clean developer-tool visual language (similar to Linear or ChatGPT) and preserve responsiveness on desktop/mobile.
- Keep all UI copy in English. Do not introduce Japanese (or mixed-language) strings in user-facing text.
- Keep design tokens and global styles in `web/src/index.css`; do not hardcode one-off inline styles when reusable utility classes or tokens are appropriate.
- Unless the user explicitly requests otherwise, follow the current login page design direction as the default visual baseline.
- Baseline style: dark neutral base, subtle gradient/grid background texture, restrained accent colors, high-contrast typography, soft borders, and glass-like card surfaces.
- Reuse existing spacing, radius, and density patterns from the login page to keep visual consistency across new screens.
- Prefer composition with existing `shadcn/ui` primitives and shared utility classes; avoid introducing a competing visual language per page.
- Design for early feedback: validate user input as early as possible (for example, verify credentials or passwords at the step where they are entered, not after a multi-step wizard completes). Avoid deferring error reporting to the end of a flow when the error is detectable earlier.

## Testing Guidelines
- Run tests with `make test` before pushing.
- Keep tests near code as `*_test.go`; prefer table-driven tests for handlers and DB logic.
- Add/update E2E or browser checks for UI/routing behavior; prefer `chrome-headless-shell` for browser verification.
- During debugging or investigation, proactively use the `sqlite3` command to inspect database data, verify state, and isolate issues quickly.
- When debugging machines or runtimes, prefer SSH access to the target machine and collect logs/runtime state directly; discover IP with `sudo virsh list` and `sudo virsh domifaddr arca-machine-xxx`, then connect via `ssh arcauser@IPADDR`.
- If tests fail, automatically attempt a fix and re-run tests without waiting for an extra user prompt.
- When a failing test indicates a product bug, fix implementation first; only adjust tests when the expected behavior is incorrect or the test is flaky.

## Generated Files & Regeneration
Do not manually edit generated outputs:
- `internal/db/sqlc/postgresql/*.go`
- `internal/db/sqlc/sqlite/*.go`

Regenerate from sources instead:
- SQL: edit `internal/db/sqlc/schema.sql` and `internal/db/sqlc/query.sql`, then run `make sqlc`.
- UI: edit `web/src/*`, then run `make build-frontend`.
- `internal/server/ui/dist/*` is build output and should not be committed.

## Commit & Pull Request Guidelines
Recent commits use concise, imperative subjects (for example, `Add ...`, `docs: ...`).
- Keep subjects specific and action-oriented.
- Split unrelated changes into separate commits.
- In PRs, include purpose, key changes, test results (`make test`), linked issues, and screenshots for UI changes.
- Confirm regenerated artifacts and docs updates when behavior or operations change.

## Agent Workflow
- Prefer root-cause fixes over workaround patches. Do not introduce server-side or temporary fallback behavior as a quick fix when the issue is in another layer unless the user explicitly asks for that tradeoff.
- For state transitions and destructive operations (for example delete/teardown), prefer idempotent, reconcile-driven designs: persist intent first (desired state), let workers/reconcile loops converge actual state, and make each step retry-safe so progress survives process crashes or restarts.
- Before creating a commit, run relevant verification for the changed scope (at minimum build/run checks for runtime changes, and tests when applicable).
- After completing a clear, self-contained requested change, **always** create a commit proactively without requiring an extra user prompt. Do not forget to commit.
- Keep each proactive commit focused to the completed task only; do not include unrelated or generated local artifacts (for example `*.db`).
- Use concise, imperative commit subjects that describe the delivered outcome.
- If the user explicitly asks not to commit, skip this workflow.
- If environment setup is required to complete requested work and the setup is non-destructive (for example installing missing runtime or browser dependencies), proceed without asking for additional confirmation.
- When asked to execute multiple defined tasks, continue autonomously until all tasks are completed.
- When idle, proactively run `$do-tasks` against `tmp/tasks.md`; continue autonomously for all doable items and ask only one consolidated question set for remaining blockers.
- If a blocker cannot be resolved in a reasonable time, stop and ask the user a focused question before proceeding.

## Product-to-Tasks Flow
- Read `tmp/product.md`.
- Convert requirements into actionable tasks in `tmp/tasks/NNN-*.md`.
- Split tasks into small, implementable units; avoid oversized, multi-epic tasks.
- Capture cross-task ordering in `tmp/tasks/000-dependencies.md` (DAG + parallelizable units).
- Include goal, scope, task list, and open questions.
- Ask the user to resolve open questions before starting implementation.
- Do not commit files under `tmp/`.
