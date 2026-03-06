---
name: do-tasks
description: Execute task files from `tmp/tasks/` end-to-end. Use when asked to run project task markdown files, analyze dependencies, execute independent tasks in parallel, offload suitable work to isolated bgcodex sessions, and move completed task files into `tmp/tasks-done/`.
---

# Do Tasks

Execute task markdown files in `tmp/tasks/` with dependency-aware, aggressively parallel orchestration using isolated git worktrees and `./scripts/bgcodex.sh`.

## Workflow

1. List task files in `tmp/tasks/` (`NNN-*.md`).
2. Read each task file in `tmp/tasks/`.
3. Build a dependency DAG from explicit dependency lines in task files.
- If no explicit dependency exists, treat tasks as independent.
4. Classify tasks:
- `ready`: dependencies already completed.
- `blocked`: waiting for dependency or missing external input.
5. For each `ready` task, create an isolated worktree run unit.
- Generate branch name: `task/<id>-<short-kebab-summary>`.
- Check out branch into `.worktrees/<branch-name>` using `git worktree add`.
- After creating the worktree, create a `tmp` symlink in that worktree that points to the repository root `tmp/` (for example: `ln -s "$(pwd)/tmp" ".worktrees/<branch-name>/tmp"` from repo root).
- If `.worktrees/<branch-name>/tmp` already exists and is not the expected symlink, stop and fix it before starting execution.
- Derive pane title from task meaning, not only id.
- Use about 20 characters that summarize the task title/content (for example `auth-fix`, `ui-login`, `db-migrate`).
- Avoid id-only titles such as `task-001` unless no meaningful summary can be derived.
- Start execution with `./scripts/bgcodex.sh "meaningful-20char-title" "your prompt" "path to the worktree dir"`.
- If the user explicitly instructs to use Claude, set `AGENT=claude` for that run (for example `AGENT=claude ./scripts/bgcodex.sh "meaningful-20char-title" "your prompt" "path to the worktree dir"`).
6. Schedule run units with **parallel-by-default** policy.
- Launch all `ready` tasks immediately unless a hard dependency or high-confidence file conflict exists.
- If 2+ tasks are `ready`, keep at least 2 concurrent run units active whenever possible.
- Do not serialize preemptively for caution; serialize only when dependency/conflict evidence is explicit.
- When conflict risk is uncertain, start tasks in parallel and resolve collisions during merge.
7. Refill parallel capacity continuously.
- Each time a run unit completes, recompute DAG and launch newly `ready` tasks immediately.
- Keep the worker pool saturated until no executable tasks remain.
8. Monitor running sessions and capture logs when needed.
- Use tmux capture for active panes, for example: `tmux capture-pane -p -S - -t :codex-agents.0`.
9. On confirmed completion of each task run unit:
- Verify task-scoped checks/tests completed in that worktree.
- Merge branch into `main`.
- Delete branch and remove `.worktrees/<branch-name>`.
- Move completed task file from `tmp/tasks/` to `tmp/tasks-done/`.
10. Recompute DAG state after every completion and continue until no `ready` tasks remain.
11. Ask one consolidated question set for unresolved blockers only after all executable work is exhausted.

## Worktree And Merge Rules

- Keep `main` clean; do not implement task changes directly on `main`.
- For each task, use one branch and one worktree.
- Ensure each worktree has `tmp -> <repo-root>/tmp` symlink before running bgcodex.
- Merge only after completion is verified.
- If merge conflicts appear, stop concurrent execution for conflicting tasks and resolve sequentially.
- After merge, clean both the branch and its worktree directory.

## Parallel Execution Rules

- Favor maximum safe concurrency over minimal risk.
- Hard dependencies always block parallelism; soft uncertainty does not.
- Use dependency plus touched-path evidence to decide conflicts, not intuition alone.
- Typical default: run all independent tasks concurrently; fall back to serial only for known high-overlap scopes.

## Prompting Rules For bgcodex

- Include task file path and acceptance criteria in each prompt.
- When the user instructs to use Claude, run `bgcodex.sh` with `AGENT=claude`.
- Require worktree-local scope and disallow unrelated edits.
- Require explicit report of verification commands and outcomes before marking done.
- Add an execution pacing instruction: start code edits quickly (avoid prolonged restatement/search loops once relevant files are identified).

## Verification Hygiene Rules

- For repos embedding frontend assets in Go binaries (for example `internal/server/ui/dist`), rebuild frontend assets before Go test runs that compile server packages.
- After frontend code changes used by server/E2E, run `make build-frontend` before `make test` to avoid stale UI artifacts.
- If E2E failures look inconsistent with latest source, assume stale built assets first, rebuild, then rerun.

## Conflict Hotspot Rules

- Treat shared high-churn files as merge hotspots and plan for merge-time reconciliation:
- `web/e2e/login.spec.ts`
- `internal/server/machine_connect_test.go`
- When multiple tasks touch a hotspot file, prefer additive edits with clearly separated test blocks and avoid broad rewrites.
- For interface-expansion fallout in test stubs, add minimal no-op/panic methods only; do not refactor unrelated tests.

## Worktree Cleanup Rules

- If `git worktree remove` fails due permissions in local caches, restore write permission first (for example `.cache` under that worktree) and retry.
- After forced cleanup, run `git worktree prune` and then delete merged task branches.

## Completion File Rules

- Create `tmp/tasks-done/` when missing.
- Preserve filename and markdown content.
- Do not move partially completed or blocked tasks.

## Dependency Rules

- Treat `depends on`, `blocked by`, `after`, `requires`, and task-id references as hard dependencies.
- Resolve by task id (`NNN`) instead of title text where possible.
- Reject cycles by reporting the minimal cycle set and stop those tasks until clarified.

## Output Rules

- Report completed task files moved to `tmp/tasks-done/`.
- Report merged branches and removed worktree paths.
- Report remaining blocked task files in `tmp/tasks/` with concrete blockers.
- Keep final status concise and actionable.
