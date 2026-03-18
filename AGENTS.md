# Repository Guidelines

> **This is an open-source project.** Do not add company-specific information (internal URLs, project IDs, credentials, deploy scripts, etc.) to this file or any tracked file. Keep such details in `AGENTS.local.md` or other `*.local.md` files, which are git-ignored.

**First**, read and apply the instructions in `AGENTS.local.md` (if it exists) before using any other tools or reading any other files.

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
- `make test`: run Go tests and fast E2E tests.
- `make test-all`: run all tests including slow E2E tests.
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
- Always consider UX from the end-user's perspective. Design flows that are intuitive and user-friendly; avoid patterns that frustrate or confuse users.

## Testing Guidelines
- Run tests with `make test` before pushing.
- For full test coverage including slow E2E tests, use `make test-all` or `make web/test-slow`.
- Keep tests near code as `*_test.go`; prefer table-driven tests for handlers and DB logic.
- Add/update E2E or browser checks for UI/routing behavior; prefer `chrome-headless-shell` for browser verification.
- During debugging or investigation, proactively use the `sqlite3` command to inspect database data, verify state, and isolate issues quickly.
- When debugging machines or runtimes, prefer SSH access to the target machine and collect logs/runtime state directly; discover IP with `sudo virsh list` and `sudo virsh domifaddr arca-machine-xxx`, then connect via `ssh arcauser@IPADDR`.
- If tests fail, automatically attempt a fix and re-run tests without waiting for an extra user prompt.
- When a failing test indicates a product bug, fix implementation first; only adjust tests when the expected behavior is incorrect or the test is flaky.
- When fixing bugs or adding logic with non-obvious edge cases (for example encoding mismatches, boundary conditions, or parsing variations), proactively add unit tests covering those cases without being asked.
- When changing implementation (API endpoints, backend logic, or UI behavior), proactively add or update E2E tests in `web/e2e/` to cover the changed behavior.
- After adding or updating E2E tests, always run them proactively without waiting for the user to ask. Run fast tests with `cd web && npx playwright test --project=fast <spec-file>`. When changes affect machine provisioning or LXD, also run `cd web && npx playwright test --project=slow e2e/lxd-provisioning.spec.ts`.

## Manual Machine Testing via API

When testing machine provisioning or runtime changes end-to-end, use the static API token to drive operations via curl instead of the browser UI.

### Setup
Start the server with `ARCA_API_TOKEN` set:
```bash
ARCA_API_TOKEN="dev-token-12345" make run
```

### API operations
All ConnectRPC endpoints accept `Authorization: Bearer <token>` header.

```bash
TOKEN="dev-token-12345"

# List machines
curl -s -X POST http://localhost:8080/arca.v1.MachineService/ListMachines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{}'

# Create a machine (runtime_id from runtimes table)
curl -s -X POST http://localhost:8080/arca.v1.MachineService/CreateMachine \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"test1","runtime_id":"RUNTIME_ID_HERE"}'

# Get machine status
curl -s -X POST http://localhost:8080/arca.v1.MachineService/GetMachine \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"machine_id":"MACHINE_ID_HERE"}'

# Delete a machine
curl -s -X POST http://localhost:8080/arca.v1.MachineService/DeleteMachine \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"machine_id":"MACHINE_ID_HERE"}'
```

### Monitoring a machine
```bash
# Check LXD containers
sudo lxc list

# View cloud-init progress
sudo lxc exec arca-machine-XXXX -- tail -20 /var/log/cloud-init-output.log

# View bootstrap service logs (arcad download, service startup)
sudo lxc exec arca-machine-XXXX -- journalctl -u arca-bootstrap.service --no-pager

# View arcad daemon logs
sudo lxc exec arca-machine-XXXX -- journalctl -u arca-arcad.service --no-pager

# Check arcad env vars (control plane URL, machine token, etc.)
sudo lxc exec arca-machine-XXXX -- cat /etc/arca/arcad.env

# Inspect DB state directly
sqlite3 arca.db "SELECT id, name, status, desired_status FROM machines m JOIN machine_states ms ON ms.machine_id = m.id;"
```

### LXD networking
LXD containers access the host server via the bridge IP `10.200.0.1`. The runtime config `serverApiUrl` must be set to `http://10.200.0.1:8080` for containers to reach the arca server (set via the runtimes DB table or the UI runtime settings page).

## Generated Files & Regeneration
Do not manually edit generated outputs:
- `internal/db/sqlc/postgresql/*.go`
- `internal/db/sqlc/sqlite/*.go`

Regenerate from sources instead:
- SQL: edit `internal/db/sqlc/schema.sql` and `internal/db/sqlc/query.sql`, then run `make sqlc`.
- UI: edit `web/src/*`, then run `make build-frontend`.
- `internal/server/ui/dist/*` is build output and should not be committed.

## Branch Strategy
- The `main` branch is protected — direct pushes are not allowed.
- Always create a feature branch and open a pull request to merge changes into `main`.

## Commit & Pull Request Guidelines
Recent commits use concise, imperative subjects (for example, `Add ...`, `docs: ...`).
- Keep subjects specific and action-oriented.
- Split unrelated changes into separate commits.
- In PRs, include purpose, key changes, test results (`make test`), linked issues, and screenshots for UI changes.
- Confirm regenerated artifacts and docs updates when behavior or operations change.

## Machine Provisioning Design Principles
- **All setup steps must be idempotent**: every provisioning step arcad executes must be safe to re-run. Steps check current state and skip if already satisfied. This guarantees correctness regardless of whether the machine started from a bare OS image, a platform image, or a user custom image.
- **Backward compatibility with older arcad**: when changing arca server APIs or behavior, maintain compatibility with older arcad versions. Older arcad instances must continue to function; new fields should be additive and optional.

## Agent Workflow
- Always run `git fetch origin` before referencing or operating on `origin/main` (or any remote branch) to ensure you have the latest state.
- Before creating a commit, verify the current branch (`git branch --show-current`) to ensure you are on the intended branch. If the branch has already been merged (check with `gh pr list --head <branch> --state merged`), switch to a new feature branch from `origin/main` instead of committing to the stale branch.
- Proactively update CLAUDE.md when you discover new patterns, conventions, or project-specific knowledge that would help future sessions. Keep it accurate and current as the codebase evolves.
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
- After creating a PR from a feature branch, always switch back to `main` (`git checkout main`) so subsequent work starts from the correct base.
- If a blocker cannot be resolved in a reasonable time, stop and ask the user a focused question before proceeding.

## Product-to-Tasks Flow
- Read `tmp/product.md`.
- Convert requirements into actionable tasks in `tmp/tasks/NNN-*.md`.
- Split tasks into small, implementable units; avoid oversized, multi-epic tasks.
- Capture cross-task ordering in `tmp/tasks/000-dependencies.md` (DAG + parallelizable units).
- Include goal, scope, task list, and open questions.
- Ask the user to resolve open questions before starting implementation.
- Do not commit files under `tmp/`.
