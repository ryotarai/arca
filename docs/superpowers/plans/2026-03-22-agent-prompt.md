# Agent Prompt Configuration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable configurable agent prompts at three levels (global, template, user) synced to machines by arcad every 5 minutes.

**Architecture:** Server stores prompts in `app_meta` (global), `users` table (per-user), and `MachineTemplateConfig` protobuf (template). A new `GetMachineAgentGuideline` RPC assembles all prompts. Arcad's new `AgentGuidelineSyncer` polls this RPC and writes to 4 guideline files.

**Tech Stack:** Go, Protobuf/ConnectRPC, React/TypeScript, SQLite/PostgreSQL

**Spec:** `docs/superpowers/specs/2026-03-22-agent-prompt-design.md`

---

## File Structure

### New files
- `internal/arcad/agent_guideline_syncer.go` — periodic syncer (mirrors `llm_syncer.go`)
- `internal/arcad/agent_guideline_syncer_test.go` — unit tests
- `internal/machine/agent_guideline_test.go` — unit tests for guideline assembly
- `internal/db/migrations_v2/000045_user_agent_prompt.up.sql` — migration to add `agent_prompt` column to `users` table

### Modified files
- `proto/arca/v1/exposure.proto` — add `GetMachineAgentGuideline` RPC + messages
- `proto/arca/v1/machine_template.proto` — add `agent_prompt` to `MachineTemplateConfig`
- `proto/arca/v1/setup.proto` — add `agent_prompt` to `SetupStatus` and `UpdateDomainSettingsRequest`
- `proto/arca/v1/user.proto` — add `GetUserAgentPrompt` / `UpdateUserAgentPrompt` RPCs + messages
- `internal/db/sqlc/query.sql` — add query for user agent prompt
- `internal/db/sqlc/schema.sql` — add `agent_prompt` column to `users` table
- `internal/db/setup_ticket_tunnel_store.go` — add `AgentPrompt` to `SetupState`, read/write via `app_meta`
- `internal/server/exposure_connect.go` — add `GetMachineAgentGuideline` handler
- `internal/server/setup_connect.go` — wire `agent_prompt` in `UpdateDomainSettings` + `GetSetupStatus`
- `internal/server/user_connect.go` — add `GetUserAgentPrompt` / `UpdateUserAgentPrompt` handlers
- `internal/machine/agent_guideline.go` — new `AssembleAgentGuideline()` function
- `internal/arcad/control_plane_client.go` — add `GetMachineAgentGuideline` method
- `cmd/arcad/main.go` — start `AgentGuidelineSyncer` in user mode
- `internal/arcad/ansible/roles/agent_guidelines/tasks/main.yml` — remove `blockinfile` tasks (keep dir creation for backward compat)
- `web/src/lib/api.ts` — add API functions for agent prompt CRUD
- `web/src/lib/types.ts` — add `agentPrompt` to `SetupStatus`
- `web/src/pages/AdminSettingsPage.tsx` — add "Agent Prompt" card
- `web/src/pages/SettingsPage.tsx` — add "Agent Prompt" card
- `web/src/pages/MachineTemplateFormPage.tsx` — add `agentPrompt` textarea field

---

## Task 1: Proto definitions

**Files:**
- Modify: `proto/arca/v1/exposure.proto`
- Modify: `proto/arca/v1/machine_template.proto`
- Modify: `proto/arca/v1/setup.proto`
- Modify: `proto/arca/v1/user.proto`

- [ ] **Step 1: Add `GetMachineAgentGuideline` to `exposure.proto`**

After line 12 (`GetMachineLLMModels`), add:

```protobuf
  rpc GetMachineAgentGuideline(GetMachineAgentGuidelineRequest) returns (GetMachineAgentGuidelineResponse);
```

At the end of the file, add:

```protobuf
message GetMachineAgentGuidelineRequest {}

message GetMachineAgentGuidelineResponse {
  string guideline = 1;
}
```

- [ ] **Step 2: Add `agent_prompt` to `MachineTemplateConfig` in `machine_template.proto`**

Add field 7 to `MachineTemplateConfig` (line 68-77):

```protobuf
message MachineTemplateConfig {
  oneof provider {
    LibvirtTemplateConfig libvirt = 1;
    GceTemplateConfig gce = 2;
    LxdTemplateConfig lxd = 4;
  }
  MachineExposureConfig exposure = 3;
  string server_api_url = 5;
  int64 auto_stop_timeout_seconds = 6;
  string agent_prompt = 7;
}
```

- [ ] **Step 3: Add `agent_prompt` to `SetupStatus` and `UpdateDomainSettingsRequest` in `setup.proto`**

Add to `SetupStatus` (after line 32, field 23):

```protobuf
  string agent_prompt = 24;
```

Add to `UpdateDomainSettingsRequest` (after line 79, field 21):

```protobuf
  string agent_prompt = 22;
```

- [ ] **Step 4: Add user agent prompt RPCs to `user.proto`**

Add to `UserService` (after `DuplicateUserLLMModel`):

```protobuf
  rpc GetUserAgentPrompt(GetUserAgentPromptRequest) returns (GetUserAgentPromptResponse);
  rpc UpdateUserAgentPrompt(UpdateUserAgentPromptRequest) returns (UpdateUserAgentPromptResponse);
```

Add messages at the end:

```protobuf
message GetUserAgentPromptRequest {}

message GetUserAgentPromptResponse {
  string agent_prompt = 1;
}

message UpdateUserAgentPromptRequest {
  string agent_prompt = 1;
}

message UpdateUserAgentPromptResponse {
  string agent_prompt = 1;
}
```

- [ ] **Step 5: Regenerate proto code**

Run: `make proto`
Expected: Generated files updated in `internal/gen/`

- [ ] **Step 6: Commit**

```bash
git add proto/ internal/gen/
git commit -m "Add agent prompt proto definitions"
```

---

## Task 2: Server-side storage (SetupState + app_meta)

**Files:**
- Modify: `internal/db/setup_ticket_tunnel_store.go`

- [ ] **Step 1: Add `AgentPrompt` to `SetupState` struct**

In `internal/db/setup_ticket_tunnel_store.go`, add `AgentPrompt string` to the `SetupState` struct.

- [ ] **Step 2: Add meta key constant**

Add constant:

```go
setupMetaAgentPrompt = "agent_prompt"
```

- [ ] **Step 3: Read `AgentPrompt` in `GetSetupState()`**

Add reading `agent_prompt` from `app_meta` in `GetSetupState()`, following the same pattern as `setupMetaMachineRuntime`:

```go
agentPrompt, err := s.getMetaValue(ctx, setupMetaAgentPrompt)
if err != nil && !errors.Is(err, sql.ErrNoRows) {
    return SetupState{}, err
}
```

Set `state.AgentPrompt = agentPrompt` in the returned struct.

- [ ] **Step 4: Write `AgentPrompt` in `UpsertSetupState()`**

Add writing `agent_prompt` to `app_meta` in `UpsertSetupState()`, following the same pattern as other meta values:

```go
if err := s.setMetaValue(ctx, setupMetaAgentPrompt, state.AgentPrompt); err != nil {
    return err
}
```

- [ ] **Step 5: Add migration for `users.agent_prompt` column**

Create `internal/db/migrations_v2/000045_user_agent_prompt.up.sql`:

```sql
ALTER TABLE users ADD COLUMN agent_prompt TEXT NOT NULL DEFAULT '';
```

Update `internal/db/sqlc/schema.sql` to add the column to the `users` CREATE TABLE (for sqlc codegen):

```sql
  agent_prompt TEXT NOT NULL DEFAULT '',
```

- [ ] **Step 6: Add sqlc queries for user agent prompt**

In `internal/db/sqlc/query.sql`, add:

```sql
-- name: GetUserAgentPromptByID :one
SELECT agent_prompt FROM users WHERE id = sqlc.arg(id) LIMIT 1;

-- name: UpdateUserAgentPromptByID :execrows
UPDATE users SET agent_prompt = sqlc.arg(agent_prompt) WHERE id = sqlc.arg(id);
```

Run: `make sqlc`

- [ ] **Step 7: Add user agent prompt store methods**

Add to the Store (in an appropriate store file):

```go
func (s *Store) GetUserAgentPrompt(ctx context.Context, userID string) (string, error) {
    return s.q.GetUserAgentPromptByID(ctx, userID)
}

func (s *Store) SetUserAgentPrompt(ctx context.Context, userID, prompt string) error {
    rows, err := s.q.UpdateUserAgentPromptByID(ctx, /* params */)
    if err != nil {
        return err
    }
    if rows == 0 {
        return fmt.Errorf("user not found: %s", userID)
    }
    return nil
}
```

- [ ] **Step 8: Verify build**

Run: `go build ./...`

- [ ] **Step 9: Commit**

```bash
git add internal/db/
git commit -m "Add agent prompt storage (app_meta for global, users table for per-user)"
```

---

## Task 3: Server handler — GetSetupStatus / UpdateDomainSettings

**Files:**
- Modify: `internal/server/setup_connect.go`

- [ ] **Step 1: Wire `AgentPrompt` in `GetSetupStatus` response**

In `GetSetupStatus()`, where the `SetupStatus` proto response is built from the `SetupState`, add:

```go
AgentPrompt: state.AgentPrompt,
```

- [ ] **Step 2: Wire `AgentPrompt` in `UpdateDomainSettings`**

In `UpdateDomainSettings()`, after the existing field assignments (around line 228), add:

```go
current.AgentPrompt = req.Msg.GetAgentPrompt()
```

Note: empty string is a valid value (clears the prompt), so no trimming/validation needed beyond what the proto gives us.

- [ ] **Step 3: Verify build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/server/setup_connect.go
git commit -m "Wire agent prompt in GetSetupStatus and UpdateDomainSettings"
```

---

## Task 4: Server handler — User agent prompt RPCs

**Files:**
- Modify: `internal/server/user_connect.go`

- [ ] **Step 1: Add `GetUserAgentPrompt` handler**

```go
func (s *userConnectService) GetUserAgentPrompt(ctx context.Context, req *connect.Request[arcav1.GetUserAgentPromptRequest]) (*connect.Response[arcav1.GetUserAgentPromptResponse], error) {
    user, err := s.authenticate(ctx, req.Header())
    if err != nil {
        return nil, err
    }
    prompt, err := s.store.GetUserAgentPrompt(ctx, user.ID)
    if err != nil {
        slog.ErrorContext(ctx, "get user agent prompt failed", "error", err)
        return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get agent prompt"))
    }
    return connect.NewResponse(&arcav1.GetUserAgentPromptResponse{AgentPrompt: prompt}), nil
}
```

- [ ] **Step 2: Add `UpdateUserAgentPrompt` handler**

```go
func (s *userConnectService) UpdateUserAgentPrompt(ctx context.Context, req *connect.Request[arcav1.UpdateUserAgentPromptRequest]) (*connect.Response[arcav1.UpdateUserAgentPromptResponse], error) {
    user, err := s.authenticate(ctx, req.Header())
    if err != nil {
        return nil, err
    }
    prompt := req.Msg.GetAgentPrompt()
    if err := s.store.SetUserAgentPrompt(ctx, user.ID, prompt); err != nil {
        slog.ErrorContext(ctx, "set user agent prompt failed", "error", err)
        return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update agent prompt"))
    }
    return connect.NewResponse(&arcav1.UpdateUserAgentPromptResponse{AgentPrompt: prompt}), nil
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/server/user_connect.go
git commit -m "Add GetUserAgentPrompt and UpdateUserAgentPrompt handlers"
```

---

## Task 5: Guideline assembly logic

**Files:**
- Modify: `internal/machine/agent_guideline.go`
- Create: `internal/machine/agent_guideline_test.go`

- [ ] **Step 1: Write tests for `AssembleAgentGuideline`**

Create `internal/machine/agent_guideline_test.go`:

```go
package machine

import "testing"

func TestAssembleAgentGuideline(t *testing.T) {
    tests := []struct {
        name           string
        endpointURL    string
        globalPrompt   string
        templatePrompt string
        userPrompt     string
        wantContains   []string
        wantNotContain []string
    }{
        {
            name:         "all empty prompts",
            endpointURL:  "https://test.example.com",
            wantContains: []string{"ARCA:AGENT_GUIDELINE_START", ":11030", "https://test.example.com", "ARCA:AGENT_GUIDELINE_END"},
            wantNotContain: []string{"Global", "Template", "User"},
        },
        {
            name:           "all prompts set",
            endpointURL:    "https://test.example.com",
            globalPrompt:   "Use internal API",
            templatePrompt: "This is Python",
            userPrompt:     "Respond in Japanese",
            wantContains:   []string{"Use internal API", "This is Python", "Respond in Japanese"},
        },
        {
            name:         "only global prompt",
            endpointURL:  "https://test.example.com",
            globalPrompt: "Global instruction",
            wantContains: []string{"Global instruction"},
            wantNotContain: []string{"Template", "User"},
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := AssembleAgentGuideline(tt.endpointURL, tt.globalPrompt, tt.templatePrompt, tt.userPrompt)
            for _, want := range tt.wantContains {
                if !containsString(got, want) {
                    t.Errorf("result should contain %q, got:\n%s", want, got)
                }
            }
            for _, notWant := range tt.wantNotContain {
                if containsString(got, notWant) {
                    t.Errorf("result should not contain %q, got:\n%s", notWant, got)
                }
            }
        })
    }
}

func containsString(s, substr string) bool {
    return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
    return len(substr) == 0 || (len(s) >= len(substr) && (s[:len(substr)] == substr || containsSubstring(s[1:], substr)))
}
```

(Use `strings.Contains` instead of the helper — just import `strings` in the test.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/machine/ -run TestAssembleAgentGuideline -v`
Expected: FAIL — `AssembleAgentGuideline` not defined

- [ ] **Step 3: Implement `AssembleAgentGuideline`**

Add to `internal/machine/agent_guideline.go`:

```go
// AssembleAgentGuideline builds the full managed guideline section from
// the hardcoded guidelines and the three configurable prompt layers.
func AssembleAgentGuideline(endpointURL, globalPrompt, templatePrompt, userPrompt string) string {
    var b strings.Builder
    b.WriteString(agentGuidelineMarkerStart)
    b.WriteString("\n")
    b.WriteString("# Arca Agent Guidelines\n\n")
    b.WriteString("This section is managed by Arca and is safe to re-generate.\n\n")
    b.WriteString("- Run your application HTTP server on `:11030`.\n")
    b.WriteString("- Endpoint URL inside this machine: `" + strings.TrimSpace(endpointURL) + "`.\n")
    b.WriteString("- Requests to the endpoint URL are delivered to port `11030` on this machine.\n")
    b.WriteString("- The server process is started and supervised by `systemd`.\n")
    b.WriteString("- Visibility scope (`owner only`, `specific users`, `all arca users`, `internet public`) is configured in the arca app (server).\n")

    if strings.TrimSpace(globalPrompt) != "" {
        b.WriteString("\n")
        b.WriteString(strings.TrimSpace(globalPrompt))
        b.WriteString("\n")
    }
    if strings.TrimSpace(templatePrompt) != "" {
        b.WriteString("\n")
        b.WriteString(strings.TrimSpace(templatePrompt))
        b.WriteString("\n")
    }
    if strings.TrimSpace(userPrompt) != "" {
        b.WriteString("\n")
        b.WriteString(strings.TrimSpace(userPrompt))
        b.WriteString("\n")
    }

    b.WriteString("\n")
    b.WriteString("You can add your own notes outside this managed block.\n")
    b.WriteString(agentGuidelineMarkerEnd)
    b.WriteString("\n")
    return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/machine/ -run TestAssembleAgentGuideline -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/machine/
git commit -m "Add AssembleAgentGuideline with configurable prompt layers"
```

---

## Task 6: Server handler — GetMachineAgentGuideline

**Files:**
- Modify: `internal/server/exposure_connect.go`

- [ ] **Step 1: Add `GetMachineAgentGuideline` handler**

Add after `GetMachineLLMModels` handler. Follow the same auth pattern:

```go
func (s *exposureConnectService) GetMachineAgentGuideline(ctx context.Context, req *connect.Request[arcav1.GetMachineAgentGuidelineRequest]) (*connect.Response[arcav1.GetMachineAgentGuidelineResponse], error) {
    if s.store == nil {
        return nil, connect.NewError(connect.CodeUnavailable, errors.New("exposure service unavailable"))
    }

    machineToken := strings.TrimSpace(machineTokenFromHeader(req.Header()))
    if machineToken == "" {
        return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("machine token is required"))
    }
    machineID, err := s.store.GetMachineIDByMachineToken(ctx, machineToken)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid machine token"))
        }
        slog.ErrorContext(ctx, "get machine id by token failed", "error", err)
        return nil, connect.NewError(connect.CodeInternal, errors.New("failed to authorize machine"))
    }

    // Get machine to extract template config and owner
    m, err := s.store.GetMachineByID(ctx, machineID)
    if err != nil {
        slog.ErrorContext(ctx, "get machine failed", "error", err)
        return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get machine"))
    }

    // Extract template agent_prompt from machine's snapshotted config
    templatePrompt := ""
    if m.TemplateConfigJSON != "" {
        var tmplConfig struct {
            AgentPrompt string `json:"agentPrompt"`
        }
        if err := json.Unmarshal([]byte(m.TemplateConfigJSON), &tmplConfig); err == nil {
            templatePrompt = tmplConfig.AgentPrompt
        }
    }

    // Get global prompt from setup state
    setupState, err := s.store.GetSetupState(ctx)
    if err != nil {
        slog.ErrorContext(ctx, "get setup state failed", "error", err)
        return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get setup state"))
    }

    // Get user prompt from machine owner
    userPrompt := ""
    ownerUserID, err := s.store.GetMachineOwnerUserID(ctx, machineID)
    if err != nil && !errors.Is(err, sql.ErrNoRows) {
        slog.ErrorContext(ctx, "get machine owner failed", "error", err)
    }
    if ownerUserID != "" {
        userPrompt, _ = s.store.GetUserAgentPrompt(ctx, ownerUserID)
    }

    // Build endpoint URL for hardcoded guidelines
    endpointURL := machine.BuildEndpointURL(m, setupState)

    guideline := machine.AssembleAgentGuideline(endpointURL, setupState.AgentPrompt, templatePrompt, userPrompt)

    return connect.NewResponse(&arcav1.GetMachineAgentGuidelineResponse{Guideline: guideline}), nil
}
```

Note: The exact method to get the endpoint URL and machine struct needs to match existing code. Check how `agentGuidelineSection` is currently called and how `endpointURL` is derived. If there's no `BuildEndpointURL` helper, extract it from the existing usage or construct the URL from machine exposures (hostname from exposures table + domain config).

- [ ] **Step 2: Verify build**

Run: `go build ./...`

- [ ] **Step 3: Commit**

```bash
git add internal/server/exposure_connect.go
git commit -m "Add GetMachineAgentGuideline handler"
```

---

## Task 7: Arcad — ControlPlaneClient extension

**Files:**
- Modify: `internal/arcad/control_plane_client.go`

- [ ] **Step 1: Add endpoint constant**

```go
getMachineAgentGuidelineEndpoint = "/arca.v1.ExposureService/GetMachineAgentGuideline"
```

- [ ] **Step 2: Add interface method**

Add to `ControlPlaneClient` interface:

```go
GetMachineAgentGuideline(ctx context.Context) (string, error)
```

- [ ] **Step 3: Implement on `HTTPControlPlaneClient`**

Follow the same pattern as `GetMachineLLMModels`:

```go
func (c *HTTPControlPlaneClient) GetMachineAgentGuideline(ctx context.Context) (string, error) {
    body, err := json.Marshal(map[string]string{})
    if err != nil {
        return "", err
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+getMachineAgentGuidelineEndpoint, bytes.NewReader(body))
    if err != nil {
        return "", err
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Arca-Machine-ID", c.machineID)
    if strings.TrimSpace(c.machineToken) != "" {
        req.Header.Set("Authorization", "Bearer "+c.machineToken)
    }
    resp, err := c.httpClient.Do(req)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("get machine agent guideline failed: status %d", resp.StatusCode)
    }
    var decoded struct {
        Guideline string `json:"guideline"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
        return "", fmt.Errorf("decode machine agent guideline: %w", err)
    }
    return decoded.Guideline, nil
}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/arcad/control_plane_client.go
git commit -m "Add GetMachineAgentGuideline to ControlPlaneClient"
```

---

## Task 8: Arcad — AgentGuidelineSyncer

**Files:**
- Create: `internal/arcad/agent_guideline_syncer.go`
- Create: `internal/arcad/agent_guideline_syncer_test.go`

- [ ] **Step 1: Write test for syncer**

Create `internal/arcad/agent_guideline_syncer_test.go` with tests for `writeGuidelineFiles` (the file-writing logic, testable in isolation):

```go
package arcad

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestWriteGuidelineFiles(t *testing.T) {
    tmpDir := t.TempDir()

    guideline := "<!-- ARCA:AGENT_GUIDELINE_START -->\ntest content\n<!-- ARCA:AGENT_GUIDELINE_END -->\n"

    if err := writeGuidelineFiles(tmpDir, guideline); err != nil {
        t.Fatalf("writeGuidelineFiles: %v", err)
    }

    paths := []string{
        filepath.Join(tmpDir, ".claude", "CLAUDE.md"),
        filepath.Join(tmpDir, ".codex", "AGENTS.md"),
        filepath.Join(tmpDir, ".gemini", "GEMINI.md"),
        filepath.Join(tmpDir, ".config", "AGENTS.md"),
    }
    for _, p := range paths {
        data, err := os.ReadFile(p)
        if err != nil {
            t.Errorf("read %s: %v", p, err)
            continue
        }
        if !strings.Contains(string(data), "test content") {
            t.Errorf("%s missing guideline content", p)
        }
    }

    // Write again with different content — should replace, not duplicate
    guideline2 := "<!-- ARCA:AGENT_GUIDELINE_START -->\nupdated content\n<!-- ARCA:AGENT_GUIDELINE_END -->\n"
    if err := writeGuidelineFiles(tmpDir, guideline2); err != nil {
        t.Fatalf("writeGuidelineFiles (update): %v", err)
    }
    for _, p := range paths {
        data, err := os.ReadFile(p)
        if err != nil {
            t.Errorf("read %s: %v", p, err)
            continue
        }
        content := string(data)
        if !strings.Contains(content, "updated content") {
            t.Errorf("%s missing updated content", p)
        }
        if strings.Contains(content, "test content") {
            t.Errorf("%s still has old content", p)
        }
    }
}

func TestWriteGuidelineFilesPreservesUserContent(t *testing.T) {
    tmpDir := t.TempDir()
    claudeDir := filepath.Join(tmpDir, ".claude")
    os.MkdirAll(claudeDir, 0755)

    // Pre-existing user content
    existing := "# My notes\nSome user content\n"
    os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(existing), 0644)

    guideline := "<!-- ARCA:AGENT_GUIDELINE_START -->\nmanaged\n<!-- ARCA:AGENT_GUIDELINE_END -->\n"
    if err := writeGuidelineFiles(tmpDir, guideline); err != nil {
        t.Fatalf("writeGuidelineFiles: %v", err)
    }

    data, _ := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
    content := string(data)
    if !strings.Contains(content, "Some user content") {
        t.Error("user content was lost")
    }
    if !strings.Contains(content, "managed") {
        t.Error("managed content missing")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/arcad/ -run TestWriteGuideline -v`
Expected: FAIL

- [ ] **Step 3: Implement `AgentGuidelineSyncer`**

Create `internal/arcad/agent_guideline_syncer.go`:

```go
package arcad

import (
    "context"
    "crypto/sha256"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/ryotarai/arca/internal/machine"
)

// AgentGuidelineSyncer periodically fetches the assembled agent guideline
// from the control plane and writes it to agent guideline files.
type AgentGuidelineSyncer struct {
    client   ControlPlaneClient
    homeDir  string
    interval time.Duration
    lastHash string
}

func NewAgentGuidelineSyncer(client ControlPlaneClient, homeDir string, interval time.Duration) *AgentGuidelineSyncer {
    if interval <= 0 {
        interval = 5 * time.Minute
    }
    return &AgentGuidelineSyncer{
        client:   client,
        homeDir:  homeDir,
        interval: interval,
    }
}

func (s *AgentGuidelineSyncer) Run(ctx context.Context) {
    s.syncOnce(ctx)

    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.syncOnce(ctx)
        }
    }
}

func (s *AgentGuidelineSyncer) syncOnce(ctx context.Context) {
    fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    guideline, err := s.client.GetMachineAgentGuideline(fetchCtx)
    if err != nil {
        log.Printf("agent-guideline-syncer: fetch guideline: %v", err)
        return
    }

    hash := fmt.Sprintf("%x", sha256.Sum256([]byte(guideline)))
    if hash == s.lastHash {
        return
    }

    if err := writeGuidelineFiles(s.homeDir, guideline); err != nil {
        log.Printf("agent-guideline-syncer: write files: %v", err)
        return
    }

    s.lastHash = hash
    log.Printf("agent-guideline-syncer: synced guideline to %s", s.homeDir)
}

// guidelineTargets are the files to write, relative to the home directory.
var guidelineTargets = []struct {
    dir  string
    file string
}{
    {".claude", "CLAUDE.md"},
    {".codex", "AGENTS.md"},
    {".gemini", "GEMINI.md"},
    {".config", "AGENTS.md"},
}

func writeGuidelineFiles(homeDir, managedSection string) error {
    for _, target := range guidelineTargets {
        dir := filepath.Join(homeDir, target.dir)
        if err := os.MkdirAll(dir, 0755); err != nil {
            return fmt.Errorf("mkdir %s: %w", dir, err)
        }
        path := filepath.Join(dir, target.file)
        existing, err := os.ReadFile(path)
        if err != nil && !os.IsNotExist(err) {
            return fmt.Errorf("read %s: %w", path, err)
        }
        updated := machine.ReplaceOrAppendMarkedSection(string(existing), managedSection)
        if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
            return fmt.Errorf("write %s: %w", path, err)
        }
    }
    return nil
}
```

Note: This requires exporting `ReplaceOrAppendMarkedSection` from `internal/machine/agent_guideline.go` (currently `replaceOrAppendMarkedSection` is unexported). Rename it to `ReplaceOrAppendMarkedSection`.

- [ ] **Step 4: Export `ReplaceOrAppendMarkedSection` in `agent_guideline.go`**

In `internal/machine/agent_guideline.go`, rename `replaceOrAppendMarkedSection` to `ReplaceOrAppendMarkedSection` and update any callers.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/arcad/ -run TestWriteGuideline -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/arcad/ internal/machine/
git commit -m "Add AgentGuidelineSyncer for periodic guideline sync"
```

---

## Task 9: Start syncer in arcad user mode

**Files:**
- Modify: `cmd/arcad/main.go`

- [ ] **Step 1: Add syncer startup in `runUserMode`**

After line 84 (`go arcad.NewLLMSyncer(...).Run(ctx)`), add:

```go
go arcad.NewAgentGuidelineSyncer(controlPlaneClient, os.Getenv("HOME"), 5*time.Minute).Run(ctx)
```

If `ARCA_INTERACTIVE_USER_HOME` env var exists, use that instead of `HOME`:

```go
guidelineHomeDir := os.Getenv("ARCA_INTERACTIVE_USER_HOME")
if guidelineHomeDir == "" {
    guidelineHomeDir = os.Getenv("HOME")
}
go arcad.NewAgentGuidelineSyncer(controlPlaneClient, guidelineHomeDir, 5*time.Minute).Run(ctx)
```

- [ ] **Step 2: Verify build**

Run: `go build ./cmd/arcad/`

- [ ] **Step 3: Commit**

```bash
git add cmd/arcad/main.go
git commit -m "Start AgentGuidelineSyncer in arcad user mode"
```

---

## Task 10: Remove Ansible agent_guidelines blockinfile

**Files:**
- Modify: `internal/arcad/ansible/roles/agent_guidelines/tasks/main.yml`

- [ ] **Step 1: Remove blockinfile tasks, keep directory creation**

Keep the directory creation tasks (ensuring `~/.claude/`, `~/.codex/`, `~/.gemini/`, `~/.config/` exist) but remove the `blockinfile` tasks that inject the managed section. The syncer now handles content injection.

Also keep the "Ensure guideline files exist" tasks (creating empty files if they don't exist) — these provide a good baseline for first boot before the syncer runs.

Remove the `blockinfile` tasks (the ones with `ARCA:AGENT_GUIDELINE` markers).

- [ ] **Step 2: Commit**

```bash
git add internal/arcad/ansible/
git commit -m "Remove Ansible blockinfile injection, keep directory creation"
```

---

## Task 11: Frontend — Admin Settings (global agent prompt)

**Files:**
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/pages/AdminSettingsPage.tsx`

- [ ] **Step 1: Add `agentPrompt` to `SetupStatus` type**

In `web/src/lib/types.ts`, add to `SetupStatus`:

```typescript
agentPrompt: string
```

- [ ] **Step 2: Update `getSetupStatus` to parse `agentPrompt`**

In `web/src/lib/api.ts`, in the `getSetupStatus` function, add `agentPrompt` to the response parsing.

- [ ] **Step 3: Update `updateDomainSettings` to send `agentPrompt`**

Add `agentPrompt` parameter to `updateDomainSettings` and include it in the request body.

- [ ] **Step 4: Add Agent Prompt card to AdminSettingsPage**

In `web/src/pages/AdminSettingsPage.tsx`, add a new `AgentPromptCard` component before the `SlackSettingsCard`:

```tsx
function AgentPromptCard({ setupStatus, onSetupStatusChange }: { setupStatus: SetupStatus; onSetupStatusChange: (s: SetupStatus) => void }) {
  const [agentPrompt, setAgentPrompt] = useState(setupStatus.agentPrompt)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    setAgentPrompt(setupStatus.agentPrompt)
  }, [setupStatus.agentPrompt])

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      await updateDomainSettings(/* ... existing params ..., */ agentPrompt)
      onSetupStatusChange({ ...setupStatus, agentPrompt })
      setSaved(true)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="py-0 shadow-sm">
      <CardHeader className="space-y-2 p-6 pb-3">
        <CardTitle className="text-xl">Agent prompt</CardTitle>
        <CardDescription>
          This prompt is injected into agent guidelines on all machines.
        </CardDescription>
      </CardHeader>
      <CardContent className="p-6 pt-3">
        <form className="space-y-4" onSubmit={submit}>
          <textarea
            value={agentPrompt}
            onChange={(e) => setAgentPrompt(e.target.value)}
            rows={6}
            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            placeholder="Example: Use our internal API at https://api.example.com"
          />
          <Button type="submit" className="h-10 w-full" disabled={saving}>
            {saving ? 'Saving...' : 'Save agent prompt'}
          </Button>
        </form>
        {saved && <p className="mt-3 text-sm text-emerald-300">Agent prompt updated.</p>}
        {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
      </CardContent>
    </Card>
  )
}
```

Note: The global agent prompt card should have its own save button, separate from the main domain settings form. This avoids coupling it to the existing large form.

However, since `updateDomainSettings` is a single RPC that saves all settings together, the agent prompt needs to be sent along with all other current values. The cleanest approach is to either:
(a) Add the textarea inside the existing form (simplest, one save button for all settings)
(b) Call `updateDomainSettings` with all current values when saving just the agent prompt

Option (a) is simpler and consistent — add the textarea to the existing form before the Save button. Then `agentPrompt` is just another state field like `serverDomain`.

- [ ] **Step 5: Verify frontend builds**

Run: `cd web && npm run build`

- [ ] **Step 6: Commit**

```bash
git add web/src/
git commit -m "Add global agent prompt to Admin Settings page"
```

---

## Task 12: Frontend — User Settings (user agent prompt)

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/pages/SettingsPage.tsx`

- [ ] **Step 1: Add API functions**

In `web/src/lib/api.ts`, add:

```typescript
export async function getUserAgentPrompt(): Promise<string> {
  const response = await callConnectJSONCandidates<{ agentPrompt?: string }>(
    ['/arca.v1.UserService/GetUserAgentPrompt'],
    {},
  )
  return response.agentPrompt ?? ''
}

export async function updateUserAgentPrompt(agentPrompt: string): Promise<string> {
  const response = await callConnectJSONCandidates<{ agentPrompt?: string }>(
    ['/arca.v1.UserService/UpdateUserAgentPrompt'],
    { agentPrompt },
  )
  return response.agentPrompt ?? ''
}
```

- [ ] **Step 2: Add `AgentPromptCard` to SettingsPage**

Add a new card component in `SettingsPage.tsx`:

```tsx
function AgentPromptCard({ userId }: { userId: string }) {
  const [agentPrompt, setAgentPrompt] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const prompt = await getUserAgentPrompt()
        if (!cancelled) setAgentPrompt(prompt)
      } catch (e) {
        if (!cancelled) setError(messageFromError(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [userId])

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSaving(true)
    setError('')
    setSaved(false)
    try {
      const result = await updateUserAgentPrompt(agentPrompt)
      setAgentPrompt(result)
      setSaved(true)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="py-0 shadow-sm">
      <CardHeader className="space-y-2 p-6 pb-3">
        <CardTitle className="text-xl">Agent prompt</CardTitle>
        <CardDescription>
          This prompt is injected into agent guidelines on all your machines.
        </CardDescription>
      </CardHeader>
      <CardContent className="p-6 pt-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <form className="space-y-4" onSubmit={submit}>
            <textarea
              value={agentPrompt}
              onChange={(e) => setAgentPrompt(e.target.value)}
              rows={6}
              className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              placeholder="Example: Always respond in Japanese."
            />
            <Button type="submit" className="h-10 w-full" disabled={saving}>
              {saving ? 'Saving...' : 'Save agent prompt'}
            </Button>
          </form>
        )}
        {saved && <p className="mt-3 text-sm text-emerald-300">Agent prompt updated.</p>}
        {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
      </CardContent>
    </Card>
  )
}
```

Add `<AgentPromptCard userId={user.id} />` to the SettingsPage layout, before `<LLMModelsCard>`.

- [ ] **Step 3: Verify frontend builds**

Run: `cd web && npm run build`

- [ ] **Step 4: Commit**

```bash
git add web/src/
git commit -m "Add user agent prompt to Settings page"
```

---

## Task 13: Frontend — Machine Template form (template agent prompt)

**Files:**
- Modify: `web/src/pages/MachineTemplateFormPage.tsx`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/lib/types.ts`

- [ ] **Step 1: Add `agentPrompt` to `MachineTemplateItem` type**

In `web/src/lib/types.ts`, add to `MachineTemplateItem`:

```typescript
agentPrompt: string
```

- [ ] **Step 2: Add `agentPrompt` to template form state**

In `MachineTemplateFormPage.tsx`, add `agentPrompt: string` to `TemplateFormState`, default to `''` in `emptyTemplateForm()`.

- [ ] **Step 3: Wire `agentPrompt` in `toConfig`, `fillFormFromTemplate`, and API**

In `templateConfigPayload` (api.ts), add `agentPrompt` to the config object sent to the server.

In `toMachineTemplateItem` (api.ts) response parsing, extract `agentPrompt`.

In `fillFormFromTemplate`, populate `agentPrompt` from template data.

- [ ] **Step 4: Add textarea to template form UI**

After the auto-stop timeout field (line ~511), add:

```tsx
<div className="space-y-2">
  <Label htmlFor="template-agent-prompt">Agent prompt (optional)</Label>
  <textarea
    id="template-agent-prompt"
    value={form.agentPrompt}
    onChange={(event) => setForm((current) => ({ ...current, agentPrompt: event.target.value }))}
    rows={4}
    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    placeholder="Example: This is a Python project environment. Use poetry for dependency management."
  />
  <p className="text-xs text-muted-foreground">This prompt is injected into agent guidelines on machines created from this template.</p>
</div>
```

- [ ] **Step 5: Verify frontend builds**

Run: `cd web && npm run build`

- [ ] **Step 6: Commit**

```bash
git add web/src/
git commit -m "Add agent prompt field to machine template form"
```

---

## Task 14: Backend tests

**Files:**
- Modify: `make test/backend`

- [ ] **Step 1: Run full backend test suite**

Run: `make test/backend`
Expected: PASS

- [ ] **Step 2: Fix any failures**

If tests fail, fix and re-run.

- [ ] **Step 3: Commit any fixes**

---

## Task 15: E2E tests

**Files:**
- Create: `web/e2e/agent-prompt.spec.ts`

- [ ] **Step 1: Write E2E test for agent prompt settings**

Create `web/e2e/agent-prompt.spec.ts` covering:
- Admin can set and save global agent prompt
- User can set and save user agent prompt
- Template form includes agent prompt field

- [ ] **Step 2: Run E2E tests**

Run: `cd web && npx playwright test --project=fast e2e/agent-prompt.spec.ts`

- [ ] **Step 3: Fix any failures and commit**

```bash
git add web/e2e/
git commit -m "Add E2E tests for agent prompt configuration"
```

---

## Task 16: Final verification and build

- [ ] **Step 1: Run `make build`**

Run: `make build`
Expected: Clean build

- [ ] **Step 2: Run `make test/backend`**

Expected: All pass

- [ ] **Step 3: Run `make test/e2e`**

Expected: All pass

- [ ] **Step 4: Final commit if needed**
