---
name: do-tasks
description: Execute task files from `tmp/tasks/` end-to-end. Use when asked to run project task markdown files, analyze dependencies, execute independent tasks in parallel, offload suitable work to isolated bgcodex sessions, and move completed task files into `tmp/tasks-done/`.
---

# Do Tasks

Execute task markdown files in `tmp/tasks/` with dependency-aware, parallel-first orchestration using isolated git worktrees and `./script/bgcodex.sh`.

## Workflow

1. List task files in `tmp/tasks/` (`NNN-*.md`).
2. Read `tmp/tasks/000-dependencies.md` if present, then read each task file.
3. Build a dependency DAG from explicit dependency lines first.
- Trust `000-dependencies.md` as the source of truth when it conflicts with inferred dependencies.
- If no explicit dependency exists, treat tasks as independent.
4. Classify tasks:
- `ready`: dependencies already completed.
- `blocked`: waiting for dependency or missing external input.
5. Ensure `.worktrees/` is gitignored.
- Add `.worktrees/` to `.gitignore` when missing.
6. For each `ready` task, create an isolated worktree run unit.
- Generate branch name: `task/<id>-<short-kebab-summary>`.
- Check out branch into `.worktrees/<branch-name>` using `git worktree add`.
- Start execution with `./script/bgcodex.sh "short pane title" "your prompt" "path to the worktree dir"`.
7. Schedule run units in parallel only when safe.
- Run concurrently only if there is no dependency edge between tasks.
- Run concurrently only if scope/file conflict risk is low.
- If dependency or conflict risk exists, run serially.
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
- Merge only after completion is verified.
- If merge conflicts appear, stop concurrent execution for conflicting tasks and resolve sequentially.
- After merge, clean both the branch and its worktree directory.

## Prompting Rules For bgcodex

- Include task file path and acceptance criteria in each prompt.
- Require worktree-local scope and disallow unrelated edits.
- Require explicit report of verification commands and outcomes before marking done.

## Completion File Rules

- Create `tmp/tasks-done/` when missing.
- Preserve filename and markdown content.
- Never move `tmp/tasks/000-dependencies.md` to `tmp/tasks-done/`.
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
