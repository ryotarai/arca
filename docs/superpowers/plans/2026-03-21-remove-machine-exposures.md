# Remove machine_exposures Table Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `machine_exposures` table, eliminate duplicated domain config from templates, and compute hostnames dynamically from `setup_state`.

**Architecture:** All hostname computation moves to a shared `MachineHostname()` function using prefix/base_domain from `setup_state`. The proxy, console authorize, and ticket validation flows resolve hostname → machine name instead of looking up exposures. The `machines.endpoint` column is dropped; the API computes it on the fly.

**Tech Stack:** Go, SQLite/PostgreSQL (sqlc), protobuf/ConnectRPC, React/TypeScript

**Spec:** `docs/superpowers/specs/2026-03-21-remove-machine-exposures-design.md`

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `internal/db/migrations_v2/000040_remove_machine_exposures.up.sql` | Drop machine_exposures table + machines.endpoint column |
| Create | `internal/db/hostname.go` | `MachineHostname()` and `ExtractMachineNameFromHostname()` functions |
| Create | `internal/db/hostname_test.go` | Tests for hostname functions |
| Modify | `internal/db/sqlc/schema.sql` | Remove machine_exposures table, machines.endpoint column |
| Modify | `internal/db/sqlc/query.sql` | Remove exposure queries + UpdateMachineEndpointByID, add GetMachineByName |
| Modify | `internal/db/setup_ticket_tunnel_store.go` | Remove MachineExposure struct, exposure store methods |
| Modify | `internal/db/machine_store.go` | Remove UpdateMachineEndpointByID, add GetMachineByName |
| Modify | `internal/db/machine_exposure_method.go` | Remove DomainPrefix/BaseDomain from TemplateExposureConfig |
| Modify | `proto/arca/v1/machine_template.proto` | Remove domain_prefix/base_domain from MachineExposureConfig |
| Modify | `proto/arca/v1/machine.proto` | Reserve endpoint field 6 |
| Modify | `proto/arca/v1/exposure.proto` | Remove unused fields (public, visibility, selected_user_ids, EndpointVisibility) |
| Modify | `internal/machine/worker.go` | Remove ensureMachineExposureProxyViaServer, machineSubdomain, sanitizeSubdomainPart |
| Modify | `internal/server/machine_proxy.go` | Rewrite to resolve hostname→machine via setup_state instead of exposures |
| Modify | `internal/server/exposure_connect.go` | Rewrite GetMachineExposureByHostname, UpsertMachineExposure, ListMachineExposures to work without DB table |
| Modify | `internal/server/exposure_access.go` | Change canUserAccessExposure to take machineID string |
| Modify | `internal/server/console_authorize.go` | Resolve hostname→machine instead of exposure lookup |
| Modify | `internal/server/ticket_connect.go` | Resolve hostname→machine instead of exposure lookup |
| Modify | `internal/server/machine_connect.go` | Compute endpoint dynamically in toMachineMessageWithAdmin |
| Modify | `internal/server/router.go` | Pass setup_state provider to MachineProxyHandler |
| Modify | `internal/server/machine_template_connect.go` | Stop persisting domain_prefix/base_domain in template config |
| Modify | `web/src/pages/MachineTemplateFormPage.tsx` | Remove domain prefix/base domain form fields |
| Modify | `web/src/pages/MachineTemplateDetailPage.tsx` | Remove domain prefix/base domain display |
| Modify | `web/src/lib/types.ts` | Remove domainPrefix/baseDomain from MachineExposureConfig |
| Modify | `web/src/lib/domainValidation.ts` | Remove template-related domain validation (keep setup page validation) |
| Modify | `web/src/lib/api.ts` | Remove domainPrefix/baseDomain from exposure config handling |
| Modify | `web/e2e/helpers/machine-template.ts` | Remove domainPrefix/baseDomain from test helpers |
| Modify | `web/e2e/` test files | Update E2E tests |

---

### Task 1: Add hostname utility functions

**Files:**
- Create: `internal/db/hostname.go`
- Create: `internal/db/hostname_test.go`

- [ ] **Step 1: Write tests for MachineHostname and ExtractMachineNameFromHostname**

```go
// internal/db/hostname_test.go
package db

import "testing"

func TestMachineHostname(t *testing.T) {
	tests := []struct {
		prefix, name, baseDomain, want string
	}{
		{"arca", "myvm", "example.com", "arcamyvm.example.com"},
		{"", "myvm", "example.com", "myvm.example.com"},
		{"dev-", "test-1", "app.io", "dev-test-1.app.io"},
	}
	for _, tt := range tests {
		got := MachineHostname(tt.prefix, tt.name, tt.baseDomain)
		if got != tt.want {
			t.Errorf("MachineHostname(%q, %q, %q) = %q, want %q", tt.prefix, tt.name, tt.baseDomain, got, tt.want)
		}
	}
}

func TestExtractMachineNameFromHostname(t *testing.T) {
	tests := []struct {
		hostname, prefix, baseDomain string
		wantName                     string
		wantOK                       bool
	}{
		{"arcamyvm.example.com", "arca", "example.com", "myvm", true},
		{"myvm.example.com", "", "example.com", "myvm", true},
		{"dev-test-1.app.io", "dev-", "app.io", "test-1", true},
		{"unrelated.other.com", "arca", "example.com", "", false},
		{"arca.example.com", "arca", "example.com", "", false}, // empty name
		{"arcamyvm.example.com", "arca", "other.com", "", false},
	}
	for _, tt := range tests {
		name, ok := ExtractMachineNameFromHostname(tt.hostname, tt.prefix, tt.baseDomain)
		if ok != tt.wantOK || name != tt.wantName {
			t.Errorf("ExtractMachineNameFromHostname(%q, %q, %q) = (%q, %v), want (%q, %v)",
				tt.hostname, tt.prefix, tt.baseDomain, name, ok, tt.wantName, tt.wantOK)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go test ./internal/db/ -run 'TestMachineHostname|TestExtractMachineNameFromHostname' -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement hostname functions**

```go
// internal/db/hostname.go
package db

import "strings"

// MachineHostname computes the full hostname for a machine from setup_state config.
// Format: {prefix}{machineName}.{baseDomain}
func MachineHostname(prefix, machineName, baseDomain string) string {
	return prefix + machineName + "." + baseDomain
}

// ExtractMachineNameFromHostname parses a hostname and extracts the machine name
// by stripping the base_domain suffix and domain_prefix prefix.
// Returns ("", false) if the hostname does not match the expected pattern.
func ExtractMachineNameFromHostname(hostname, prefix, baseDomain string) (string, bool) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	suffix := "." + baseDomain
	if !strings.HasSuffix(hostname, suffix) {
		return "", false
	}
	subdomain := strings.TrimSuffix(hostname, suffix)
	if !strings.HasPrefix(subdomain, prefix) {
		return "", false
	}
	name := strings.TrimPrefix(subdomain, prefix)
	if name == "" {
		return "", false
	}
	return name, true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go test ./internal/db/ -run 'TestMachineHostname|TestExtractMachineNameFromHostname' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/hostname.go internal/db/hostname_test.go
git commit -m "Add MachineHostname and ExtractMachineNameFromHostname utilities"
```

---

### Task 2: DB migration and schema changes

**Files:**
- Create: `internal/db/migrations_v2/000040_remove_machine_exposures.up.sql`
- Modify: `internal/db/sqlc/schema.sql`
- Modify: `internal/db/sqlc/query.sql`

- [ ] **Step 1: Create migration file**

```sql
-- internal/db/migrations_v2/000040_remove_machine_exposures.up.sql
DROP TABLE IF EXISTS machine_exposures;

-- SQLite does not support DROP COLUMN directly. We recreate the machines table
-- without the endpoint column. For PostgreSQL, use ALTER TABLE DROP COLUMN.
-- Since sqlc generates for both, we handle this at the application level.
-- For SQLite migration, we create a new table without endpoint and copy data.
CREATE TABLE machines_new (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  template_id TEXT NOT NULL DEFAULT 'libvirt',
  template_type TEXT NOT NULL DEFAULT '',
  template_config_json TEXT NOT NULL DEFAULT '{}',
  setup_version TEXT NOT NULL DEFAULT '',
  options_json TEXT NOT NULL DEFAULT '{}',
  created_at BIGINT NOT NULL DEFAULT 0,
  updated_at BIGINT NOT NULL DEFAULT 0
);

INSERT INTO machines_new (id, name, template_id, template_type, template_config_json, setup_version, options_json, created_at, updated_at)
SELECT id, name, template_id, template_type, template_config_json, setup_version, options_json, created_at, updated_at FROM machines;

DROP TABLE machines;
ALTER TABLE machines_new RENAME TO machines;
```

Note: Check the full machines table schema before writing the migration — include ALL columns except endpoint. The above is a template; the implementer must read the actual schema from `schema.sql` lines 73-84 and include all columns (ssh_public_keys_json etc.) in the recreation.

- [ ] **Step 2: Update schema.sql — remove machine_exposures table and machines.endpoint**

Remove lines 188-200 (machine_exposures table and index). Remove `endpoint TEXT NOT NULL DEFAULT ''` from the machines table definition (line 80).

- [ ] **Step 3: Update query.sql — remove exposure queries and endpoint update**

Remove these named queries:
- `UpsertMachineExposure` (lines 569-583)
- `ListMachineExposuresByMachineID` (lines 585-589)
- `GetMachineExposureByHostname` (lines 591-595)
- `GetMachineExposureByMachineIDAndName` (lines 597-602)
- `UpdateMachineEndpointByID` (lines 234-237)

Add new query:
```sql
-- name: GetMachineByName :one
SELECT id, name, template_id, template_type, template_config_json, setup_version, options_json, created_at, updated_at
FROM machines
WHERE name = sqlc.arg(name)
LIMIT 1;
```

Also remove `endpoint` from all SELECT columns in existing machine queries (grep for `endpoint` in query.sql machine-related queries).

- [ ] **Step 4: Regenerate sqlc**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make sqlc`
Expected: Generated files updated without errors

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations_v2/000040_remove_machine_exposures.up.sql internal/db/sqlc/
git commit -m "Add migration to drop machine_exposures table and machines.endpoint column"
```

---

### Task 3: Update Go DB store layer

**Files:**
- Modify: `internal/db/setup_ticket_tunnel_store.go` — remove MachineExposure struct, exposure methods
- Modify: `internal/db/machine_store.go` — remove UpdateMachineEndpointByID, add GetMachineByName
- Modify: `internal/db/machine_exposure_method.go` — remove DomainPrefix/BaseDomain from TemplateExposureConfig
- Modify: `internal/db/arcad_session_store.go` — exposure_id becomes always empty string (backward compat, no schema change needed for arcad_exchange_tokens/arcad_sessions)

- [ ] **Step 1: Remove MachineExposure struct and store methods**

In `internal/db/setup_ticket_tunnel_store.go`:
- Remove the `MachineExposure` struct (lines 50-58)
- Remove methods: `UpsertMachineExposure`, `ListMachineExposuresByMachineID`, `GetMachineExposureByMachineIDAndName`, `GetMachineExposureByHostname`, and helper conversion functions (`toMachineExposure`, `toMachineExposurePG`)

- [ ] **Step 2: Remove UpdateMachineEndpointByID, add GetMachineByName in machine_store.go**

In `internal/db/machine_store.go`:
- Remove `UpdateMachineEndpointByID` method (lines 732-748)
- Add `GetMachineByName` method wrapping the new sqlc query
- Remove `Endpoint` field references from any Machine struct mapping (the field no longer exists after sqlc regeneration)

- [ ] **Step 3: Simplify TemplateExposureConfig**

In `internal/db/machine_exposure_method.go`:
- Remove `DomainPrefix` and `BaseDomain` fields from `TemplateExposureConfig` struct (lines 17-18)
- Keep `Method` and `Connectivity` fields

- [ ] **Step 4: Build to verify compilation**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./...`
Expected: May have compilation errors in server/ and machine/ packages — that's expected, will fix in subsequent tasks

- [ ] **Step 5: Commit**

```bash
git add internal/db/
git commit -m "Remove MachineExposure DB layer and machines.endpoint references"
```

---

### Task 4: Update protobuf definitions

**Files:**
- Modify: `proto/arca/v1/machine_template.proto`
- Modify: `proto/arca/v1/machine.proto`
- Modify: `proto/arca/v1/exposure.proto`

- [ ] **Step 1: Remove domain_prefix/base_domain from MachineExposureConfig**

In `proto/arca/v1/machine_template.proto`, change `MachineExposureConfig`:
```protobuf
message MachineExposureConfig {
  MachineExposureMethod method = 1;
  reserved 2; // was domain_prefix
  reserved 3; // was base_domain
  reserved 4, 5, 6;
  MachineConnectivity connectivity = 7;
}
```

- [ ] **Step 2: Reserve endpoint field in Machine message**

In `proto/arca/v1/machine.proto`, replace `string endpoint = 6;` with `reserved 6; // was endpoint`.

- [ ] **Step 3: Remove unused fields from exposure.proto**

In `proto/arca/v1/exposure.proto`:
- Remove `EndpointVisibility` enum (lines 15-21)
- In `MachineExposure` message, reserve removed fields:
```protobuf
message MachineExposure {
  string id = 1;
  string machine_id = 2;
  string name = 3;
  string hostname = 4;
  string service = 5;
  reserved 6; // was public
  reserved 7; // was visibility
  reserved 8; // was selected_user_ids
}
```
- In `UpsertMachineExposureRequest`, reserve removed fields:
```protobuf
message UpsertMachineExposureRequest {
  string machine_id = 1;
  reserved 2;
  string name = 3;
  reserved 4; // was public
  reserved 5; // was visibility
  reserved 6; // was selected_user_ids
}
```

- [ ] **Step 4: Regenerate proto code**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make proto`
Expected: Generated code updated

- [ ] **Step 5: Commit**

```bash
git add proto/ internal/gen/ web/src/gen/
git commit -m "Remove domain_prefix/base_domain from proto, reserve endpoint field"
```

---

### Task 5: Rewrite machine proxy handler

**Files:**
- Modify: `internal/server/machine_proxy.go`
- Modify: `internal/server/router.go`

- [ ] **Step 1: Add setup_state provider to MachineProxyHandler**

Update `MachineProxyHandler` struct to hold a `store` reference (it already does) and use `store.GetSetupState()` for hostname resolution. The setup state should be cached. Either add an in-memory cache with TTL, or fetch per-request if the DB call is lightweight.

Rewrite `MachineProxyHandler`:
```go
type MachineProxyHandler struct {
	store   *db.Store
	ipCache *machine.MachineIPCache
}
```

- [ ] **Step 2: Rewrite IsMachineProxyRequest**

```go
func (h *MachineProxyHandler) IsMachineProxyRequest(r *http.Request) bool {
	if h == nil || h.store == nil {
		return false
	}
	hostname := extractHostname(r.Host)
	if hostname == "" {
		return false
	}
	setup, err := h.store.GetSetupState(r.Context())
	if err != nil || setup.BaseDomain == "" {
		return false
	}
	name, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
	if !ok {
		return false
	}
	_, err = h.store.GetMachineByName(r.Context(), name)
	return err == nil
}
```

- [ ] **Step 3: Rewrite TryServeHTTP**

Replace exposure lookup with hostname→machine name→machine lookup. Remove `resolveUpstreamURL`'s exposure parameter and legacy endpoint fallback. Use constant arcad port (21030).

```go
func (h *MachineProxyHandler) TryServeHTTP(w http.ResponseWriter, r *http.Request) bool {
	// ... nil checks, extract hostname ...
	setup, err := h.store.GetSetupState(r.Context())
	// ... error handling ...
	name, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
	if !ok {
		return false
	}
	m, err := h.store.GetMachineByName(r.Context(), name)
	// ... error handling, resolve upstream IP, proxy ...
}
```

- [ ] **Step 4: Simplify resolveUpstreamURL**

Remove the `exposure db.MachineExposure` parameter. Use constant port 21030. Remove legacy endpoint fallback.

```go
const arcadListenPort = "21030"

func resolveUpstreamURL(info *machine.RuntimeMachineInfo, connectivity string) string {
	var ip string
	if info != nil {
		conn := strings.ToLower(strings.TrimSpace(connectivity))
		switch {
		case conn == "public_ip" || strings.HasSuffix(conn, "_public_ip"):
			ip = info.PublicIP
			if ip == "" {
				ip = info.PrivateIP
			}
		default:
			ip = info.PrivateIP
		}
	}
	if ip == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(ip, arcadListenPort)
}
```

- [ ] **Step 5: Build to verify**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./internal/server/`
Expected: May still have errors in other files — focus on machine_proxy.go compiling

- [ ] **Step 6: Commit**

```bash
git add internal/server/machine_proxy.go internal/server/router.go
git commit -m "Rewrite machine proxy to resolve hostname via setup_state"
```

---

### Task 6: Rewrite exposure connect service

**Files:**
- Modify: `internal/server/exposure_connect.go`

The ExposureService RPC must remain for backward compatibility with older arcad. But the implementation changes from DB lookups to dynamic computation.

- [ ] **Step 1: Rewrite GetMachineExposureByHostname**

Instead of DB lookup, parse the hostname to extract machine name, look up machine by name, construct response dynamically:

```go
func (s *exposureConnectService) GetMachineExposureByHostname(ctx context.Context, req *connect.Request[arcav1.GetMachineExposureByHostnameRequest]) (*connect.Response[arcav1.GetMachineExposureByHostnameResponse], error) {
	// ... auth checks (machine token / machine ID) ...
	hostname := strings.TrimSpace(req.Msg.GetHostname())
	setup, err := s.store.GetSetupState(ctx)
	// ... error handling ...
	name, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("exposure not found"))
	}
	m, err := s.store.GetMachineByName(ctx, name)
	// ... error handling, verify machine ID matches ...
	return connect.NewResponse(&arcav1.GetMachineExposureByHostnameResponse{
		Exposure: &arcav1.MachineExposure{
			Id:       m.ID, // use machine ID as exposure ID for compat
			MachineId: m.ID,
			Name:     "default",
			Hostname: hostname,
			Service:  "http://localhost:21030",
		},
	}), nil
}
```

- [ ] **Step 2: Simplify UpsertMachineExposure**

This is called by the UI. Without the DB table, it becomes a no-op that validates the machine exists and returns a dynamically constructed exposure:

```go
func (s *exposureConnectService) UpsertMachineExposure(...) (...) {
	// ... auth, validate machine_id ...
	setup, _ := s.store.GetSetupState(ctx)
	m, err := s.store.GetMachineByID(ctx, machineID)
	// ... error handling ...
	hostname := db.MachineHostname(setup.DomainPrefix, m.Name, setup.BaseDomain)
	return connect.NewResponse(&arcav1.UpsertMachineExposureResponse{
		Exposure: &arcav1.MachineExposure{
			Id: m.ID, MachineId: m.ID, Name: "default",
			Hostname: hostname, Service: "http://localhost:21030",
		},
	}), nil
}
```

- [ ] **Step 3: Simplify ListMachineExposures**

Return a single dynamically constructed exposure for the machine:

```go
func (s *exposureConnectService) ListMachineExposures(...) (...) {
	// ... auth, validate machine_id ...
	setup, _ := s.store.GetSetupState(ctx)
	m, err := s.store.GetMachineByID(ctx, machineID)
	// ... error handling ...
	hostname := db.MachineHostname(setup.DomainPrefix, m.Name, setup.BaseDomain)
	return connect.NewResponse(&arcav1.ListMachineExposuresResponse{
		Exposures: []*arcav1.MachineExposure{{
			Id: m.ID, MachineId: m.ID, Name: "default",
			Hostname: hostname, Service: "http://localhost:21030",
		}},
	}), nil
}
```

- [ ] **Step 4: Build to verify**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./internal/server/`

- [ ] **Step 5: Commit**

```bash
git add internal/server/exposure_connect.go
git commit -m "Rewrite exposure connect service to compute exposures dynamically"
```

---

### Task 7: Update console_authorize, ticket_connect, and exposure_access

**Files:**
- Modify: `internal/server/console_authorize.go`
- Modify: `internal/server/ticket_connect.go`
- Modify: `internal/server/exposure_access.go`

- [ ] **Step 1: Change canUserAccessExposure to take machineID**

In `internal/server/exposure_access.go`, change signature from `exposure db.MachineExposure` to `machineID string`:

```go
func canUserAccessMachine(ctx context.Context, store *db.Store, machineID, userID, targetPath string) bool {
	role := store.ResolveMachineRole(ctx, userID, machineID)
	if role == db.MachineRoleNone {
		return false
	}
	if isPrivilegedArcaPath(targetPath) {
		return role == db.MachineRoleAdmin || role == db.MachineRoleEditor
	}
	return true
}
```

- [ ] **Step 2: Update console_authorize.go**

Replace exposure lookup with hostname→machine resolution:

```go
// Replace:
//   exposure, err := store.GetMachineExposureByHostname(...)
// With:
setup, err := store.GetSetupState(r.Context())
// ... error handling ...
machineName, ok := db.ExtractMachineNameFromHostname(exposureHost, setup.DomainPrefix, setup.BaseDomain)
if !ok {
	http.NotFound(w, r)
	return
}
m, err := store.GetMachineByName(r.Context(), machineName)
// ... error handling ...

if !canUserAccessMachine(r.Context(), store, m.ID, userID, targetURL.Path) {
	// ... redirect to access-denied, use m.ID ...
}
token, err := store.CreateArcadExchangeToken(r.Context(), userID, m.ID, "" /* no exposure ID */, expiresAt.Unix())
```

- [ ] **Step 3: Update ticket_connect.go ValidateArcadSession**

Replace exposure lookup with hostname→machine resolution:

```go
// Replace:
//   exposure, err := s.store.GetMachineExposureByHostname(ctx, hostname)
// With:
setup, err := s.store.GetSetupState(ctx)
// ... error handling ...
machineName, ok := db.ExtractMachineNameFromHostname(hostname, setup.DomainPrefix, setup.BaseDomain)
if !ok {
	return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
}
resolvedMachine, err := s.store.GetMachineByName(ctx, machineName)
// ... error handling ...
if resolvedMachine.ID != machineID {
	return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid session"))
}
// Skip exposure ID check (session.ExposureID) — it's legacy
if !canUserAccessMachine(ctx, s.store, machineID, session.UserID, targetPath) {
	// ...
}
```

- [ ] **Step 4: Build to verify**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./internal/server/`

- [ ] **Step 5: Commit**

```bash
git add internal/server/console_authorize.go internal/server/ticket_connect.go internal/server/exposure_access.go
git commit -m "Update auth flows to resolve hostname via setup_state instead of exposures"
```

---

### Task 8: Update worker and machine_connect

**Files:**
- Modify: `internal/machine/worker.go`
- Modify: `internal/server/machine_connect.go`

- [ ] **Step 1: Remove exposure functions from worker.go**

- Remove `machineSubdomain()` function (lines 506-538)
- Remove `sanitizeSubdomainPart()` function (lines 540-549)
- Remove `ensureMachineExposureProxyViaServer()` method (lines 558-583)
- In the `handleStart` method, remove the call to `ensureMachineExposureProxyViaServer` (line 381) and the corresponding event emission (line 384)

- [ ] **Step 2: Update toMachineMessageWithAdmin in machine_connect.go**

Remove the `Endpoint: machine.Endpoint` line (line 496). The proto field is now reserved so the generated struct won't have it.

If the UI still needs the endpoint, compute it dynamically. However, since the proto field is reserved, the UI must get it another way (e.g., compute client-side from machine name + setup state, or add a computed field). For now, simply remove the line — the frontend will be updated to compute it.

- [ ] **Step 3: Build the full project**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./...`
Expected: PASS (all Go compilation errors resolved)

- [ ] **Step 4: Commit**

```bash
git add internal/machine/worker.go internal/server/machine_connect.go
git commit -m "Remove worker exposure logic and endpoint field from machine response"
```

---

### Task 9: Update template proto handling and connect service

**Files:**
- Modify: `internal/server/machine_template_connect.go`

- [ ] **Step 1: Stop persisting domain_prefix/base_domain in template config**

In `machine_template_connect.go`, in the template validation function (around line 282-291), stop setting DomainPrefix and BaseDomain on the exposure config:

```go
// Before:
exposureConfig = &arcav1.MachineExposureConfig{
	Method:       exp.GetMethod(),
	DomainPrefix: strings.ToLower(strings.TrimSpace(exp.GetDomainPrefix())),
	BaseDomain:   strings.ToLower(strings.TrimSpace(exp.GetBaseDomain())),
	Connectivity: exp.GetConnectivity(),
}

// After:
exposureConfig = &arcav1.MachineExposureConfig{
	Method:       exp.GetMethod(),
	Connectivity: exp.GetConnectivity(),
}
```

- [ ] **Step 2: Build to verify**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go build ./internal/server/`

- [ ] **Step 3: Commit**

```bash
git add internal/server/machine_template_connect.go
git commit -m "Stop persisting domain_prefix/base_domain in template config"
```

---

### Task 10: Frontend — remove domain fields from template form

**Files:**
- Modify: `web/src/pages/MachineTemplateFormPage.tsx`
- Modify: `web/src/pages/MachineTemplateDetailPage.tsx`
- Modify: `web/src/lib/types.ts`
- Modify: `web/src/lib/domainValidation.ts`
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Remove domain fields from types.ts**

Remove `domainPrefix` and `baseDomain` from `MachineExposureConfig` type. Keep `method` and `connectivity`.

- [ ] **Step 2: Remove template domain validation from domainValidation.ts**

The `validateBaseDomainInput`, `normalizeBaseDomainInput`, `validateDomainPrefixInput`, `normalizeDomainPrefixInput` functions may still be needed for the setup page. Check if the setup page imports them; if so, keep them. If only the template form used them, remove.

- [ ] **Step 3: Remove domain fields from MachineTemplateFormPage.tsx**

- Remove `exposureDomainPrefix` and `exposureBaseDomain` from form state type and defaults
- Remove the "Domain prefix" and "Base domain" input fields from the JSX
- Remove from `toExposureConfig` and form initialization

- [ ] **Step 4: Remove domain display from MachineTemplateDetailPage.tsx**

Remove the lines showing domain prefix and base domain (around lines 152-153).

- [ ] **Step 5: Update api.ts**

Remove `domainPrefix` and `baseDomain` from exposure config handling in template serialization/deserialization.

- [ ] **Step 6: Update App.tsx**

Remove `baseDomain` and `domainPrefix` from any default exposure config objects.

- [ ] **Step 7: Build frontend**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make build-frontend`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add web/src/
git commit -m "Remove domain prefix/base domain fields from template UI"
```

---

### Task 11: Frontend — compute endpoint URL from setup state

**Files:**
- Modify: `web/src/pages/MachineDetailPage.tsx`
- Modify: `web/src/pages/MachinesPage.tsx`
- Possibly: `web/src/lib/api.ts` or `web/src/App.tsx`

The UI currently reads `machine.endpoint` from the API response to build links like `https://{endpoint}`. Since the proto field is now reserved, the frontend needs another way to get the endpoint.

Options:
1. Add the setup_state's `domainPrefix`/`baseDomain` to the frontend context (already available from the setup API) and compute `{prefix}{machineName}.{baseDomain}` client-side.
2. Add a new computed field to the Machine proto message.

Option 1 is preferred (no proto change needed).

- [ ] **Step 1: Ensure setup state is available in frontend context**

Check if the setup state (with domain prefix / base domain) is already loaded in the app context. If so, create a helper function:

```typescript
export function machineEndpointURL(prefix: string, machineName: string, baseDomain: string): string {
  return `https://${prefix}${machineName}.${baseDomain}`
}
```

- [ ] **Step 2: Update MachineDetailPage.tsx**

Replace `machine.endpoint` usage with computed hostname from setup state.

- [ ] **Step 3: Update MachinesPage.tsx**

Replace `machine.endpoint` usage with computed hostname from setup state.

- [ ] **Step 4: Build frontend**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make build-frontend`

- [ ] **Step 5: Commit**

```bash
git add web/src/
git commit -m "Compute machine endpoint URL client-side from setup state"
```

---

### Task 12: Update E2E tests

**Files:**
- Modify: `web/e2e/helpers/machine-template.ts`
- Modify: `web/e2e/helpers/auth.ts`
- Modify: `web/e2e/template-catalog.spec.ts`
- Modify: `web/e2e/machine-options.spec.ts`
- Modify: `web/e2e/lxd-provisioning.spec.ts`

- [ ] **Step 1: Remove domainPrefix/baseDomain from E2E test helpers**

In `web/e2e/helpers/machine-template.ts`:
- Remove `domainPrefix` and `baseDomain` from template creation helper config
- Remove from function parameter types

In `web/e2e/helpers/auth.ts`:
- Keep `baseDomain` and `domainPrefix` in setup config (it's for setup_state, which remains)

- [ ] **Step 2: Update E2E test specs**

In `web/e2e/template-catalog.spec.ts`:
- Remove domain prefix/base domain form fill steps (lines ~155-156)

In `web/e2e/machine-options.spec.ts`:
- Remove domainPrefix/baseDomain from template config objects

In `web/e2e/lxd-provisioning.spec.ts`:
- Remove domainPrefix/baseDomain from template config

- [ ] **Step 3: Run fast E2E tests**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures/web && npx playwright test --project=fast`

- [ ] **Step 4: Commit**

```bash
git add web/e2e/
git commit -m "Update E2E tests to remove domain prefix/base domain from templates"
```

---

### Task 13: Run full test suite and fix issues

- [ ] **Step 1: Run Go tests**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make test`

Fix any failures. Common issues:
- Tests in `internal/server/exposure_connect_test.go` that create exposures via DB
- Tests in `internal/server/setup_connect_test.go` for domain validation
- Worker tests that call ensureMachineExposureProxyViaServer

- [ ] **Step 2: Run full test suite**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make test`
Expected: PASS

- [ ] **Step 3: Commit any test fixes**

```bash
git add -A
git commit -m "Fix test compilation and failures after exposure removal"
```

---

### Task 14: Clean up exposure_connect_test.go

**Files:**
- Modify: `internal/server/exposure_connect_test.go`

- [ ] **Step 1: Update tests**

The existing tests in `exposure_connect_test.go` create machines and then test ReportMachineReadiness and similar flows. They use `UpsertMachineExposure` to set up test state. Since exposures are now dynamic, these tests need to:
- Remove direct exposure creation calls
- Set up `setup_state` with base_domain/prefix instead
- Verify that the dynamic exposure lookup works

- [ ] **Step 2: Run tests**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && go test ./internal/server/ -v`

- [ ] **Step 3: Commit**

```bash
git add internal/server/exposure_connect_test.go
git commit -m "Update exposure connect tests for dynamic exposure computation"
```

---

### Task 15: Final verification and cleanup

- [ ] **Step 1: Run full build**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make build`
Expected: PASS

- [ ] **Step 2: Run full test suite**

Run: `cd /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures && make test`
Expected: PASS

- [ ] **Step 3: Verify no remaining references to removed items**

```bash
grep -r 'machine_exposures\|MachineExposure\b\|machineSubdomain\|sanitizeSubdomainPart\|UpdateMachineEndpointByID' \
  --include='*.go' --include='*.ts' --include='*.tsx' --include='*.sql' --include='*.proto' \
  --exclude-dir=node_modules --exclude-dir=gen --exclude-dir=dist \
  /home/r-arai.linux/work/arca/.claude/worktrees/ryotarai/remove-machine-exposures/
```

Expected: Only references in generated files, migration files, or proto reserved comments.

- [ ] **Step 4: Final commit if needed**

```bash
git add -A
git commit -m "Final cleanup: remove stale exposure references"
```
