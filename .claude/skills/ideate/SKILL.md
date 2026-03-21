---
name: ideate
description: Use when asked to brainstorm ideas, find improvements, audit the codebase for UX gaps, code quality issues, or generate feature proposals for the current project.
---

# Ideate

Deeply analyze the codebase and generate a comprehensive set of improvement ideas, then append them to `tmp/ideas.md`.

## Process

1. **Explore the codebase thoroughly** using subagents in parallel:
   - **Frontend UX audit**: Examine all pages, components, routing, loading/error/empty states, accessibility, responsiveness, form handling, real-time update mechanisms, and search/filter capabilities.
   - **Backend architecture audit**: Examine API design, middleware chain, error handling, logging, database schema/indexes/queries, security posture, health checks, worker/job patterns, and robustness gaps.
   - **Holistic review**: Assess project goals, feature coverage, deployment patterns, testing coverage, dependency health, and developer experience.

2. **Generate ideas across all of these perspectives** (and any other relevant angles you discover):
   - UX and user experience improvements
   - Reliability and robustness fixes
   - Security hardening
   - Code quality and refactoring opportunities
   - New features that enhance convenience and utility
   - Novel and creative features — especially those useful for AI coding agent workflows and rapid prototyping

3. **Append ideas to `tmp/ideas.md`** following the existing format:
   - Use `- [ ]` checkbox format for each idea
   - Group by category with `##` headings
   - Each idea: concise title + 1–2 sentence description of the problem and proposed solution
   - Do not overwrite or reorder previous ideas — append new sections only
   - Deduplicate against existing ideas before appending

## Quality Bar

- Ideas must be **specific and actionable**, not vague ("improve performance").
- Each idea should explain **why** it matters, not just what to change.
- Prioritize root-cause improvements over surface-level polish.
- Include at least a few unconventional or creative proposals.
- Consider the project's domain and user base when proposing features.
- Cover both quick wins and larger strategic improvements.
