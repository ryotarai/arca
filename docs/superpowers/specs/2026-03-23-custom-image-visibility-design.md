# Custom Image Visibility & User Management

## Problem

Regular users can create custom images via `CreateImageFromMachine`, but cannot see or manage them afterward because `ListCustomImages` is admin-only. Conversely, `ListAvailableImages` (used during machine creation) returns all profile-associated images to all users, with no ownership scoping.

## Goals

- Users who create images can see and manage their own images.
- Images are private by default; only admins can make them shared (visible to all).
- Admins retain full control over all images.

## Data Model Changes

Add two columns to `custom_images`:

| Column | Type | Default | Description |
|---|---|---|---|
| `created_by_user_id` | TEXT NOT NULL | `''` | Creator's user ID |
| `visibility` | TEXT NOT NULL | `'private'` | `private` or `shared` |

The global `UNIQUE(name, provider_type)` constraint is kept as-is. Name collisions across users are possible but acceptable — users receive a clear "name already exists" error and must choose a different name.

### Migration

- New columns added with defaults.
- Existing images get `created_by_user_id = ''` and `visibility = 'shared'` so they remain accessible to all users after migration.

### Deleted users

Images owned by a deleted user become orphaned (invisible to non-admin users since no user matches `created_by_user_id`). Admins can see and manage these images via the admin page and may clean them up or change visibility to `shared` as needed.

## Permission Model

| Operation | Regular user | Admin |
|---|---|---|
| Create (admin page) | No | Yes (all fields) |
| Create (from machine) | Yes (private, auto) | Yes (private, auto) |
| List | Own private + shared | All images |
| Edit name/description | Own images only | All images |
| Edit visibility | No | Yes |
| Edit profile association | No | Yes |
| Edit provider data | No | Yes |
| Delete | Own images only | All images |
| ListAvailableImages (machine creation) | Own private + shared, filtered by profile | Own private + shared, filtered by profile |

## Proto Changes

### `CustomImage` message

Add fields:

```protobuf
string created_by_user_id = 9;
string visibility = 10;
```

### `UpdateCustomImageRequest` message

Add field:

```protobuf
string visibility = 7;
```

### No new RPCs

`ListCustomImages` is extended to support non-admin callers (returns scoped results). No new endpoints needed.

## API Behavior Changes

### `ListCustomImages`

- **Before:** Admin-only, returns all images.
- **After:** Any authenticated user can call. Returns:
  - Admin: all images.
  - Regular user: images where `created_by_user_id = caller` OR `visibility = 'shared'`.

### `CreateCustomImage` (admin page)

- Admin-only (unchanged).
- Sets `created_by_user_id` to the calling admin's user ID.
- `visibility` defaults to `private`. No `visibility` field in the create request — admins use `UpdateCustomImage` to change visibility after creation.

### `CreateImageFromMachine`

- Sets `created_by_user_id` to the calling user's ID.
- `visibility` is always `private`.
- **Implementation note:** The actual image record is created asynchronously by the image job worker (`image_job.go`), not directly in the RPC handler. The requesting user's ID must be stored in the job's `metadata_json` so the worker can set `created_by_user_id` when calling `CreateCustomImageFromMachine`.

### `UpdateCustomImage`

- **Admin:** Can update all fields including `visibility` and profile associations.
- **Regular user:** Can update `name` and `description` only, and only for their own images. If the request includes changes to `visibility`, profile associations, or provider data, the handler rejects the entire request with `PermissionDenied`. Attempting to edit another user's image also returns `PermissionDenied`.

### `DeleteCustomImage`

- **Admin:** Can delete any image.
- **Regular user:** Can delete only their own images.

### `ListAvailableImages` (machine creation)

- **Before:** Returns all images for the given profile, any authenticated user.
- **After:** Returns images for the given profile where `created_by_user_id = caller` OR `visibility = 'shared'`. Same behavior for all users including admins.

## DB Query Changes

### New queries (sqlc)

- `ListCustomImagesByUserOrShared(userID)` — returns images where `created_by_user_id = ?` OR `visibility = 'shared'`, ordered by `created_at DESC`.
- `ListCustomImagesByUserOrSharedAndProfileID(userID, profileID)` — same filter plus profile association join.

Consider adding an index on `(visibility, created_by_user_id)` in the migration for query performance.

### Modified queries

- `CreateCustomImage` — accepts `created_by_user_id` and `visibility` params.
- `InsertCustomImageWithSource` — accepts `created_by_user_id` param, sets `visibility = 'private'`.

## UI Changes

### Existing `CustomImagesPage` (admin)

- Add `Created by` and `Visibility` columns to the table.
- Add visibility toggle (private/shared) per image.
- No other changes; admin retains full CRUD.

### New "My Images" page (regular users)

- Accessible to all authenticated users.
- Shows own private images + shared images.
- Own images: edit (name/description) and delete buttons.
- Shared images: read-only (no edit/delete).
- Visibility badge displayed per image (`Private` / `Shared`).
- New navigation entry in sidebar/header at route `/images`.

### `CreateMachinePage`

- No UI changes needed; `ListAvailableImages` already drives the image selector and will now return scoped results.
