---
name: ideas-to-tasks
description: Convert product ideas in `tmp/ideas.md` into concrete implementation task files under `tmp/tasks/NNN-TASK_NAME.md` and write task file content in Japanese. Use when asked to operationalize rough idea backlogs, build the best overall implementation plan across related ideas, merge or split tasks when appropriate, ask humans clarification questions when details are missing, and mark each fully processed idea as completed in `tmp/ideas.md`.
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
# NNN タスクタイトル

## 目的
<達成すべき成果を1文で記述>

## スコープ
- <対象範囲 1>
- <対象範囲 2>

## タスク
1. <具体的な実装ステップ>
2. <具体的な実装ステップ>
3. <具体的な実装ステップ>

## 補足
- <制約、前提、安全性要件など>

## 未解決事項
- なし
```

## Quality Rules

- Make each task independently executable by another engineer/agent.
- Prefer concrete implementation language over product vagueness.
- Reference relevant repo areas (for example `proto/`, `internal/`, `web/`) when useful.
- Keep each task focused; split oversized work into multiple task files when it improves parallelism or reduces risk.
- Preserve original intent from `tmp/ideas.md`; do not silently redefine the idea.
- Write all task file body content in Japanese.
- Keep code identifiers, paths, and command names as-is when Japanese translation would reduce precision.

## Planning Rules

- **"Ultrathink" before writing any task file.** Invest significant upfront effort in understanding the codebase, relevant interfaces, data models, and constraints so that the resulting task spec is detailed enough for an implementer to start coding immediately without further research or guesswork.
- When planning individual tasks, use the **Agent tool to research in parallel** — for example, spawn concurrent agents to explore different parts of the codebase (proto definitions, DB schema, frontend components, existing tests, etc.) that are relevant to different ideas or task groups. Maximize parallelism to gather context efficiently.
- Each task's `タスク` section must contain **concrete, step-by-step implementation instructions** including: specific files to modify/create, function signatures or API shapes to add, schema changes, UI component names, and expected behavior. Avoid vague steps like "implement the feature" — instead spell out exactly what code changes are needed.
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
