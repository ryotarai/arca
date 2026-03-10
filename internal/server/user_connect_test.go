package server

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func newUserServiceForTest(t *testing.T) (*db.Store, *auth.Service) {
	t.Helper()

	dsn := "file:" + filepath.Join(t.TempDir(), "server-user-test.db") + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	dbConn, err := db.Open(db.Config{Driver: db.DriverSQLite, DSN: dsn})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = dbConn.Close()
	})

	if err := db.ApplyMigrations(context.Background(), dbConn, db.DriverSQLite); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	store := db.NewStore(dbConn, db.DriverSQLite)
	return store, auth.NewService(store)
}

func loginToken(t *testing.T, authenticator *auth.Service, email, password string) string {
	t.Helper()
	_, _, _, sessionToken, _, err := authenticator.Login(context.Background(), email, password)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return sessionToken
}

func authRequest[T any](msg T, sessionToken string) *connect.Request[T] {
	req := connect.NewRequest(&msg)
	req.Header().Set("Cookie", sessionCookieName+"="+sessionToken)
	return req
}

func TestUserConnectService_AdminOnlyOperations(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newUserConnectService(store, authenticator)

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

	adminToken := loginToken(t, authenticator, "admin@example.com", "admin-password")
	memberToken := loginToken(t, authenticator, "member@example.com", "member-password")

	tests := []struct {
		name       string
		call       func(token string) error
		wantCode   connect.Code
		memberOnly bool
	}{
		{
			name: "list users",
			call: func(token string) error {
				_, err := service.ListUsers(ctx, authRequest(arcav1.ListUsersRequest{}, token))
				return err
			},
			wantCode:   connect.CodePermissionDenied,
			memberOnly: true,
		},
		{
			name: "create user",
			call: func(token string) error {
				_, err := service.CreateUser(ctx, authRequest(arcav1.CreateUserRequest{Email: "new@example.com"}, token))
				return err
			},
			wantCode:   connect.CodePermissionDenied,
			memberOnly: true,
		},
		{
			name: "issue setup token",
			call: func(token string) error {
				_, _, setupToken, _, createErr := authenticator.ProvisionUser(ctx, "token-target@example.com", adminID)
				if createErr != nil && !errors.Is(createErr, auth.ErrEmailAlreadyUsed) {
					t.Fatalf("provision target user: %v", createErr)
				}
				if setupToken != "" {
					_, _, _ = authenticator.CompleteUserSetup(ctx, setupToken, "password-1234")
				}
				target, getErr := store.GetUserByEmail(ctx, "token-target@example.com")
				if getErr != nil {
					t.Fatalf("get target user: %v", getErr)
				}
				_, err := service.IssueUserSetupToken(ctx, authRequest(arcav1.IssueUserSetupTokenRequest{UserId: target.ID}, token))
				return err
			},
			wantCode:   connect.CodePermissionDenied,
			memberOnly: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name+" denied for non-admin", func(t *testing.T) {
			err := tt.call(memberToken)
			if err == nil {
				t.Fatalf("expected error for non-admin")
			}
			if got := connect.CodeOf(err); got != tt.wantCode {
				t.Fatalf("code = %v, want %v", got, tt.wantCode)
			}
		})
	}

	if _, err := service.ListUsers(ctx, authRequest(arcav1.ListUsersRequest{}, adminToken)); err != nil {
		t.Fatalf("admin list users: %v", err)
	}
	if _, err := service.CreateUser(ctx, authRequest(arcav1.CreateUserRequest{Email: "allowed@example.com"}, adminToken)); err != nil {
		t.Fatalf("admin create user: %v", err)
	}
	target, err := store.GetUserByEmail(ctx, "allowed@example.com")
	if err != nil {
		t.Fatalf("get created user: %v", err)
	}
	if _, err := service.IssueUserSetupToken(ctx, authRequest(arcav1.IssueUserSetupTokenRequest{UserId: target.ID}, adminToken)); err != nil {
		t.Fatalf("admin issue setup token: %v", err)
	}
}

func TestUserConnectService_UserSettings(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newUserConnectService(store, authenticator)

	if _, _, err := authenticator.Register(ctx, "member@example.com", "member-password"); err != nil {
		t.Fatalf("register member: %v", err)
	}
	memberToken := loginToken(t, authenticator, "member@example.com", "member-password")

	getResp, err := service.GetUserSettings(ctx, authRequest(arcav1.GetUserSettingsRequest{}, memberToken))
	if err != nil {
		t.Fatalf("get user settings: %v", err)
	}
	if got := len(getResp.Msg.GetSettings().GetSshPublicKeys()); got != 0 {
		t.Fatalf("ssh public keys len = %d, want 0", got)
	}

	validKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOJ9vZxA2v4n5hF8B07A2fkYg6P5mK2xOb3d9HfNQh8S test@example.com"
	updateResp, err := service.UpdateUserSettings(ctx, authRequest(arcav1.UpdateUserSettingsRequest{
		Settings: &arcav1.UserSettings{
			SshPublicKeys: []string{
				"  " + validKey + "  ",
				validKey,
				"",
			},
		},
	}, memberToken))
	if err != nil {
		t.Fatalf("update user settings: %v", err)
	}
	if got := len(updateResp.Msg.GetSettings().GetSshPublicKeys()); got != 1 {
		t.Fatalf("updated ssh public keys len = %d, want 1", got)
	}

	getResp, err = service.GetUserSettings(ctx, authRequest(arcav1.GetUserSettingsRequest{}, memberToken))
	if err != nil {
		t.Fatalf("get user settings after update: %v", err)
	}
	if got := len(getResp.Msg.GetSettings().GetSshPublicKeys()); got != 1 {
		t.Fatalf("stored ssh public keys len = %d, want 1", got)
	}

	_, err = service.UpdateUserSettings(ctx, authRequest(arcav1.UpdateUserSettingsRequest{
		Settings: &arcav1.UserSettings{SshPublicKeys: []string{"invalid-key"}},
	}, memberToken))
	if err == nil {
		t.Fatalf("expected invalid key error")
	}
	if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
		t.Fatalf("invalid key code = %v, want %v", code, connect.CodeInvalidArgument)
	}
}
