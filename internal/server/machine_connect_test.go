package server

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestValidateMachineName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{name: "valid simple", input: "app1"},
		{name: "valid hyphen", input: "my-machine-1"},
		{name: "empty", input: "", wantError: "name is required"},
		{name: "too short", input: "ab", wantError: "name must be at least 3 characters"},
		{name: "dot not allowed", input: "my.machine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "uppercase not allowed", input: "MyMachine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "leading hyphen", input: "-machine", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "trailing hyphen", input: "machine-", wantError: "name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen"},
		{name: "reserved admin", input: "admin", wantError: "name is reserved"},
		{name: "reserved console", input: "console", wantError: "name is reserved"},
		{name: "reserved dash", input: "dash", wantError: "name is reserved"},
		{name: "reserved api", input: "api", wantError: "name is reserved"},
		{name: "reserved system", input: "system", wantError: "name is reserved"},
		{name: "reserved arca prefix", input: "arca-demo", wantError: "name cannot start with arca-"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateMachineName(tt.input)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Fatalf("unexpected error: got %q want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestListMachinesPropagatesUpdateRequired(t *testing.T) {
	t.Setenv("ARCA_SETUP_VERSION", "v2.0.0")

	auth := &authenticatorStub{
		authenticateFunc: func(ctx context.Context, sessionToken string) (string, string, string, error) {
			return "user-1", "admin@example.com", "user", nil
		},
	}
	store := &machineStoreStub{
		listMachinesByUserFunc: func(ctx context.Context, userID string) ([]db.Machine, error) {
			if userID != "user-1" {
				t.Fatalf("unexpected user id: %s", userID)
			}
			return []db.Machine{
				{
					ID:            "machine-old",
					Name:          "machine-old",
					Status:        db.MachineStatusRunning,
					DesiredStatus: db.MachineDesiredRunning,
					SetupVersion:  "v1.9.0",
				},
				{
					ID:            "machine-current",
					Name:          "machine-current",
					Status:        db.MachineStatusRunning,
					DesiredStatus: db.MachineDesiredRunning,
					SetupVersion:  "v2.0.0",
				},
			}, nil
		},
	}
	svc := newMachineConnectService(auth, store, nil)

	req := connect.NewRequest(&arcav1.ListMachinesRequest{})
	req.Header().Set("Cookie", sessionCookieName+"=session-token")
	resp, err := svc.ListMachines(context.Background(), req)
	if err != nil {
		t.Fatalf("ListMachines() unexpected error: %v", err)
	}

	machines := resp.Msg.GetMachines()
	if len(machines) != 2 {
		t.Fatalf("len(machines) = %d, want 2", len(machines))
	}
	if !machines[0].GetUpdateRequired() {
		t.Fatalf("machine-old updateRequired = false, want true")
	}
	if machines[1].GetUpdateRequired() {
		t.Fatalf("machine-current updateRequired = true, want false")
	}
}

func TestGetMachinePropagatesUpdateRequired(t *testing.T) {
	t.Setenv("ARCA_SETUP_VERSION", "v3.0.0")

	auth := &authenticatorStub{
		authenticateFunc: func(ctx context.Context, sessionToken string) (string, string, string, error) {
			return "user-1", "admin@example.com", "user", nil
		},
	}
	store := &machineStoreStub{
		getMachineByIDForUserFunc: func(ctx context.Context, userID, machineID string) (db.Machine, error) {
			if userID != "user-1" {
				t.Fatalf("unexpected user id: %s", userID)
			}
			if machineID != "machine-1" {
				t.Fatalf("unexpected machine id: %s", machineID)
			}
			return db.Machine{
				ID:            machineID,
				Name:          "machine-1",
				Status:        db.MachineStatusStopped,
				DesiredStatus: db.MachineDesiredStopped,
				SetupVersion:  "v2.9.0",
			}, nil
		},
	}
	svc := newMachineConnectService(auth, store, nil)

	req := connect.NewRequest(&arcav1.GetMachineRequest{MachineId: "machine-1"})
	req.Header().Set("Cookie", sessionCookieName+"=session-token")
	resp, err := svc.GetMachine(context.Background(), req)
	if err != nil {
		t.Fatalf("GetMachine() unexpected error: %v", err)
	}
	if resp.Msg.GetMachine() == nil {
		t.Fatal("GetMachine() machine is nil")
	}
	if !resp.Msg.GetMachine().GetUpdateRequired() {
		t.Fatalf("GetMachine() updateRequired = false, want true")
	}
}

type authenticatorStub struct {
	authenticateFunc func(context.Context, string) (string, string, string, error)
}

func (s *authenticatorStub) Register(context.Context, string, string) (string, string, error) {
	panic("Register should not be called in this test")
}

func (s *authenticatorStub) Login(context.Context, string, string) (string, string, string, string, time.Time, error) {
	panic("Login should not be called in this test")
}

func (s *authenticatorStub) ListUsers(context.Context) ([]db.ManagedUser, error) {
	panic("ListUsers should not be called in this test")
}

func (s *authenticatorStub) ProvisionUser(context.Context, string, string) (string, string, string, time.Time, error) {
	panic("ProvisionUser should not be called in this test")
}

func (s *authenticatorStub) IssueUserSetupToken(context.Context, string, string) (string, time.Time, error) {
	panic("IssueUserSetupToken should not be called in this test")
}

func (s *authenticatorStub) CompleteUserSetup(context.Context, string, string) (string, string, error) {
	panic("CompleteUserSetup should not be called in this test")
}

func (s *authenticatorStub) StartOIDCLogin(context.Context, string, string) (string, error) {
	panic("StartOIDCLogin should not be called in this test")
}

func (s *authenticatorStub) LoginWithOIDCCode(context.Context, string, string) (string, string, string, string, time.Time, error) {
	panic("LoginWithOIDCCode should not be called in this test")
}

func (s *authenticatorStub) Authenticate(ctx context.Context, sessionToken string) (string, string, string, error) {
	if s.authenticateFunc == nil {
		panic("Authenticate should not be called in this test")
	}
	return s.authenticateFunc(ctx, sessionToken)
}

func (s *authenticatorStub) Logout(context.Context, string) error {
	panic("Logout should not be called in this test")
}

var _ Authenticator = (*authenticatorStub)(nil)

type machineStoreStub struct {
	listMachinesByUserFunc    func(context.Context, string) ([]db.Machine, error)
	getMachineByIDForUserFunc func(context.Context, string, string) (db.Machine, error)
}

func (s *machineStoreStub) CreateMachineWithOwner(context.Context, string, string, string, string) (db.Machine, error) {
	panic("CreateMachineWithOwner should not be called in this test")
}

func (s *machineStoreStub) ListMachinesByUser(ctx context.Context, userID string) ([]db.Machine, error) {
	if s.listMachinesByUserFunc == nil {
		panic("ListMachinesByUser should not be called in this test")
	}
	return s.listMachinesByUserFunc(ctx, userID)
}

func (s *machineStoreStub) GetMachineByIDForUser(ctx context.Context, userID, machineID string) (db.Machine, error) {
	if s.getMachineByIDForUserFunc == nil {
		panic("GetMachineByIDForUser should not be called in this test")
	}
	return s.getMachineByIDForUserFunc(ctx, userID, machineID)
}

func (s *machineStoreStub) ListMachineEventsByMachineIDForUser(context.Context, string, string, int64) ([]db.MachineEvent, error) {
	panic("ListMachineEventsByMachineIDForUser should not be called in this test")
}

func (s *machineStoreStub) UpdateMachineNameByIDForOwner(context.Context, string, string, string) (bool, error) {
	panic("UpdateMachineNameByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) UpdateMachineRuntimeByIDForOwner(context.Context, string, string, string, string) (bool, error) {
	panic("UpdateMachineRuntimeByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) RequestStartMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestStartMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) RequestStopMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestStopMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) RequestDeleteMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestDeleteMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) DeleteMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("DeleteMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) DeleteMachineByID(context.Context, string) (bool, error) {
	panic("DeleteMachineByID should not be called in this test")
}

func (s *machineStoreStub) GetMachineTunnelByMachineID(context.Context, string) (db.MachineTunnel, error) {
	panic("GetMachineTunnelByMachineID should not be called in this test")
}

func (s *machineStoreStub) GetRuntimeByID(context.Context, string) (db.RuntimeCatalog, error) {
	panic("GetRuntimeByID should not be called in this test")
}
