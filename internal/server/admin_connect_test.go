package server

import (
	"context"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func newAdminServiceForTest(t *testing.T) (*adminConnectService, *userConnectService, *db.Store, *auth.Service) {
	t.Helper()

	dsn := "file:" + filepath.Join(t.TempDir(), "admin-test.db") + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	dbConn, err := db.Open(db.Config{Driver: db.DriverSQLite, DSN: dsn})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbConn.Close() })

	if err := db.ApplyMigrations(context.Background(), dbConn, db.DriverSQLite); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store := db.NewStore(dbConn, db.DriverSQLite)
	authenticator := auth.NewService(store)
	adminSvc := newAdminConnectService(store, authenticator)
	userSvc := newUserConnectService(store, authenticator, nil)
	return adminSvc, userSvc, store, authenticator
}

func setupAdminAndMember(t *testing.T, store *db.Store, authenticator *auth.Service) (adminToken, memberToken string) {
	t.Helper()
	ctx := context.Background()

	adminID, _, err := authenticator.Register(ctx, "admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, _, err := authenticator.Register(ctx, "member@example.com", "member-password"); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}

	adminToken = loginToken(t, authenticator, "admin@example.com", "admin-password")
	memberToken = loginToken(t, authenticator, "member@example.com", "member-password")
	return adminToken, memberToken
}

func TestSetAdminViewMode_AdminSucceeds(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	adminToken, _ := setupAdminAndMember(t, store, authenticator)

	// Set to "user" mode
	_, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "user"}, adminToken))
	if err != nil {
		t.Fatalf("SetAdminViewMode(user) failed: %v", err)
	}

	// Set back to "admin" mode
	_, err = adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "admin"}, adminToken))
	if err != nil {
		t.Fatalf("SetAdminViewMode(admin) failed: %v", err)
	}
}

func TestSetAdminViewMode_NonAdminDenied(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	_, memberToken := setupAdminAndMember(t, store, authenticator)

	_, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "user"}, memberToken))
	if err == nil {
		t.Fatalf("expected error for non-admin")
	}
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("code = %v, want %v", got, connect.CodePermissionDenied)
	}
}

func TestSetAdminViewMode_InvalidMode(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	adminToken, _ := setupAdminAndMember(t, store, authenticator)

	tests := []struct {
		name string
		mode string
	}{
		{name: "empty", mode: ""},
		{name: "unknown value", mode: "superadmin"},
		{name: "uppercase", mode: "Admin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: tt.mode}, adminToken))
			if err == nil {
				t.Fatalf("expected error for invalid mode %q", tt.mode)
			}
			if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
				t.Fatalf("code = %v, want %v", got, connect.CodeInvalidArgument)
			}
		})
	}
}

func TestGetAdminViewMode_Admin(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	adminToken, _ := setupAdminAndMember(t, store, authenticator)

	// Default should be "admin" mode
	resp, err := adminSvc.GetAdminViewMode(ctx, authRequest(arcav1.GetAdminViewModeRequest{}, adminToken))
	if err != nil {
		t.Fatalf("GetAdminViewMode failed: %v", err)
	}
	if resp.Msg.GetMode() != "admin" {
		t.Fatalf("mode = %q, want %q", resp.Msg.GetMode(), "admin")
	}
	if !resp.Msg.GetIsAdmin() {
		t.Fatalf("is_admin = false, want true")
	}

	// Switch to user mode
	if _, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "user"}, adminToken)); err != nil {
		t.Fatalf("SetAdminViewMode(user): %v", err)
	}

	resp, err = adminSvc.GetAdminViewMode(ctx, authRequest(arcav1.GetAdminViewModeRequest{}, adminToken))
	if err != nil {
		t.Fatalf("GetAdminViewMode after switch: %v", err)
	}
	if resp.Msg.GetMode() != "user" {
		t.Fatalf("mode = %q, want %q", resp.Msg.GetMode(), "user")
	}
	if !resp.Msg.GetIsAdmin() {
		t.Fatalf("is_admin = false, want true")
	}
}

func TestGetAdminViewMode_NonAdmin(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	_, memberToken := setupAdminAndMember(t, store, authenticator)

	resp, err := adminSvc.GetAdminViewMode(ctx, authRequest(arcav1.GetAdminViewModeRequest{}, memberToken))
	if err != nil {
		t.Fatalf("GetAdminViewMode failed: %v", err)
	}
	if resp.Msg.GetMode() != "admin" {
		t.Fatalf("mode = %q, want %q", resp.Msg.GetMode(), "admin")
	}
	if resp.Msg.GetIsAdmin() {
		t.Fatalf("is_admin = true, want false")
	}
}

func TestAdminInNonAdminMode_BlockedFromAdminEndpoints(t *testing.T) {
	ctx := context.Background()
	adminSvc, userSvc, store, authenticator := newAdminServiceForTest(t)
	adminToken, _ := setupAdminAndMember(t, store, authenticator)

	// Switch admin to non-admin mode
	if _, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "user"}, adminToken)); err != nil {
		t.Fatalf("SetAdminViewMode(user): %v", err)
	}

	// Admin endpoints that use authenticateAdmin (effective role) should be blocked
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ListAuditLogs",
			call: func() error {
				_, err := adminSvc.ListAuditLogs(ctx, authRequest(arcav1.ListAuditLogsRequest{}, adminToken))
				return err
			},
		},
		{
			name: "ListServerLLMModels",
			call: func() error {
				_, err := adminSvc.ListServerLLMModels(ctx, authRequest(arcav1.ListServerLLMModelsRequest{}, adminToken))
				return err
			},
		},
		{
			name: "ListUsers",
			call: func() error {
				_, err := userSvc.ListUsers(ctx, authRequest(arcav1.ListUsersRequest{}, adminToken))
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatalf("expected PermissionDenied for admin in non-admin mode")
			}
			if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
				t.Fatalf("code = %v, want %v", got, connect.CodePermissionDenied)
			}
		})
	}
}

func TestAdminInNonAdminMode_CanSwitchBack(t *testing.T) {
	ctx := context.Background()
	adminSvc, _, store, authenticator := newAdminServiceForTest(t)
	adminToken, _ := setupAdminAndMember(t, store, authenticator)

	// Switch to non-admin mode
	if _, err := adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "user"}, adminToken)); err != nil {
		t.Fatalf("SetAdminViewMode(user): %v", err)
	}

	// Verify blocked from admin endpoints
	_, err := adminSvc.ListAuditLogs(ctx, authRequest(arcav1.ListAuditLogsRequest{}, adminToken))
	if err == nil {
		t.Fatalf("expected PermissionDenied while in non-admin mode")
	}

	// Can still switch back using SetAdminViewMode (uses actual DB role)
	_, err = adminSvc.SetAdminViewMode(ctx, authRequest(arcav1.SetAdminViewModeRequest{Mode: "admin"}, adminToken))
	if err != nil {
		t.Fatalf("SetAdminViewMode(admin) while in non-admin mode should succeed: %v", err)
	}

	// Now admin endpoints should work again
	_, err = adminSvc.ListAuditLogs(ctx, authRequest(arcav1.ListAuditLogsRequest{}, adminToken))
	if err != nil {
		t.Fatalf("ListAuditLogs after switching back to admin mode: %v", err)
	}
}
