package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ryotarai/arca/internal/db"
)

func newTestAuthService(t *testing.T) *Service {
	t.Helper()

	dsn := "file:" + filepath.Join(t.TempDir(), "auth-test.db") + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
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
	return NewService(store)
}

func TestProvisionAndCompleteUserSetupLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newTestAuthService(t)

	adminID, _, err := svc.Register(ctx, "admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}

	provisionedUserID, provisionedEmail, setupToken, setupTokenExpiresAt, err := svc.ProvisionUser(ctx, "user1@example.com", adminID)
	if err != nil {
		t.Fatalf("provision user: %v", err)
	}
	if provisionedUserID == "" {
		t.Fatalf("provisioned user id is empty")
	}
	if provisionedEmail != "user1@example.com" {
		t.Fatalf("provisioned email = %q, want user1@example.com", provisionedEmail)
	}
	if setupToken == "" {
		t.Fatalf("setup token is empty")
	}
	if !setupTokenExpiresAt.After(svc.now()) {
		t.Fatalf("setup token expiry %v must be in future", setupTokenExpiresAt)
	}

	if _, _, _, _, _, err := svc.Login(ctx, "user1@example.com", "initial-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login before setup error = %v, want %v", err, ErrInvalidCredentials)
	}

	if _, _, err := svc.CompleteUserSetup(ctx, setupToken, "password-1234"); err != nil {
		t.Fatalf("complete setup: %v", err)
	}
	if _, _, _, _, _, err := svc.Login(ctx, "user1@example.com", "password-1234"); err != nil {
		t.Fatalf("login after setup: %v", err)
	}

	if _, _, err := svc.CompleteUserSetup(ctx, setupToken, "password-1234"); !errors.Is(err, ErrInvalidSetupToken) {
		t.Fatalf("reusing setup token error = %v, want %v", err, ErrInvalidSetupToken)
	}

	newSetupToken, _, err := svc.IssueUserSetupToken(ctx, provisionedUserID, adminID)
	if err != nil {
		t.Fatalf("issue setup token: %v", err)
	}
	if newSetupToken == "" {
		t.Fatalf("new setup token is empty")
	}

	if _, _, _, _, _, err := svc.Login(ctx, "user1@example.com", "password-1234"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login after reset issuance error = %v, want %v", err, ErrInvalidCredentials)
	}

	if _, _, err := svc.CompleteUserSetup(ctx, newSetupToken, "password-5678"); err != nil {
		t.Fatalf("complete setup after reset issuance: %v", err)
	}
	if _, _, _, _, _, err := svc.Login(ctx, "user1@example.com", "password-5678"); err != nil {
		t.Fatalf("login after reset completion: %v", err)
	}
}

func TestCompleteUserSetupRejectsExpiredToken(t *testing.T) {
	ctx := context.Background()
	svc := newTestAuthService(t)

	now := time.Unix(1_700_000_000, 0)
	svc.now = func() time.Time { return now }
	svc.userSetupTokenTTL = time.Minute

	adminID, _, err := svc.Register(ctx, "admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}

	_, _, setupToken, _, err := svc.ProvisionUser(ctx, "user2@example.com", adminID)
	if err != nil {
		t.Fatalf("provision user: %v", err)
	}

	now = now.Add(2 * time.Minute)
	if _, _, err := svc.CompleteUserSetup(ctx, setupToken, "password-1234"); !errors.Is(err, ErrInvalidSetupToken) {
		t.Fatalf("complete setup with expired token error = %v, want %v", err, ErrInvalidSetupToken)
	}
}
