# Remove machine_exposures Table and Centralize Domain Config

## Summary

Remove the `machine_exposures` table and eliminate duplicated domain configuration (prefix, base_domain) from machine templates and `machines.endpoint`. The `setup_state` table becomes the single source of truth for domain settings. Hostnames are computed dynamically as `{prefix}{machine_name}.{base_domain}`.

## Motivation

Currently, `domain_prefix` and `base_domain` are stored/derived in multiple places:

1. `setup_state` table (admin config)
2. Machine template `config_json` (`MachineExposureConfig`)
3. `machine_exposures.hostname` (precomputed full hostname)
4. `machines.endpoint` (copy of hostname written by worker)

If an admin changes the prefix or base domain, the precomputed hostnames in `machine_exposures` and `machines.endpoint`, as well as template-embedded values, become stale. The `machine_exposures` table adds no unique data beyond what can be derived from the machine name and admin config.

Additionally, the `service` field (`http://localhost:21030`) is always the hardcoded arcad port and should not be stored.

## Design

### What Gets Removed

1. **`machine_exposures` table** — drop table, remove all queries (`UpsertMachineExposure`, `ListMachineExposuresByMachineID`, `GetMachineExposureByHostname`, `GetMachineExposureByMachineIDAndName`), remove migration.
2. **`MachineExposureConfig.domain_prefix` / `base_domain`** — remove from `proto/arca/v1/machine_template.proto`, from `TemplateExposureConfig` Go struct, and from the template form UI.
3. **`machineSubdomain()` / `sanitizeSubdomainPart()`** — remove sanitization logic from `worker.go`. Machine names must be validated at creation time instead.
4. **`exposure.proto` unused fields** — remove `public`, `visibility`, `selected_user_ids` from `MachineExposure` message. Remove `EndpointVisibility` enum. These were never implemented in the DB or server.
5. **`db.MachineExposure` struct** — remove from `setup_ticket_tunnel_store.go`.
6. **`machines.endpoint` hostname writes** — stop writing hostname to `machines.endpoint` in worker. This field must not contain prefix/base_domain-derived values.

### What Changes

#### Machine Name Validation

Enforce subdomain-safe names at machine creation time:
- Lowercase ASCII letters, digits, and hyphens only
- Must not start or end with a hyphen
- Max 63 characters
- Regex: `^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`

This replaces the runtime sanitization in `machineSubdomain()`.

#### Hostname Computation

A shared function computes the hostname dynamically:

```go
func MachineHostname(prefix, machineName, baseDomain string) string {
    return prefix + machineName + "." + baseDomain
}
```

No sanitization — the machine name is already validated at creation.

#### Proxy Lookup (machine_proxy.go)

Current flow:
```
Host header → GetMachineExposureByHostname(hostname) → exposure.MachineID → machine
```

New flow:
```
Host header → strip base_domain suffix → strip prefix → GetMachineByName(name) → machine
```

- `setup_state` (base_domain, prefix) is cached in-memory with periodic refresh (reuse existing `SetupState` caching if available, or add a simple TTL cache).
- If the host doesn't match the base_domain suffix, return false (not a machine proxy request).

#### Worker (worker.go)

`ensureMachineExposureProxyViaServer()` simplifies dramatically — the exposure upsert and endpoint write are both removed. The function may be eliminated entirely or reduced to a validation check that setup_state has base_domain configured.

#### Machine API Response (endpoint field)

The `machines.endpoint` DB column is no longer written by the worker. Instead, the API layer (`machine_connect.go`) computes the hostname dynamically when building the response:

```go
// In toMachineProto() or equivalent
endpoint := MachineHostname(setupState.DomainPrefix, machine.Name, setupState.BaseDomain)
```

The UI continues to work without changes — it receives the computed hostname in the API response.

#### arcad `GetMachineExposureByHostname` RPC

This RPC is called by arcad to resolve a hostname to a machine exposure. It must remain functional for backward compatibility with older arcad versions.

Changes:
- Instead of querying `machine_exposures`, parse the hostname to extract the machine name (strip base_domain, strip prefix).
- Look up the machine by name.
- Construct and return a `MachineExposure` proto message with dynamically computed hostname and service.

#### Template Form UI

- Remove "Domain prefix" and "Base domain" input fields from `MachineTemplateFormPage.tsx`.
- Remove validation logic in `domainValidation.ts` related to these fields (or keep only for setup page).
- Template detail page: remove display of domain prefix / base domain.

#### Setup Page

The setup page already manages `base_domain` and `domain_prefix` in `setup_state`. No changes needed here — this remains the single source of truth.

### Arcad Port

The arcad listen port (21030) becomes a constant in the proxy resolution logic, replacing the stored `service` field. Defined in a shared location (e.g., `internal/machine/constants.go` or inline in proxy code).

### Backward Compatibility

- **Older arcad versions** calling `GetMachineExposureByHostname`: still works — the server resolves hostname to machine dynamically and returns a constructed `MachineExposure` response.
- **Existing template `config_json`**: `domain_prefix` and `base_domain` fields in stored JSON are silently ignored. No migration needed for the JSON content.
- **`MachineExposure` proto message**: retained with `hostname` and `service` fields for API compatibility, but values are always computed server-side.

### Migration

- Add a migration to drop the `machine_exposures` table and its index.
- No data migration needed — all exposure data is derivable.

## Out of Scope

- Per-exposure visibility/access control (not yet implemented, can be added later on machines table or a new table if needed)
- Multiple exposures per machine (currently always "default"; can revisit if needed)
- Removing `machines.endpoint` column itself (may still be useful for other purposes; just stop writing hostname to it)
