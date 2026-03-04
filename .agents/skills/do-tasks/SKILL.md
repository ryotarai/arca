---
name: do-tasks
description: Process task lists in tmp/tasks.md end-to-end. Use when asked to execute all tasks from a markdown checklist, respect task dependencies, mark completed tasks as - [x], move completed tasks to tmp/tasks-done.md, defer only items that require human input, and ask all remaining questions in one batch at the end.
---

# Do Tasks

Execute tasks in `tmp/tasks.md` autonomously with dependency-aware ordering.

## Workflow

1. Read `tmp/tasks.md` and extract all unchecked tasks (`- [ ]`).
2. Detect dependencies from explicit signals first.
- Treat text such as `depends on`, `blocked by`, `after`, `requires`, or task-number references as hard dependencies.
- If dependency structure is unclear, use file order as the fallback order and state that assumption.
3. Split tasks into two groups.
- `doable now`: Can be executed without user decisions.
- `needs input`: Requires human choice, missing credentials, or unavailable external info.
4. Execute all `doable now` tasks.
- Run independent tasks in parallel when safe.
- Run dependent tasks sequentially in resolved order.
5. After each completed task, immediately update the task text to `- [x]`.
6. Move each checked completed task from `tmp/tasks.md` to `tmp/tasks-done.md` right away.
- If `tmp/tasks-done.md` does not exist, create it.
- Preserve the original task wording when moving it.
7. Leave `needs input` tasks unchecked in `tmp/tasks.md` and continue with every other task.
8. When no more doable tasks remain, ask one consolidated question set covering every remaining blocker.

## Execution Rules

- Prefer finishing implementation, verification, and local validation before marking done.
- Keep unfinished and blocked tasks in `tmp/tasks.md`; keep completed tasks in `tmp/tasks-done.md`.
- Do not mark a task done if it is only partially complete.
- If a task becomes unblocked during execution, move it back to `doable now` and continue.
- Prefer multi-agent execution when possible: assign independent tasks to separate agents and keep dependency-bound tasks sequential.
- Keep questions concise and grouped, not scattered through the run.
- Do not bundle commits; split commits into the smallest meaningful units with clear intent.

## Output Rules

- Report what was completed and what remains blocked.
- Include concrete next actions for each blocked task.
- Keep responses concise and action-oriented.
