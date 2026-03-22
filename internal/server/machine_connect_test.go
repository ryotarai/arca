package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
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
		{name: "reserved www", input: "www", wantError: "name is reserved"},
		{name: "reserved mail", input: "mail", wantError: "name is reserved"},
		{name: "reserved login", input: "login", wantError: "name is reserved"},
		{name: "reserved staging", input: "staging", wantError: "name is reserved"},
		{name: "reserved cdn", input: "cdn", wantError: "name is reserved"},
		{name: "reserved grafana", input: "grafana", wantError: "name is reserved"},
		{name: "reserved docs", input: "docs", wantError: "name is reserved"},
		{name: "reserved localhost", input: "localhost", wantError: "name is reserved"},
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

func TestExtractInfrastructureConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:  "empty string passthrough",
			input: "",
			checkFunc: func(t *testing.T, result string) {
				if result != "" {
					t.Fatalf("expected empty, got %q", result)
				}
			},
		},
		{
			name:  "empty object passthrough",
			input: "{}",
			checkFunc: func(t *testing.T, result string) {
				if result != "{}" {
					t.Fatalf("expected {}, got %q", result)
				}
			},
		},
		{
			name:  "strips top-level serverApiUrl",
			input: `{"serverApiUrl":"http://example.com","libvirt":{"uri":"qemu:///system"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "serverApiUrl") {
					t.Fatalf("expected serverApiUrl to be stripped, got %q", result)
				}
				if !strings.Contains(result, "libvirt") {
					t.Fatalf("expected libvirt to be preserved, got %q", result)
				}
			},
		},
		{
			name:  "strips top-level autoStopTimeoutSeconds",
			input: `{"autoStopTimeoutSeconds":"3600","gce":{"project":"my-proj"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "autoStopTimeoutSeconds") {
					t.Fatalf("expected autoStopTimeoutSeconds to be stripped, got %q", result)
				}
			},
		},
		{
			name:  "strips libvirt startupScript",
			input: `{"libvirt":{"uri":"qemu:///system","startupScript":"#!/bin/bash\necho hi"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "startupScript") {
					t.Fatalf("expected startupScript to be stripped, got %q", result)
				}
				if !strings.Contains(result, "uri") {
					t.Fatalf("expected uri to be preserved, got %q", result)
				}
			},
		},
		{
			name:  "strips gce startup_script snake_case",
			input: `{"gce":{"project":"my-proj","startup_script":"#!/bin/bash"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "startup_script") {
					t.Fatalf("expected startup_script to be stripped, got %q", result)
				}
				if !strings.Contains(result, "project") {
					t.Fatalf("expected project to be preserved, got %q", result)
				}
			},
		},
		{
			name:  "strips lxd startupScript",
			input: `{"lxd":{"endpoint":"https://localhost:8443","startupScript":"#!/bin/bash"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "startupScript") {
					t.Fatalf("expected startupScript to be stripped, got %q", result)
				}
				if !strings.Contains(result, "endpoint") {
					t.Fatalf("expected endpoint to be preserved, got %q", result)
				}
			},
		},
		{
			name:  "strips all dynamic fields at once",
			input: `{"serverApiUrl":"http://x","autoStopTimeoutSeconds":"300","libvirt":{"uri":"qemu:///system","startupScript":"echo"}}`,
			checkFunc: func(t *testing.T, result string) {
				if strings.Contains(result, "serverApiUrl") || strings.Contains(result, "autoStopTimeoutSeconds") || strings.Contains(result, "startupScript") {
					t.Fatalf("expected all dynamic fields stripped, got %q", result)
				}
				if !strings.Contains(result, "uri") {
					t.Fatalf("expected uri to be preserved, got %q", result)
				}
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := extractInfrastructureConfig(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			tc.checkFunc(t, result)
		})
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

func (s *authenticatorStub) AuthenticateIAPJWT(context.Context, string) (string, string, string, error) {
	return "", "", "", errors.New("iap not configured")
}

func (s *authenticatorStub) Logout(context.Context, string) error {
	panic("Logout should not be called in this test")
}

func (s *authenticatorStub) AuthenticateFull(ctx context.Context, sessionToken string) (auth.AuthResult, error) {
	userID, email, role, err := s.Authenticate(ctx, sessionToken)
	if err != nil {
		return auth.AuthResult{}, err
	}
	return auth.AuthResult{UserID: userID, Email: email, Role: role}, nil
}

var _ Authenticator = (*authenticatorStub)(nil)

type machineStoreStub struct {
	listMachinesByUserFunc    func(context.Context, string) ([]db.Machine, error)
	getMachineByIDForUserFunc func(context.Context, string, string) (db.Machine, error)
}

func (s *machineStoreStub) CreateMachineWithOwner(_ context.Context, _, _, _, _ string, _ ...string) (db.Machine, error) {
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

func (s *machineStoreStub) RequestStartMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestStartMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) RequestStopMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestStopMachineByIDForOwner should not be called in this test")
}

func (s *machineStoreStub) RequestRestartMachineByIDForOwner(context.Context, string, string) (bool, error) {
	panic("RequestRestartMachineByIDForOwner should not be called in this test")
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

func (s *machineStoreStub) GetMachineProfileByID(context.Context, string) (db.MachineProfile, error) {
	panic("GetMachineProfileByID should not be called in this test")
}
