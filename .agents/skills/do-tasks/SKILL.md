---
name: do-tasks
description: Execute task files from `tmp/tasks/` end-to-end. Use when asked to run project task markdown files, analyze dependencies, execute independent tasks in parallel, offload suitable work to sub-agents to reduce context usage, and move completed task files into `tmp/tasks-done/`.
---

# Do Tasks

Execute task markdown files in `tmp/tasks/` with dependency-aware, parallel-first orchestration.

## Workflow

1. List task files in `tmp/tasks/` (`NNN-*.md`).
2. Read `tmp/tasks/000-dependencies.md` if present, then read each task file.
3. Build a dependency DAG from explicit dependency lines first.
- Trust `000-dependencies.md` as the source of truth when it conflicts with inferred dependencies.
- If no explicit dependency exists, treat tasks as independent.
4. Classify tasks:
- `ready`: dependencies already completed.
- `blocked`: waiting for dependency or missing external input.
5. Execute `ready` tasks.
- Run independent `ready` tasks in parallel.
- Use sub-agents for self-contained tasks to reduce main-context usage.
- Keep dependency-coupled or high-risk integration work in the main agent when coordination is required.
6. For each completed task, move its file from `tmp/tasks/` to `tmp/tasks-done/` immediately.
- Create `tmp/tasks-done/` when missing.
- Preserve filename and markdown content.
7. Recompute the DAG state after every completion and continue until no `ready` tasks remain.
8. Ask one consolidated question set for unresolved blockers only after all executable work is exhausted.

## Dependency Rules

- Treat `depends on`, `blocked by`, `after`, `requires`, and task-id references as hard dependencies.
- Resolve by task id (`NNN`) instead of title text where possible.
- Reject cycles by reporting the minimal cycle set and stop those tasks until clarified.

## Sub-Agent Rules

- Delegate bounded tasks with clear ownership and disjoint write scope.
- Avoid delegating the immediate critical-path blocker when main-agent local execution is faster.
- While sub-agents run, continue non-overlapping main-agent work.
- Integrate returned changes, run validation, then mark task complete.

## Completion Rules

- Mark a task complete only after implementation and relevant verification for that task scope.
- Do not move partially completed or blocked tasks.
- Keep blocked files in `tmp/tasks/`.
- Never move `tmp/tasks/000-dependencies.md` to `tmp/tasks-done/`.

## Output Rules

- Report completed task files moved to `tmp/tasks-done/`.
- Report remaining blocked task files in `tmp/tasks/` with concrete blockers.
- Keep final status concise and actionable.
