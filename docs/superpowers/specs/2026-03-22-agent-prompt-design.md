# Agent Prompt Configuration

## Overview

Add configurable prompts at three levels — global (server-wide), machine template, and per-user — that are injected into agent guideline files on machines. Arcad syncs these prompts periodically (every 5 minutes) from the server, replacing the current Ansible-based agent guidelines mechanism.

## Motivation

Currently, agent guidelines on machines are static boilerplate injected by Ansible at setup time. There is no way for administrators, template authors, or users to customize the prompts that guide AI agents on their machines. This feature enables:

- **Administrators** to set organization-wide instructions (e.g., internal API endpoints, coding standards)
- **Template authors** to set environment-specific instructions (e.g., "This is a Python project. Use poetry.")
- **Users** to set personal preferences (e.g., "Always respond in Japanese.")

## Data Model

### Global prompt

Add `agent_prompt` column to `setup_state` table:

```sql
ALTER TABLE setup_state ADD COLUMN agent_prompt TEXT NOT NULL DEFAULT '';
```

### Machine template prompt

Add `agent_prompt` field to `MachineTemplateConfig` protobuf message. This is stored in the existing `config_json` column — no DB schema change needed.

```protobuf
message MachineTemplateConfig {
  // ... existing fields ...
  string agent_prompt = N;
}
```

### User prompt

Add `agent_prompt` column to `user_settings` table:

```sql
ALTER TABLE user_settings ADD COLUMN agent_prompt TEXT NOT NULL DEFAULT '';
```

## API

### Settings APIs (existing RPCs extended)

- **Global prompt**: `SetupStatus` proto message gains `string agent_prompt` field. Read/written via existing `GetSetupStatus` / `UpdateDomainSettings` RPCs.
- **Template prompt**: `MachineTemplateConfig` gains `string agent_prompt` field. Read/written via existing `CreateMachineTemplate` / `UpdateMachineTemplate`.
- **User prompt**: `UserSettings` message gains `string agent_prompt` field. Read/written via existing `GetUserSettings` / `UpdateUserSettings`.

### Arcad retrieval API (new RPC)

Added to `ExposureService` (the service arcad already authenticates against):

```protobuf
rpc GetMachineAgentGuideline(GetMachineAgentGuidelineRequest) returns (GetMachineAgentGuidelineResponse);

message GetMachineAgentGuidelineRequest {}

message GetMachineAgentGuidelineResponse {
  string guideline = 1;  // Combined text (hardcoded + global + template + user)
}
```

No `machine_id` in the request — the machine is identified from the machine token (`Authorization: Bearer` header + `X-Arca-Machine-ID` header), following the same authentication pattern as `ReportMachineReadiness` and `GetMachineLLMModels`.

## Server-Side Guideline Assembly

The `GetMachineAgentGuideline` handler:

1. Identifies the machine from the machine token
2. Loads `setup_state.agent_prompt` (global)
3. Loads `template_config_json` from the machine record, extracts `agent_prompt` (template)
4. Loads `user_settings.agent_prompt` for the machine owner (user)
5. Assembles and returns the combined guideline:

```
{hardcoded guideline (port 11030, endpoint URL, etc.)}

{global prompt — omitted if empty}

{template prompt — omitted if empty}

{user prompt — omitted if empty}
```

The hardcoded guideline reuses the existing `agentGuidelineSection()` logic (port 11030, endpoint URL, etc.).

## Arcad Implementation

### AgentGuidelineSyncer

A new periodic syncer in arcad user mode, following the same pattern as `LLMSyncer`:

- **Interval**: 5 minutes (`AgentGuidelineSyncInterval`). First sync runs immediately on startup.
- **Change detection**: SHA256 hash of the response. Only writes files when content has changed.
- **Target files** (4 files, all in the interactive user's home directory — e.g., `/home/arcauser/`; the path is discovered via the `ARCA_INTERACTIVE_USER_HOME` environment variable):
  - `$HOME/.claude/CLAUDE.md`
  - `$HOME/.codex/AGENTS.md`
  - `$HOME/.gemini/GEMINI.md`
  - `$HOME/.config/AGENTS.md`
- **Write method**: Reuses `replaceOrAppendMarkedSection()` with `ARCA:AGENT_GUIDELINE_START` / `ARCA:AGENT_GUIDELINE_END` markers. Content outside markers is preserved.
- **Directory creation**: The syncer creates parent directories (`.claude/`, `.codex/`, etc.) if they don't exist.

### Ansible agent_guidelines role removal

The Ansible role `agent_guidelines` (file creation and blockinfile injection) is removed. All guideline management moves to the AgentGuidelineSyncer.

## UI

### Settings page (Global prompt)

Add an "Agent Prompt" section to the existing Settings page:

- Textarea for editing the global prompt
- Save button
- Helper text: "This prompt is applied to all machines. Example: 'Use our internal API at https://api.example.com'"

### Machine Template form (Template prompt)

Add an "Agent Prompt" field to the template create/edit form:

- Textarea
- Helper text: "This prompt is applied to machines created from this template. Example: 'This is a Python project environment. Use poetry for dependency management.'"

### User Settings page (User prompt)

Add an "Agent Prompt" section to the existing User Settings page:

- Textarea for editing the user prompt
- Save button
- Helper text: "This prompt is applied to all your machines. Example: 'Always respond in Japanese.'"

All textareas are plain text — no Markdown preview or rich editing.

## Backward Compatibility

- **Older arcad versions**: Will not call the new `GetMachineAgentGuideline` RPC, so they simply won't sync prompts. The Ansible-injected guidelines will remain on existing machines until arcad is updated. This is acceptable.
- **New arcad on machines with Ansible-injected guidelines**: The syncer uses the same markers (`ARCA:AGENT_GUIDELINE_START/END`), so it will seamlessly replace the Ansible-injected content.
- **Empty prompts**: When all three prompts are empty, only the hardcoded guideline is written (same as current behavior).
