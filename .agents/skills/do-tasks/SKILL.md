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
5. Ensure `.worktrees/` is gitignored.
- Add `.worktrees/` to `.gitignore` when missing.
6. For each `ready` task, create an isolated worktree run unit.
- Generate branch name: `task/<id>-<short-kebab-summary>`.
- Check out branch into `.worktrees/<branch-name>` using `git worktree add`.
- After creating the worktree, create a `tmp` symlink in that worktree that points to the repository root `tmp/` (for example: `ln -s "$(pwd)/tmp" ".worktrees/<branch-name>/tmp"` from repo root).
- If `.worktrees/<branch-name>/tmp` already exists and is not the expected symlink, stop and fix it before starting execution.
- Derive pane title from task meaning, not only id.
- Use about 20 characters that summarize the task title/content (for example `auth-fix`, `ui-login`, `db-migrate`).
- Avoid id-only titles such as `task-001` unless no meaningful summary can be derived.
- Start execution with `./scripts/bgcodex.sh "meaningful-20char-title" "your prompt" "path to the worktree dir"`.
7. Schedule run units with **parallel-by-default** policy.
- Launch all `ready` tasks immediately unless a hard dependency or high-confidence file conflict exists.
- If 2+ tasks are `ready`, keep at least 2 concurrent run units active whenever possible.
- Do not serialize preemptively for caution; serialize only when dependency/conflict evidence is explicit.
- When conflict risk is uncertain, start tasks in parallel and resolve collisions during merge.
8. Refill parallel capacity continuously.
- Each time a run unit completes, recompute DAG and launch newly `ready` tasks immediately.
- Keep the worker pool saturated until no executable tasks remain.
9. Monitor running sessions and capture logs when needed.
- Use tmux capture for active panes, for example: `tmux capture-pane -p -S - -t :codex-agents.0`.
10. On confirmed completion of each task run unit:
- Verify task-scoped checks/tests completed in that worktree.
- Merge branch into `main`.
- Delete branch and remove `.worktrees/<branch-name>`.
- Move completed task file from `tmp/tasks/` to `tmp/tasks-done/`.
11. Recompute DAG state after every completion and continue until no `ready` tasks remain.
12. Ask one consolidated question set for unresolved blockers only after all executable work is exhausted.

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
- Require worktree-local scope and disallow unrelated edits.
- Require explicit report of verification commands and outcomes before marking done.

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
