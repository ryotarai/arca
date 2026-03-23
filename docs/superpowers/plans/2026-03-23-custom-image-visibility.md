# Custom Image Visibility & User Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ownership and visibility (private/shared) to custom images so users can manage their own images and admins control sharing.

**Architecture:** Add `created_by_user_id` and `visibility` columns to `custom_images`. Extend all image RPCs with role-based scoping: regular users see/edit their own private images + shared images; admins see/edit all. Add a user-facing "My Images" page.

**Tech Stack:** Go, SQLite/PostgreSQL (sqlc), ConnectRPC (protobuf), React + shadcn/ui + Tailwind

**Spec:** `docs/superpowers/specs/2026-03-23-custom-image-visibility-design.md`

---

## File Map

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/db/migrations_v2/000051_custom_image_visibility.up.sql` | Migration: add columns + index |
| Create | `internal/db/migrations_v2/000051_custom_image_visibility.down.sql` | Rollback migration |
| Modify | `internal/db/sqlc/schema.sql:254-264` | Update schema for sqlc codegen |
| Modify | `internal/db/sqlc/query.sql:907-984` | Add/update custom image queries |
| Modify | `internal/db/custom_image_store.go` | Update struct + store functions |
| Modify | `proto/arca/v1/image.proto` | Add fields to CustomImage + UpdateRequest |
| Modify | `internal/server/image_connect.go` | Permission logic for all image RPCs |
| Create | `internal/server/image_connect_test.go` | Permission tests for image handlers |
| Modify | `internal/server/machine_connect.go:629-632` | Pass user ID in job metadata |
| Modify | `internal/machine/image_job.go:39-44,78-80` | Read user ID from metadata, pass to store |
| Modify | `web/src/lib/api.ts` | Update CustomImage type + API functions |
| Modify | `web/src/pages/CustomImagesPage.tsx` | Add visibility/created_by columns + toggle |
| Create | `web/src/pages/MyImagesPage.tsx` | New user-facing images page |
| Modify | `web/src/pages/AppLayout.tsx:29-32` | Add "Images" nav item for all users |
| Modify | `web/src/App.tsx:22,183` | Add route for /images |

---

### Task 1: Database Migration

**Files:**
- Create: `internal/db/migrations_v2/000051_custom_image_visibility.up.sql`
- Create: `internal/db/migrations_v2/000051_custom_image_visibility.down.sql`
- Modify: `internal/db/sqlc/schema.sql:254-264`

- [ ] **Step 1: Create up migration file**

```sql
-- 000051_custom_image_visibility.up.sql
ALTER TABLE custom_images ADD COLUMN created_by_user_id TEXT NOT NULL DEFAULT '';
ALTER TABLE custom_images ADD COLUMN visibility TEXT NOT NULL DEFAULT 'private';

-- Existing images become shared so they remain accessible after migration
UPDATE custom_images SET visibility = 'shared' WHERE visibility = 'private';

CREATE INDEX IF NOT EXISTS idx_custom_images_visibility_user ON custom_images(visibility, created_by_user_id);
```

- [ ] **Step 1b: Create down migration file**

```sql
-- 000051_custom_image_visibility.down.sql
DROP INDEX IF EXISTS idx_custom_images_visibility_user;
ALTER TABLE custom_images DROP COLUMN visibility;
ALTER TABLE custom_images DROP COLUMN created_by_user_id;
```

- [ ] **Step 2: Update sqlc schema**

In `internal/db/sqlc/schema.sql`, update the `custom_images` table definition to include the new columns (between `source_machine_id` and `created_at`):

```sql
CREATE TABLE IF NOT EXISTS custom_images (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider_type TEXT NOT NULL,
  data_json TEXT NOT NULL DEFAULT '{}',
  description TEXT NOT NULL DEFAULT '',
  source_machine_id TEXT,
  created_by_user_id TEXT NOT NULL DEFAULT '',
  visibility TEXT NOT NULL DEFAULT 'private',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(name, provider_type)
);
```

Add the index after the `profile_custom_images` index:

```sql
CREATE INDEX IF NOT EXISTS idx_custom_images_visibility_user ON custom_images(visibility, created_by_user_id);
```

- [ ] **Step 3: Commit**

```bash
git add internal/db/migrations_v2/000051_custom_image_visibility.up.sql internal/db/sqlc/schema.sql
git commit -m "Add created_by_user_id and visibility columns to custom_images"
```

---

### Task 2: SQL Queries & sqlc Codegen

**Files:**
- Modify: `internal/db/sqlc/query.sql:907-984`

- [ ] **Step 1: Update existing queries to include new columns**

All SELECT queries for custom_images must now include `created_by_user_id` and `visibility` in their column list. Update these queries:

- `ListCustomImages` (line 907)
- `ListCustomImagesByRuntimeType` (line 912)
- `GetCustomImage` (line 918)
- `GetCustomImageByNameAndProviderType` (line 924)
- `ListCustomImagesByProfileID` (line 959)

For each, add `created_by_user_id, visibility` to the SELECT column list. Example for `ListCustomImages`:

```sql
-- name: ListCustomImages :many
SELECT id, name, provider_type, data_json, description, source_machine_id, created_by_user_id, visibility, created_at, updated_at
FROM custom_images
ORDER BY created_at DESC;
```

- [ ] **Step 2: Update CreateCustomImage to include new columns**

```sql
-- name: CreateCustomImage :exec
INSERT INTO custom_images (id, name, provider_type, data_json, description, created_by_user_id, visibility, created_at, updated_at)
VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(provider_type),
  sqlc.arg(data_json),
  sqlc.arg(description),
  sqlc.arg(created_by_user_id),
  sqlc.arg(visibility),
  sqlc.arg(created_at),
  sqlc.arg(updated_at)
);
```

- [ ] **Step 3: Update InsertCustomImageWithSource to include created_by_user_id**

```sql
-- name: InsertCustomImageWithSource :exec
INSERT INTO custom_images (id, name, provider_type, data_json, description, source_machine_id, created_by_user_id, visibility, created_at, updated_at)
VALUES (sqlc.arg(id), sqlc.arg(name), sqlc.arg(provider_type), sqlc.arg(data_json), sqlc.arg(description), sqlc.arg(source_machine_id), sqlc.arg(created_by_user_id), 'private', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
```

- [ ] **Step 4: Update UpdateCustomImage to support visibility**

```sql
-- name: UpdateCustomImage :execrows
UPDATE custom_images
SET name = sqlc.arg(name),
    provider_type = sqlc.arg(provider_type),
    data_json = sqlc.arg(data_json),
    description = sqlc.arg(description),
    visibility = sqlc.arg(visibility),
    updated_at = sqlc.arg(updated_at)
WHERE id = sqlc.arg(id);
```

- [ ] **Step 5: Add new scoped query — ListCustomImagesByUserOrShared**

```sql
-- name: ListCustomImagesByUserOrShared :many
SELECT id, name, provider_type, data_json, description, source_machine_id, created_by_user_id, visibility, created_at, updated_at
FROM custom_images
WHERE created_by_user_id = sqlc.arg(user_id) OR visibility = 'shared'
ORDER BY created_at DESC;
```

- [ ] **Step 6: Add new scoped query — ListCustomImagesByUserOrSharedAndProfileID**

```sql
-- name: ListCustomImagesByUserOrSharedAndProfileID :many
SELECT ci.id, ci.name, ci.provider_type, ci.data_json, ci.description, ci.source_machine_id, ci.created_by_user_id, ci.visibility, ci.created_at, ci.updated_at
FROM custom_images ci
JOIN profile_custom_images pci ON pci.custom_image_id = ci.id
WHERE pci.profile_id = sqlc.arg(profile_id)
  AND (ci.created_by_user_id = sqlc.arg(user_id) OR ci.visibility = 'shared')
ORDER BY ci.name ASC;
```

- [ ] **Step 7: Run sqlc codegen**

Run: `make sqlc`
Expected: generates updated Go code in `internal/db/sqlc/sqlite/` and `internal/db/sqlc/postgresql/`

- [ ] **Step 8: Commit**

```bash
git add internal/db/sqlc/
git commit -m "Update sqlc queries for custom image visibility"
```

---

### Task 3: Go Store Layer

**Files:**
- Modify: `internal/db/custom_image_store.go`

- [ ] **Step 1: Update CustomImage struct**

Add two fields to the `CustomImage` struct (after `SourceMachineID`):

```go
type CustomImage struct {
	ID              string
	Name            string
	ProviderType    string
	DataJSON        string
	Description     string
	SourceMachineID string
	CreatedByUserID string
	Visibility      string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
```

- [ ] **Step 2: Update all row-scanning functions**

Every function that reads `CustomImage` rows from the database needs to map the two new fields. This includes `ListCustomImages`, `GetCustomImage`, `GetCustomImageByNameAndProviderType`, `ListCustomImagesByProfileID`, and `ListCustomImagesByTemplateID`. For each, add:

```go
CreatedByUserID: row.CreatedByUserID,
Visibility:      row.Visibility,
```

to the `CustomImage{}` struct literal where rows are mapped.

- [ ] **Step 3: Update CreateCustomImage to accept and pass new fields**

Change signature to:
```go
func (s *Store) CreateCustomImage(ctx context.Context, name, runtimeType, dataJSON, description, createdByUserID string) (CustomImage, error) {
```

Add to the struct literal:
```go
CreatedByUserID: createdByUserID,
Visibility:      "private",
```

Add to both sqlite and postgres `CreateCustomImageParams`:
```go
CreatedByUserID: item.CreatedByUserID,
Visibility:      item.Visibility,
```

- [ ] **Step 4: Update UpdateCustomImage to accept visibility**

Change signature to:
```go
func (s *Store) UpdateCustomImage(ctx context.Context, id, name, runtimeType, dataJSON, description, visibility string) (CustomImage, bool, error) {
```

Add `Visibility: visibility` to the update params for both drivers.

- [ ] **Step 5: Update CreateCustomImageFromMachine to accept createdByUserID**

Change signature to:
```go
func (s *Store) CreateCustomImageFromMachine(ctx context.Context, name, providerType, dataJSON, description, sourceMachineID, profileID, createdByUserID string) (*CustomImage, error) {
```

Add `CreatedByUserID: createdByUserID` to the `InsertCustomImageWithSourceParams` for both drivers.

- [ ] **Step 6: Add ListCustomImagesByUserOrShared**

```go
func (s *Store) ListCustomImagesByUserOrShared(ctx context.Context, userID string) ([]CustomImage, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListCustomImagesByUserOrShared(ctx, userID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID: row.ID, Name: row.Name, ProviderType: row.ProviderType,
				DataJSON: row.DataJson, Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
				CreatedByUserID: row.CreatedByUserID, Visibility: row.Visibility,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return items, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListCustomImagesByUserOrShared(ctx, userID)
		if err != nil {
			return nil, err
		}
		items := make([]CustomImage, 0, len(rows))
		for _, row := range rows {
			items = append(items, CustomImage{
				ID: row.ID, Name: row.Name, ProviderType: row.ProviderType,
				DataJSON: row.DataJson, Description: row.Description,
				SourceMachineID: row.SourceMachineID.String,
				CreatedByUserID: row.CreatedByUserID, Visibility: row.Visibility,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
			})
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}
```

- [ ] **Step 7: Add ListCustomImagesByUserOrSharedAndProfileID**

Same pattern as step 6 but calling the profile-joined query with both `userID` and `profileID` params.

- [ ] **Step 8: Verify build**

Run: `go vet ./...` from the repo root
Expected: no errors

- [ ] **Step 9: Commit**

```bash
git add internal/db/custom_image_store.go
git commit -m "Update custom image store with visibility and ownership fields"
```

---

### Task 4: Proto & Codegen

**Files:**
- Modify: `proto/arca/v1/image.proto`

- [ ] **Step 1: Add fields to CustomImage message**

In `proto/arca/v1/image.proto`, add to the `CustomImage` message:

```protobuf
message CustomImage {
  string id = 1;
  string name = 2;
  string template_type = 3;
  map<string, string> data = 4;
  string description = 5;
  repeated string associated_template_ids = 6;
  string created_at = 7;
  string source_machine_id = 8;
  string created_by_user_id = 9;
  string visibility = 10;
}
```

- [ ] **Step 2: Add visibility to UpdateCustomImageRequest**

```protobuf
message UpdateCustomImageRequest {
  string id = 1;
  string name = 2;
  string template_type = 3;
  map<string, string> data = 4;
  string description = 5;
  repeated string template_ids = 6;
  string visibility = 7;
}
```

- [ ] **Step 3: Run proto codegen**

Run: `make proto`
Expected: regenerated files in `internal/gen/` and `web/src/gen/`

- [ ] **Step 4: Commit**

```bash
git add proto/ internal/gen/ web/src/gen/
git commit -m "Add visibility and created_by_user_id to image proto"
```

---

### Task 5: API Handlers — Image Service

**Files:**
- Modify: `internal/server/image_connect.go`

- [ ] **Step 1: Update ListCustomImages — open to all users with scoping**

Replace the `authenticateAdmin` call with `authenticateUserFromHeaderWithResult`. If admin, call `s.store.ListCustomImages(ctx)`. Otherwise, call `s.store.ListCustomImagesByUserOrShared(ctx, userID)`.

```go
func (s *imageConnectService) ListCustomImages(ctx context.Context, req *connect.Request[arcav1.ListCustomImagesRequest]) (*connect.Response[arcav1.ListCustomImagesResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	var images []db.CustomImage
	if result.Role == db.UserRoleAdmin {
		images, err = s.store.ListCustomImages(ctx)
	} else {
		images, err = s.store.ListCustomImagesByUserOrShared(ctx, result.UserID)
	}
	if err != nil {
		slog.ErrorContext(ctx, "list custom images failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list custom images"))
	}

	items := make([]*arcav1.CustomImage, 0, len(images))
	for _, img := range images {
		runtimeIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, img.ID)
		items = append(items, toCustomImageMessage(img, runtimeIDs))
	}

	return connect.NewResponse(&arcav1.ListCustomImagesResponse{Images: items}), nil
}
```

- [ ] **Step 2: Update CreateCustomImage — pass created_by_user_id**

In `CreateCustomImage`, pass `adminUserID` to `s.store.CreateCustomImage`:

```go
img, err := s.store.CreateCustomImage(ctx, name, runtimeType, string(dataJSON), req.Msg.GetDescription(), adminUserID)
```

- [ ] **Step 3: Update UpdateCustomImage — role-based field restrictions**

After authenticating, check role. If not admin:
1. Fetch the image, verify `created_by_user_id == userID`, else `PermissionDenied`.
2. If request includes `visibility`, `template_ids`, or changed `data`/`template_type` compared to existing, return `PermissionDenied`.
3. Only allow `name` and `description` updates.

If admin: allow all fields including `visibility`.

```go
func (s *imageConnectService) UpdateCustomImage(ctx context.Context, req *connect.Request[arcav1.UpdateCustomImageRequest]) (*connect.Response[arcav1.UpdateCustomImageResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}
	isAdmin := result.Role == db.UserRoleAdmin

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	existing, err := s.store.GetCustomImage(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	if !isAdmin {
		if existing.CreatedByUserID != result.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you can only edit your own images"))
		}
		// Non-admin: reject if trying to change restricted fields
		if req.Msg.GetVisibility() != "" && req.Msg.GetVisibility() != existing.Visibility {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admin can change visibility"))
		}
		if len(req.Msg.GetTemplateIds()) > 0 {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admin can change profile associations"))
		}
		reqType := strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
		if reqType != "" && reqType != existing.ProviderType {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admin can change provider type"))
		}
		// Check data unchanged
		reqData := req.Msg.GetData()
		if len(reqData) > 0 {
			existingData := make(map[string]string)
			_ = json.Unmarshal([]byte(existing.DataJSON), &existingData)
			if !mapsEqual(reqData, existingData) {
				return nil, connect.NewError(connect.CodePermissionDenied, errors.New("only admin can change image data"))
			}
		}
	}

	// Proceed with update (admin: full update, user: name+description only)
	name := strings.TrimSpace(req.Msg.GetName())
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}

	runtimeType := existing.ProviderType
	dataJSON := existing.DataJSON
	visibility := existing.Visibility

	if isAdmin {
		runtimeType = strings.ToLower(strings.TrimSpace(req.Msg.GetTemplateType()))
		if runtimeType == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_type is required"))
		}
		data := req.Msg.GetData()
		if err := validateCustomImageData(runtimeType, data); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		dj, _ := json.Marshal(data)
		dataJSON = string(dj)
		if v := strings.TrimSpace(req.Msg.GetVisibility()); v != "" {
			visibility = v
		}
	}

	img, updated, err := s.store.UpdateCustomImage(ctx, id, name, runtimeType, dataJSON, req.Msg.GetDescription(), visibility)
	if err != nil {
		if errors.Is(err, db.ErrCustomImageNameAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("image with this name and runtime type already exists"))
		}
		slog.ErrorContext(ctx, "update custom image failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update custom image"))
	}
	if !updated {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
	}

	// Re-sync profile associations — admin only
	if isAdmin {
		if err := s.store.DisassociateAllTemplatesFromCustomImage(ctx, id); err != nil {
			slog.ErrorContext(ctx, "disassociate runtimes failed", "error", err)
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update runtime associations"))
		}
		for _, rid := range req.Msg.GetTemplateIds() {
			rid = strings.TrimSpace(rid)
			if rid == "" {
				continue
			}
			if err := s.validateTemplateTypeMatch(ctx, rid, runtimeType); err != nil {
				return nil, err
			}
			if err := s.store.AssociateTemplateCustomImage(ctx, rid, id); err != nil {
				slog.ErrorContext(ctx, "associate runtime custom image failed", "error", err)
				return nil, connect.NewError(connect.CodeInternal, errors.New("failed to associate runtime"))
			}
		}
	}

	associatedIDs, _ := s.store.ListTemplateIDsByCustomImageID(ctx, id)
	writeAuditLog(ctx, s.store, result.UserID, "", "image.update", "custom_image", id, fmt.Sprintf(`{"name":%q}`, name))
	return connect.NewResponse(&arcav1.UpdateCustomImageResponse{Image: toCustomImageMessage(img, associatedIDs)}), nil
}
```

Add a helper:

```go
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Update DeleteCustomImage — role-based ownership check**

Replace `authenticateAdmin` with full auth. If not admin, fetch image and verify `created_by_user_id == userID`.

```go
func (s *imageConnectService) DeleteCustomImage(ctx context.Context, req *connect.Request[arcav1.DeleteCustomImageRequest]) (*connect.Response[arcav1.DeleteCustomImageResponse], error) {
	result, err := authenticateUserFromHeaderWithResult(ctx, s.authenticator, s.store, req.Header())
	if err != nil {
		return nil, err
	}

	id := strings.TrimSpace(req.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	if result.Role != db.UserRoleAdmin {
		img, err := s.store.GetCustomImage(ctx, id)
		if err != nil {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("custom image not found"))
		}
		if img.CreatedByUserID != result.UserID {
			return nil, connect.NewError(connect.CodePermissionDenied, errors.New("you can only delete your own images"))
		}
	}

	// ... existing delete + audit log logic
}
```

- [ ] **Step 5: Update ListAvailableImages — scoped by user**

Replace the current unscoped query with the user-scoped one:

```go
func (s *imageConnectService) ListAvailableImages(ctx context.Context, req *connect.Request[arcav1.ListAvailableImagesRequest]) (*connect.Response[arcav1.ListAvailableImagesResponse], error) {
	userID, err := s.authenticate(ctx, req.Header())
	if err != nil {
		return nil, err
	}

	runtimeID := strings.TrimSpace(req.Msg.GetTemplateId())
	if runtimeID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("runtime_id is required"))
	}

	images, err := s.store.ListCustomImagesByUserOrSharedAndProfileID(ctx, userID, runtimeID)
	// ... rest unchanged
}
```

- [ ] **Step 6: Update toCustomImageMessage to include new fields**

```go
func toCustomImageMessage(img db.CustomImage, templateIDs []string) *arcav1.CustomImage {
	data := make(map[string]string)
	if img.DataJSON != "" && img.DataJSON != "{}" {
		_ = json.Unmarshal([]byte(img.DataJSON), &data)
	}
	return &arcav1.CustomImage{
		Id:                    img.ID,
		Name:                  img.Name,
		TemplateType:          img.ProviderType,
		Data:                  data,
		Description:           img.Description,
		AssociatedTemplateIds: templateIDs,
		CreatedAt:             img.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		SourceMachineId:       img.SourceMachineID,
		CreatedByUserId:       img.CreatedByUserID,
		Visibility:            img.Visibility,
	}
}
```

- [ ] **Step 7: Verify build**

Run: `go vet ./...`
Expected: no errors

- [ ] **Step 8: Commit**

```bash
git add internal/server/image_connect.go
git commit -m "Add visibility and ownership permission checks to image handlers"
```

---

### Task 6: Job Metadata — Pass User ID Through Image Job

**Files:**
- Modify: `internal/server/machine_connect.go:629-632`
- Modify: `internal/machine/image_job.go:39-44,78-80`

- [ ] **Step 1: Add user ID to job metadata in CreateImageFromMachine handler**

In `internal/server/machine_connect.go`, line 631, update the metadata map:

```go
metadata := map[string]string{
	"image_name":         imageName,
	"requesting_user_id": userID,
}
```

- [ ] **Step 2: Read user ID in image job worker**

In `internal/machine/image_job.go`, update the params struct (line 39):

```go
var params struct {
	ImageName        string `json:"image_name"`
	RequestingUserID string `json:"requesting_user_id"`
}
```

- [ ] **Step 3: Pass user ID to CreateCustomImageFromMachine**

In `internal/machine/image_job.go`, update the store call (line 78):

```go
customImage, err := w.store.CreateCustomImageFromMachine(sCtx,
	runner.Get("image_name"), machine.ProviderType, runner.Get("image_data"),
	job.Description, machine.ID, machine.ProfileID, params.RequestingUserID)
```

Also add after reading params:
```go
runner.Set("requesting_user_id", params.RequestingUserID)
```

Note: since `RequestingUserID` is used in a non-workflow-step context (the params struct), it can be read directly from params rather than the runner. However, if the save step needs it from the runner for idempotency, set it on the runner too.

- [ ] **Step 4: Verify build**

Run: `go vet ./...`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add internal/server/machine_connect.go internal/machine/image_job.go
git commit -m "Pass requesting user ID through image creation job metadata"
```

---

### Task 7: Backend Tests

**Files:**
- Create: `internal/server/image_connect_test.go`

- [ ] **Step 1: Write tests for image permission logic**

Create `internal/server/image_connect_test.go` (reference `internal/server/machine_profile_connect_test.go` for test setup patterns). Add table-driven tests covering:
- ListCustomImages: admin sees all, regular user sees own private + shared only
- UpdateCustomImage: regular user can edit own name/description, rejected for restricted fields, rejected for other user's image
- DeleteCustomImage: regular user can delete own, rejected for other user's image
- ListAvailableImages: returns only own private + shared for given profile

- [ ] **Step 2: Run backend tests**

Run: `make test/backend`
Expected: all tests pass

- [ ] **Step 3: Commit**

```bash
git add internal/
git commit -m "Add tests for custom image visibility permissions"
```

---

### Task 8: Frontend — API Layer & Types

**Files:**
- Modify: `web/src/lib/api.ts`

- [ ] **Step 1: Update CustomImage type**

The `CustomImage` type in `api.ts` is derived from the proto-generated code. After proto codegen (Task 4), the generated types should already include `createdByUserId` and `visibility`. Verify the type is correct and update `listCustomImages`, `updateCustomImage` to pass/receive these fields.

Update `updateCustomImage` to accept optional `visibility`:
```typescript
export async function updateCustomImage(params: {
  id: string
  name: string
  templateType: string
  data: Record<string, string>
  description: string
  templateIds: string[]
  visibility?: string
}) {
  // ... include visibility in the request
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/lib/api.ts
git commit -m "Update frontend API types for image visibility"
```

---

### Task 9: Frontend — Admin CustomImagesPage Updates

**Files:**
- Modify: `web/src/pages/CustomImagesPage.tsx`

- [ ] **Step 1: Add Visibility and Created By columns to the table**

In the table header, add after "Source Machine":
```tsx
<th className="px-4 py-3 text-left font-medium text-muted-foreground">Created by</th>
<th className="px-4 py-3 text-left font-medium text-muted-foreground">Visibility</th>
```

In the table body, add corresponding cells:
```tsx
<td className="px-4 py-3 text-muted-foreground">{img.createdByUserId || '-'}</td>
<td className="px-4 py-3">
  <Button
    variant={img.visibility === 'shared' ? 'default' : 'secondary'}
    size="sm"
    onClick={() => handleToggleVisibility(img)}
  >
    {img.visibility === 'shared' ? 'Shared' : 'Private'}
  </Button>
</td>
```

- [ ] **Step 2: Add visibility toggle handler**

```typescript
const handleToggleVisibility = async (img: CustomImage) => {
  const newVisibility = img.visibility === 'shared' ? 'private' : 'shared'
  try {
    await updateCustomImage({
      id: img.id,
      name: img.name,
      templateType: img.templateType,
      data: img.data,
      description: img.description,
      templateIds: img.associatedTemplateIds,
      visibility: newVisibility,
    })
    toast.success(`Image visibility changed to ${newVisibility}.`)
    await refresh()
  } catch (e) {
    toast.error(messageFromError(e))
  }
}
```

- [ ] **Step 3: Add visibility field to edit form**

In the form section, add a visibility selector (only shown when editing):
```tsx
{editingId && (
  <div className="space-y-2">
    <Label>Visibility</Label>
    <select
      value={form.visibility}
      onChange={(e) => setForm((p) => ({ ...p, visibility: e.target.value }))}
      className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
    >
      <option value="private">Private</option>
      <option value="shared">Shared</option>
    </select>
  </div>
)}
```

Update `ImageFormData` type and `emptyForm` to include `visibility: string`.

- [ ] **Step 4: Commit**

```bash
git add web/src/pages/CustomImagesPage.tsx
git commit -m "Add visibility toggle and created-by column to admin images page"
```

---

### Task 10: Frontend — New MyImagesPage

**Files:**
- Create: `web/src/pages/MyImagesPage.tsx`
- Modify: `web/src/App.tsx`
- Modify: `web/src/pages/AppLayout.tsx`

- [ ] **Step 1: Create MyImagesPage component**

Create `web/src/pages/MyImagesPage.tsx`. This page:
- Calls `listCustomImages()` (which now returns scoped results for regular users)
- Shows a table with columns: Name, Provider type, Description, Source Machine, Visibility (badge), Created, Actions
- Own images (where `createdByUserId === currentUser.id`): show Edit and Delete buttons
- Shared images created by others: read-only (no actions)
- Edit opens an inline form for name/description only (no provider data, no profile association, no visibility)

Follow the same visual patterns as `CustomImagesPage` but simpler — no profile association UI, no provider-specific data fields in edit.

- [ ] **Step 2: Add route in App.tsx**

Import and add route:
```tsx
import { MyImagesPage } from '@/pages/MyImagesPage'

// Inside the Route tree, alongside /machines and /settings:
<Route path="/images" element={<MyImagesPage user={user} onLogout={handleLogout} />} />
```

- [ ] **Step 3: Add nav item in AppLayout.tsx**

Add to the `navItems` array (line 29):
```typescript
const navItems = [
  { to: '/machines', label: 'Machines', icon: Cpu },
  { to: '/images', label: 'Images', icon: Image },
  { to: '/settings', label: 'User settings', icon: Settings },
]
```

The `Image` icon is already imported (line 1).

- [ ] **Step 4: Verify frontend build**

Run: `make build-frontend`
Expected: build succeeds with no errors

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/MyImagesPage.tsx web/src/App.tsx web/src/pages/AppLayout.tsx
git commit -m "Add user-facing My Images page with scoped visibility"
```

---

### Task 11: E2E Tests

**Files:**
- Create: `web/e2e/custom-image-visibility.spec.ts`

- [ ] **Step 1: Write E2E tests**

Cover:
- Admin can see all images, toggle visibility
- Regular user sees only own private + shared images
- Regular user can edit name/description of own image
- Regular user cannot edit visibility or delete shared images
- ListAvailableImages during machine creation shows correct scoped images

- [ ] **Step 2: Run E2E tests**

Run: `cd web && npx playwright test --project=fast e2e/custom-image-visibility.spec.ts`
Expected: all tests pass

- [ ] **Step 3: Commit**

```bash
git add web/e2e/custom-image-visibility.spec.ts
git commit -m "Add E2E tests for custom image visibility"
```

---

### Task 12: Final Verification

- [ ] **Step 1: Run full backend tests**

Run: `make test/backend`
Expected: all pass

- [ ] **Step 2: Run full E2E tests**

Run: `make test/e2e`
Expected: all pass

- [ ] **Step 3: Verify build**

Run: `make build`
Expected: builds successfully
