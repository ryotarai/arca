---
name: ideas-to-tasks
description: Convert product ideas in `tmp/ideas.md` into concrete implementation task files under `tmp/tasks/NNN-TASK_NAME.md`. Use when asked to operationalize rough idea backlogs, build the best overall implementation plan across related ideas, merge or split tasks when appropriate, ask humans clarification questions when details are missing, and mark each fully processed idea as completed in `tmp/ideas.md`.
---

# Ideas To Tasks

Convert unchecked idea items in `tmp/ideas.md` into standalone task specs in `tmp/tasks/`.

## Workflow

1. Read `tmp/ideas.md` and collect unchecked checklist items (`- [ ] ...`).
2. Ignore already checked items (`- [x] ...`).
3. Determine the next task number by scanning both `tmp/tasks/*.md` and `tmp/tasks-done/*.md`, then taking `max(NNN) + 1` across both directories.
4. Build a planning view across all unchecked ideas before writing files.
5. Group ideas that target the same feature area, workflow, or shared implementation surface.
6. Decide per group whether to merge into one task file or split into multiple task files for better execution.
7. Check whether each planned task is specific enough to be executable.
8. If details are missing or ambiguous, ask concise clarification questions to a human before writing task files.
9. Create task files at `tmp/tasks/NNN-TASK_NAME.md` according to the plan (one-to-many or many-to-one versus original ideas).
10. After writing all files tied to an idea, update that idea line in `tmp/ideas.md` from `- [ ]` to `- [x]`.

## File Naming Rules

- Use zero-padded 3-digit numbers for `NNN` (`001`, `002`, ...).
- Always compute `NNN` from the global max across `tmp/tasks/` and `tmp/tasks-done/`.
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
- Keep each task focused; split oversized work into multiple task files when it improves parallelism or reduces risk.
- Preserve original intent from `tmp/ideas.md`; do not silently redefine the idea.

## Planning Rules

- Optimize for the best overall delivery plan, not strict one-idea-to-one-file mapping.
- Merge ideas into one task when they share the same code path, API surface, rollout dependency, or acceptance criteria.
- Split an idea into multiple tasks when it spans distinct layers, has independent milestones, or would produce an oversized task.
- Prefer task boundaries that minimize cross-task coupling and allow safe parallel execution.
- Keep traceability by mentioning covered idea text in each generated task file `Notes` section.

## Clarification Rules

- Ask a human when an idea lacks implementation-critical information (for example target runtime, UX behavior, compatibility constraints, security expectations, or migration scope).
- Prefer one consolidated question set for all currently ambiguous ideas instead of scattered questions.
- Keep ambiguous ideas unchecked until clarification is received and a task file is created.
- Do not invent irreversible product decisions to fill missing requirements.

## Completion Rules

- Treat the operation as complete only when both are done:
1. `tmp/tasks/NNN-TASK_NAME.md` files are created according to the merged/split plan.
2. Matching idea lines in `tmp/ideas.md` are checked (`- [x]`).
- If clarification is still pending, leave the corresponding idea line unchecked.
- Do not commit files under `tmp/` unless the user explicitly requests it.
