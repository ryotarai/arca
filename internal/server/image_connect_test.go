package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

// fakeAuthenticator implements the Authenticator interface for tests.
// It returns the configured AuthResult for any session token via AuthenticateFull.
type fakeAuthenticator struct {
	result auth.AuthResult
}

func (f *fakeAuthenticator) Register(context.Context, string, string) (string, string, error) {
	return "", "", nil
}
func (f *fakeAuthenticator) Login(context.Context, string, string) (string, string, string, string, time.Time, error) {
	return "", "", "", "", time.Time{}, nil
}
func (f *fakeAuthenticator) ListUsers(context.Context) ([]db.ManagedUser, error) {
	return nil, nil
}
func (f *fakeAuthenticator) ProvisionUser(context.Context, string, string) (string, string, string, time.Time, error) {
	return "", "", "", time.Time{}, nil
}
func (f *fakeAuthenticator) IssueUserSetupToken(context.Context, string, string) (string, time.Time, error) {
	return "", time.Time{}, nil
}
func (f *fakeAuthenticator) CompleteUserSetup(context.Context, string, string) (string, string, error) {
	return "", "", nil
}
func (f *fakeAuthenticator) StartOIDCLogin(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *fakeAuthenticator) LoginWithOIDCCode(context.Context, string, string) (string, string, string, string, time.Time, error) {
	return "", "", "", "", time.Time{}, nil
}
func (f *fakeAuthenticator) Authenticate(ctx context.Context, token string) (string, string, string, error) {
	return f.result.UserID, f.result.Email, f.result.Role, nil
}
func (f *fakeAuthenticator) AuthenticateIAPJWT(ctx context.Context, jwt string) (string, string, string, error) {
	return f.result.UserID, f.result.Email, f.result.Role, nil
}
func (f *fakeAuthenticator) Logout(context.Context, string) error { return nil }
func (f *fakeAuthenticator) AuthenticateFull(ctx context.Context, token string) (auth.AuthResult, error) {
	return f.result, nil
}

// setupTestImageService creates a real SQLite-backed store and returns an
// imageConnectService plus the store for seeding test data.
func setupTestImageService(t *testing.T, authenticator Authenticator) (*imageConnectService, *db.Store) {
	t.Helper()
	ctx := context.Background()

	tmpDB := t.TempDir() + "/test.db"
	dsn := "file:" + tmpDB + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	dbConn, err := db.Open(db.Config{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { dbConn.Close() })

	if err := db.ApplyMigrations(ctx, dbConn, "sqlite"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store := db.NewStore(dbConn, "sqlite")

	svc := newImageConnectService(store, authenticator)
	return svc, store
}

// bearerHeader returns an HTTP header with a Bearer token.
func bearerHeader(token string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+token)
	return h
}

// seedImage creates a custom image directly via the store and returns it.
func seedImage(t *testing.T, ctx context.Context, store *db.Store, name, providerType, dataJSON, description, createdByUserID, visibility string) db.CustomImage {
	t.Helper()
	img, err := store.CreateCustomImage(ctx, name, providerType, dataJSON, description, createdByUserID)
	if err != nil {
		t.Fatalf("seed image %q: %v", name, err)
	}
	// Update visibility if not default "private"
	if visibility != "" && visibility != "private" {
		_, _, err := store.UpdateCustomImage(ctx, img.ID, img.Name, img.ProviderType, img.DataJSON, img.Description, visibility)
		if err != nil {
			t.Fatalf("update image visibility: %v", err)
		}
		img.Visibility = visibility
	}
	return img
}

func TestListCustomImages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	adminAuth := &fakeAuthenticator{result: auth.AuthResult{UserID: "admin-1", Email: "admin@test.com", Role: db.UserRoleAdmin}}
	userAuth := &fakeAuthenticator{result: auth.AuthResult{UserID: "user-1", Email: "user@test.com", Role: db.UserRoleUser}}

	tests := []struct {
		name          string
		auth          *fakeAuthenticator
		setupImages   func(t *testing.T, store *db.Store) // seed images
		wantCount     int
		wantImageNames []string // expected names (order does not matter)
	}{
		{
			name: "admin sees all images",
			auth: adminAuth,
			setupImages: func(t *testing.T, store *db.Store) {
				seedImage(t, ctx, store, "admin-private", "lxd", `{"image_alias":"a"}`, "", "admin-1", "private")
				seedImage(t, ctx, store, "user-private", "lxd", `{"image_alias":"b"}`, "", "user-1", "private")
				seedImage(t, ctx, store, "shared-img", "lxd", `{"image_alias":"c"}`, "", "user-1", "shared")
			},
			wantCount:     3,
			wantImageNames: []string{"admin-private", "user-private", "shared-img"},
		},
		{
			name: "regular user sees own private and all shared",
			auth: userAuth,
			setupImages: func(t *testing.T, store *db.Store) {
				seedImage(t, ctx, store, "user-private", "lxd", `{"image_alias":"a"}`, "", "user-1", "private")
				seedImage(t, ctx, store, "other-private", "lxd", `{"image_alias":"b"}`, "", "other-user", "private")
				seedImage(t, ctx, store, "shared-img", "lxd", `{"image_alias":"c"}`, "", "other-user", "shared")
			},
			wantCount:     2,
			wantImageNames: []string{"user-private", "shared-img"},
		},
		{
			name: "regular user sees nothing when no own or shared images exist",
			auth: userAuth,
			setupImages: func(t *testing.T, store *db.Store) {
				seedImage(t, ctx, store, "other-private", "lxd", `{"image_alias":"a"}`, "", "other-user", "private")
			},
			wantCount:     0,
			wantImageNames: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, store := setupTestImageService(t, tt.auth)
			// Seed users needed for audit log / foreign keys
			store.CreateUser(ctx, "admin-1", "admin@test.com", "hash")
			store.CreateUser(ctx, "user-1", "user@test.com", "hash")
			store.CreateUser(ctx, "other-user", "other@test.com", "hash")

			if tt.setupImages != nil {
				tt.setupImages(t, store)
			}

			req := connect.NewRequest(&arcav1.ListCustomImagesRequest{})
			req.Header().Set("Authorization", "Bearer fake-token")
			resp, err := svc.ListCustomImages(ctx, req)
			if err != nil {
				t.Fatalf("ListCustomImages: %v", err)
			}
			if got := len(resp.Msg.Images); got != tt.wantCount {
				t.Errorf("image count = %d, want %d", got, tt.wantCount)
			}
			gotNames := make(map[string]bool)
			for _, img := range resp.Msg.Images {
				gotNames[img.Name] = true
			}
			for _, wantName := range tt.wantImageNames {
				if !gotNames[wantName] {
					t.Errorf("expected image %q not found in results", wantName)
				}
			}
		})
	}
}

func TestUpdateCustomImage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		role     string
		userID   string
		ownerID  string // who created the image
		reqFunc  func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest]
		wantCode connect.Code
		wantErr  bool
	}{
		{
			name:    "owner can update name and description",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:   imgID,
					Name: "new-name",
					Data: map[string]string{"image_alias": "a"},
				})
			},
			wantErr: false,
		},
		{
			name:    "non-owner cannot update",
			role:    db.UserRoleUser,
			userID:  "user-2",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:   imgID,
					Name: "new-name",
					Data: map[string]string{"image_alias": "a"},
				})
			},
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "user cannot change visibility",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:         imgID,
					Name:       "img1",
					Visibility: "shared",
					Data:       map[string]string{"image_alias": "a"},
				})
			},
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "user cannot change template_ids",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:          imgID,
					Name:        "img1",
					TemplateIds: []string{"some-profile"},
					Data:        map[string]string{"image_alias": "a"},
				})
			},
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "user cannot change provider type",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:           imgID,
					Name:         "img1",
					TemplateType: "gce",
					Data:         map[string]string{"image_alias": "a"},
				})
			},
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "user cannot change image data",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:   imgID,
					Name: "img1",
					Data: map[string]string{"image_alias": "changed"},
				})
			},
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "admin can update any image including visibility",
			role:    db.UserRoleAdmin,
			userID:  "admin-1",
			ownerID: "user-1",
			reqFunc: func(imgID string) *connect.Request[arcav1.UpdateCustomImageRequest] {
				return connect.NewRequest(&arcav1.UpdateCustomImageRequest{
					Id:           imgID,
					Name:         "admin-renamed",
					TemplateType: "lxd",
					Data:         map[string]string{"image_alias": "new-alias"},
					Visibility:   "shared",
				})
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fakeAuth := &fakeAuthenticator{result: auth.AuthResult{
				UserID: tt.userID,
				Email:  tt.userID + "@test.com",
				Role:   tt.role,
			}}
			svc, store := setupTestImageService(t, fakeAuth)

			store.CreateUser(ctx, "admin-1", "admin@test.com", "hash")
			store.CreateUser(ctx, "user-1", "user@test.com", "hash")
			store.CreateUser(ctx, "user-2", "user2@test.com", "hash")

			img := seedImage(t, ctx, store, "img1", "lxd", `{"image_alias":"a"}`, "desc", tt.ownerID, "private")

			req := tt.reqFunc(img.ID)
			req.Header().Set("Authorization", "Bearer fake-token")

			_, err := svc.UpdateCustomImage(ctx, req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				connErr, ok := err.(*connect.Error)
				if !ok {
					t.Fatalf("expected connect.Error, got %T: %v", err, err)
				}
				if connErr.Code() != tt.wantCode {
					t.Errorf("error code = %v, want %v", connErr.Code(), tt.wantCode)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestDeleteCustomImage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name     string
		role     string
		userID   string
		ownerID  string
		wantCode connect.Code
		wantErr  bool
	}{
		{
			name:    "owner can delete own image",
			role:    db.UserRoleUser,
			userID:  "user-1",
			ownerID: "user-1",
			wantErr: false,
		},
		{
			name:     "non-owner cannot delete",
			role:     db.UserRoleUser,
			userID:   "user-2",
			ownerID:  "user-1",
			wantErr:  true,
			wantCode: connect.CodePermissionDenied,
		},
		{
			name:    "admin can delete any image",
			role:    db.UserRoleAdmin,
			userID:  "admin-1",
			ownerID: "user-1",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fakeAuth := &fakeAuthenticator{result: auth.AuthResult{
				UserID: tt.userID,
				Email:  tt.userID + "@test.com",
				Role:   tt.role,
			}}
			svc, store := setupTestImageService(t, fakeAuth)

			store.CreateUser(ctx, "admin-1", "admin@test.com", "hash")
			store.CreateUser(ctx, "user-1", "user@test.com", "hash")
			store.CreateUser(ctx, "user-2", "user2@test.com", "hash")

			img := seedImage(t, ctx, store, "img-del", "lxd", `{"image_alias":"a"}`, "", tt.ownerID, "private")

			req := connect.NewRequest(&arcav1.DeleteCustomImageRequest{Id: img.ID})
			req.Header().Set("Authorization", "Bearer fake-token")

			_, err := svc.DeleteCustomImage(ctx, req)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				connErr, ok := err.(*connect.Error)
				if !ok {
					t.Fatalf("expected connect.Error, got %T: %v", err, err)
				}
				if connErr.Code() != tt.wantCode {
					t.Errorf("error code = %v, want %v", connErr.Code(), tt.wantCode)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestListAvailableImages(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name           string
		role           string
		userID         string
		wantCount      int
		wantImageNames []string
	}{
		{
			name:           "admin sees own private and shared (not other private)",
			role:           db.UserRoleAdmin,
			userID:         "admin-1",
			wantCount:      2,
			wantImageNames: []string{"admin-private", "other-shared"},
		},
		{
			name:           "user sees own private and shared (not other private)",
			role:           db.UserRoleUser,
			userID:         "user-1",
			wantCount:      2,
			wantImageNames: []string{"user-private", "other-shared"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fakeAuth := &fakeAuthenticator{result: auth.AuthResult{
				UserID: tt.userID,
				Email:  tt.userID + "@test.com",
				Role:   tt.role,
			}}
			svc, store := setupTestImageService(t, fakeAuth)

			store.CreateUser(ctx, "admin-1", "admin@test.com", "hash")
			store.CreateUser(ctx, "user-1", "user@test.com", "hash")
			store.CreateUser(ctx, "other-user", "other@test.com", "hash")

			// Create a machine profile to associate images with
			profile, err := store.CreateMachineProfile(ctx, "test-profile", "lxd", `{}`)
			if err != nil {
				t.Fatalf("create profile: %v", err)
			}

			// Seed images and associate with the profile
			adminPrivate := seedImage(t, ctx, store, "admin-private", "lxd", `{"image_alias":"x"}`, "", "admin-1", "private")
			userPrivate := seedImage(t, ctx, store, "user-private", "lxd", `{"image_alias":"x"}`, "", "user-1", "private")
			otherPrivate := seedImage(t, ctx, store, "other-private", "lxd", `{"image_alias":"x"}`, "", "other-user", "private")
			otherShared := seedImage(t, ctx, store, "other-shared", "lxd", `{"image_alias":"x"}`, "", "other-user", "shared")

			for _, img := range []db.CustomImage{adminPrivate, userPrivate, otherPrivate, otherShared} {
				if err := store.AssociateProfileCustomImage(ctx, profile.ID, img.ID); err != nil {
					t.Fatalf("associate profile: %v", err)
				}
			}

			req := connect.NewRequest(&arcav1.ListAvailableImagesRequest{TemplateId: profile.ID})
			req.Header().Set("Authorization", "Bearer fake-token")

			resp, err := svc.ListAvailableImages(ctx, req)
			if err != nil {
				t.Fatalf("ListAvailableImages: %v", err)
			}
			if got := len(resp.Msg.Images); got != tt.wantCount {
				names := make([]string, 0, len(resp.Msg.Images))
				for _, img := range resp.Msg.Images {
					names = append(names, img.Name)
				}
				t.Errorf("image count = %d, want %d (got names: %v)", got, tt.wantCount, names)
			}
			gotNames := make(map[string]bool)
			for _, img := range resp.Msg.Images {
				gotNames[img.Name] = true
			}
			for _, wantName := range tt.wantImageNames {
				if !gotNames[wantName] {
					t.Errorf("expected image %q not found in results", wantName)
				}
			}
		})
	}
}
