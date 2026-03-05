---
name: ideas-to-tasks
description: Convert product ideas in `tmp/ideas.md` into concrete implementation task files under `tmp/tasks/NNN-TASK_NAME.md`. Use when asked to operationalize idea backlogs, break vague ideas into actionable engineering tasks, and mark each processed idea as completed in `tmp/ideas.md`.
---

# Ideas To Tasks

Convert unchecked idea items in `tmp/ideas.md` into standalone task specs in `tmp/tasks/`.

## Workflow

1. Read `tmp/ideas.md` and collect unchecked checklist items (`- [ ] ...`).
2. Ignore already checked items (`- [x] ...`).
3. Determine the next task number by scanning `tmp/tasks/*.md` and taking `max(NNN) + 1`.
4. Process each unchecked idea in order of appearance.
5. Create one task file per idea at `tmp/tasks/NNN-TASK_NAME.md`.
6. After writing each task file, update the corresponding line in `tmp/ideas.md` from `- [ ]` to `- [x]`.

## File Naming Rules

- Use zero-padded 3-digit numbers for `NNN` (`001`, `002`, ...).
- Build `TASK_NAME` as uppercase snake case (`[A-Z0-9_]+`).
- Keep filenames ASCII only.
- Translate non-English idea text into concise English keywords for `TASK_NAME`.
- Avoid duplicate names; if needed, append a suffix like `_V2`.

## Task File Template

Use this structure for every generated task file:

```markdown
# NNN Task Title

## Goal
<One clear outcome sentence>

## Scope
- <In scope item 1>
- <In scope item 2>

## Tasks
1. <Concrete implementation step>
2. <Concrete implementation step>
3. <Concrete implementation step>

## Notes
- <Constraints, assumptions, or safety requirements>

## Open Questions
- None
```

## Quality Rules

- Make each task independently executable by another engineer/agent.
- Prefer concrete implementation language over product vagueness.
- Reference relevant repo areas (for example `proto/`, `internal/`, `web/`) when useful.
- Keep each task focused; split large ideas into phased steps inside the single task file.
- Preserve original intent from `tmp/ideas.md`; do not silently redefine the idea.

## Completion Rules

- Treat the operation as complete only when both are done:
1. `tmp/tasks/NNN-TASK_NAME.md` is created for each processed idea.
2. Matching idea lines in `tmp/ideas.md` are checked (`- [x]`).
- Do not commit files under `tmp/` unless the user explicitly requests it.
