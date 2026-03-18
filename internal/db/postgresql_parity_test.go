package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

type parityBackend struct {
	name   string
	driver string
	dsn    string
}

func TestStoreParityCoreWorkflows(t *testing.T) {
	t.Parallel()

	for _, backend := range parityBackends(t) {
		backend := backend
		t.Run(backend.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			dbConn, err := Open(Config{Driver: backend.driver, DSN: backend.dsn})
			if err != nil {
				t.Fatalf("open db: %v", err)
			}
			defer dbConn.Close()

			if err := ApplyMigrations(ctx, dbConn, backend.driver); err != nil {
				t.Fatalf("apply migrations: %v", err)
			}
			if err := ApplyMigrations(ctx, dbConn, backend.driver); err != nil {
				t.Fatalf("apply migrations idempotent run: %v", err)
			}

			store := NewStore(dbConn, backend.driver)
			if err := store.Ping(ctx); err != nil {
				t.Fatalf("store ping: %v", err)
			}

			userID := "user-1"
			if err := store.CreateUser(ctx, userID, "owner@example.com", "pw-hash"); err != nil {
				t.Fatalf("create user: %v", err)
			}

			sessionToken := "session-token-hash"
			if err := store.CreateSession(ctx, "session-1", userID, sessionToken, time.Now().Add(30*time.Minute).Unix()); err != nil {
				t.Fatalf("create session: %v", err)
			}
			authUser, err := store.GetUserByActiveSessionTokenHash(ctx, sessionToken, time.Now().Unix())
			if err != nil {
				t.Fatalf("get user by active session: %v", err)
			}
			if authUser.ID != userID {
				t.Fatalf("active session user mismatch: got %q want %q", authUser.ID, userID)
			}
			if err := store.RevokeSessionByTokenHash(ctx, sessionToken); err != nil {
				t.Fatalf("revoke session: %v", err)
			}
			if _, err := store.GetUserByActiveSessionTokenHash(ctx, sessionToken, time.Now().Unix()); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected revoked session to be unavailable, got err=%v", err)
			}

			if err := store.UpsertSetupState(ctx, SetupState{
				Completed:      true,
				BaseDomain:     "example.com",
				DomainPrefix:   "arca-",
				MachineRuntime: "libvirt",
				OIDCEnabled:    true,
				OIDCIssuerURL:  "https://accounts.google.com",
				OIDCClientID:   "client-id",
				OIDCClientSecret: "client-secret",
				OIDCAllowedEmailDomains: []string{
					"example.com",
				},
			}); err != nil {
				t.Fatalf("upsert setup state: %v", err)
			}
			setup, err := store.GetSetupState(ctx)
			if err != nil {
				t.Fatalf("get setup state: %v", err)
			}
			if !setup.Completed || setup.MachineRuntime != "libvirt" {
				t.Fatalf("unexpected setup state: %+v", setup)
			}
			if !setup.OIDCEnabled || setup.OIDCIssuerURL != "https://accounts.google.com" || setup.OIDCClientID != "client-id" || setup.OIDCClientSecret != "client-secret" {
				t.Fatalf("unexpected oidc setup state: %+v", setup)
			}

			created, err := store.CreateMachineWithOwner(ctx, userID, "machine-one", "libvirt", "v1")
			if err != nil {
				t.Fatalf("create machine: %v", err)
			}
			if _, err := store.CreateMachineWithOwner(ctx, userID, "machine-one", "libvirt", "v1"); !errors.Is(err, ErrMachineNameAlreadyExists) {
				t.Fatalf("expected duplicate name error, got %v", err)
			}

			machines, err := store.ListMachinesByUser(ctx, userID)
			if err != nil {
				t.Fatalf("list machines: %v", err)
			}
			if len(machines) != 1 {
				t.Fatalf("expected 1 machine, got %d", len(machines))
			}
			if machines[0].Name != "machine-one" || machines[0].DesiredStatus != MachineDesiredRunning {
				t.Fatalf("unexpected machine from list: %+v", machines[0])
			}

			requested, err := store.RequestStopMachineByIDForOwner(ctx, userID, created.ID)
			if err != nil {
				t.Fatalf("request stop: %v", err)
			}
			if !requested {
				t.Fatal("expected stop request to succeed")
			}

			_, found, err := store.ClaimNextMachineJob(ctx, "worker-1", time.Now().Add(30*time.Second).Unix(), time.Now().Unix())
			if err != nil {
				t.Fatalf("claim next machine job: %v", err)
			}
			if !found {
				t.Fatal("expected at least one runnable machine job")
			}

			ticket, err := store.CreateAuthTicket(ctx, userID, created.ID, "", time.Now().Add(5*time.Minute).Unix())
			if err != nil {
				t.Fatalf("create auth ticket: %v", err)
			}
			verified, err := store.VerifyAndConsumeAuthTicket(ctx, created.MachineToken, ticket, time.Now().Unix())
			if err != nil {
				t.Fatalf("verify auth ticket: %v", err)
			}
			if verified.UserID != userID || verified.MachineID != created.ID {
				t.Fatalf("unexpected verified ticket: %+v", verified)
			}
			if _, err := store.VerifyAndConsumeAuthTicket(ctx, created.MachineToken, ticket, time.Now().Unix()); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected consumed ticket failure, got %v", err)
			}

			firstExposure, err := store.UpsertMachineExposure(ctx, created.ID, "ssh", "ssh.example.com", "http://localhost:2222")
			if err != nil {
				t.Fatalf("upsert machine exposure: %v", err)
			}
			updatedExposure, err := store.UpsertMachineExposure(ctx, created.ID, "ssh", "ssh-updated.example.com", "http://localhost:2223")
			if err != nil {
				t.Fatalf("upsert machine exposure update: %v", err)
			}
			if updatedExposure.ID != firstExposure.ID {
				t.Fatalf("expected upsert to preserve exposure id: before=%q after=%q", firstExposure.ID, updatedExposure.ID)
			}
			if updatedExposure.Hostname != "ssh-updated.example.com" {
				t.Fatalf("unexpected updated exposure: %+v", updatedExposure)
			}

			deleted, err := store.DeleteMachineByIDForOwner(ctx, userID, created.ID)
			if err != nil {
				t.Fatalf("delete machine by owner: %v", err)
			}
			if !deleted {
				t.Fatal("expected machine deletion to succeed")
			}
			if _, err := store.GetMachineByID(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("expected deleted machine lookup to fail with sql.ErrNoRows, got %v", err)
			}

			beforeCount, err := countMachineEventsByMachineID(ctx, dbConn, backend.driver, created.ID)
			if err != nil {
				t.Fatalf("count events before insert after delete: %v", err)
			}
			if beforeCount == 0 {
				t.Fatal("expected machine events to remain after machine deletion")
			}
			if err := store.CreateMachineEvent(ctx, MachineEventInput{
				MachineID: created.ID,
				JobID:     "job-after-delete",
				Level:     "info",
				EventType: "post_delete_event",
				Message:   "event recorded after machine deletion",
			}); err != nil {
				t.Fatalf("create machine event after machine deletion: %v", err)
			}
			afterCount, err := countMachineEventsByMachineID(ctx, dbConn, backend.driver, created.ID)
			if err != nil {
				t.Fatalf("count events after insert after delete: %v", err)
			}
			if afterCount != beforeCount+1 {
				t.Fatalf("expected event count to increase after insert: before=%d after=%d", beforeCount, afterCount)
			}
		})
	}
}

func countMachineEventsByMachineID(ctx context.Context, dbConn *sql.DB, driver, machineID string) (int, error) {
	var row *sql.Row
	switch driver {
	case DriverSQLite:
		row = dbConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM machine_events WHERE machine_id = ?", machineID)
	case DriverPostgres:
		row = dbConn.QueryRowContext(ctx, "SELECT COUNT(*) FROM machine_events WHERE machine_id = $1", machineID)
	default:
		return 0, fmt.Errorf("unsupported driver %q", driver)
	}

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func parityBackends(t *testing.T) []parityBackend {
	t.Helper()

	backends := []parityBackend{
		{
			name:   "sqlite",
			driver: DriverSQLite,
			dsn:    fmt.Sprintf("file:%s/parity.db?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", t.TempDir()),
		},
	}

	basePostgresDSN := strings.TrimSpace(os.Getenv("ARCA_TEST_POSTGRES_DSN"))
	if basePostgresDSN == "" {
		t.Log("ARCA_TEST_POSTGRES_DSN is not set; running SQLite parity checks only")
		return backends
	}

	postgresDSN, cleanup := preparePostgresSchemaDSN(t, basePostgresDSN)
	t.Cleanup(cleanup)
	backends = append(backends, parityBackend{
		name:   "postgresql",
		driver: DriverPostgres,
		dsn:    postgresDSN,
	})

	return backends
}

func preparePostgresSchemaDSN(t *testing.T, baseDSN string) (string, func()) {
	t.Helper()

	parsed, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("parse ARCA_TEST_POSTGRES_DSN: %v", err)
	}

	schemaName := fmt.Sprintf("parity_%d", time.Now().UnixNano())
	adminDB, err := sql.Open("pgx", baseDSN)
	if err != nil {
		t.Fatalf("open postgres admin db: %v", err)
	}
	if _, err := adminDB.Exec(`CREATE SCHEMA "` + schemaName + `"`); err != nil {
		adminDB.Close()
		t.Fatalf("create schema %q: %v", schemaName, err)
	}

	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()

	return parsed.String(), func() {
		_, _ = adminDB.Exec(`DROP SCHEMA IF EXISTS "` + schemaName + `" CASCADE`)
		_ = adminDB.Close()
	}
}
