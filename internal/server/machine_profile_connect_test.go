package server

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestMachineProfileConnectService_AdminOnlyOperations(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineProfileConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "profile-admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, _, err := authenticator.Register(ctx, "profile-member@example.com", "member-password"); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}

	adminToken := loginToken(t, authenticator, "profile-admin@example.com", "admin-password")
	memberToken := loginToken(t, authenticator, "profile-member@example.com", "member-password")
	created, err := service.CreateMachineProfile(ctx, authRequest(arcav1.CreateMachineProfileRequest{
		Name: "profile-edit-target",
		Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
		Config: &arcav1.MachineProfileConfig{
			Provider: &arcav1.MachineProfileConfig_Libvirt{
				Libvirt: &arcav1.LibvirtProfileConfig{
					Uri:           "qemu:///system",
					Network:       "default",
					StoragePool:   "default",
					StartupScript: "echo libvirt startup",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create profile as admin: %v", err)
	}
	profileID := created.Msg.GetProfile().GetId()

	tests := []struct {
		name     string
		call     func(token string) error
		wantCode connect.Code
	}{
		{
			name: "list",
			call: func(token string) error {
				_, err := service.ListMachineProfiles(ctx, authRequest(arcav1.ListMachineProfilesRequest{}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "create",
			call: func(token string) error {
				_, err := service.CreateMachineProfile(ctx, authRequest(arcav1.CreateMachineProfileRequest{
					Name: "member-profile",
					Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
					Config: &arcav1.MachineProfileConfig{
						Provider: &arcav1.MachineProfileConfig_Libvirt{
							Libvirt: &arcav1.LibvirtProfileConfig{
								Uri:         "qemu:///session",
								Network:     "default",
								StoragePool: "default",
							},
						},
					},
				}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "update",
			call: func(token string) error {
				_, err := service.UpdateMachineProfile(ctx, authRequest(arcav1.UpdateMachineProfileRequest{
					ProfileId: profileID,
					Name:      "member-update",
					Type:      arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE,
					Config: &arcav1.MachineProfileConfig{
						Provider: &arcav1.MachineProfileConfig_Gce{
							Gce: &arcav1.GceProfileConfig{
								Project:             "my-project",
								Zone:                "us-central1-a",
								Network:             "default",
								Subnetwork:          "default",
								ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
							},
						},
					},
				}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "delete",
			call: func(token string) error {
				_, err := service.DeleteMachineProfile(ctx, authRequest(arcav1.DeleteMachineProfileRequest{ProfileId: profileID}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(memberToken)
			if err == nil {
				t.Fatalf("expected error")
			}
			if got := connect.CodeOf(err); got != tt.wantCode {
				t.Fatalf("code = %v, want %v", got, tt.wantCode)
			}
		})
	}
}

func TestMachineProfileConnectService_CRUD(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineProfileConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "profile-admin2@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}
	adminToken := loginToken(t, authenticator, "profile-admin2@example.com", "admin-password")

	createResp, err := service.CreateMachineProfile(ctx, authRequest(arcav1.CreateMachineProfileRequest{
		Name: "edge-libvirt",
		Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
		Config: &arcav1.MachineProfileConfig{
			Provider: &arcav1.MachineProfileConfig_Libvirt{
				Libvirt: &arcav1.LibvirtProfileConfig{
					Uri:           "qemu:///system",
					Network:       "default",
					StoragePool:   "default",
					StartupScript: "echo libvirt startup",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if createResp.Msg.GetProfile().GetType() != arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT {
		t.Fatalf("created profile type = %v", createResp.Msg.GetProfile().GetType())
	}
	if createResp.Msg.GetProfile().GetConfig().GetLibvirt() == nil {
		t.Fatalf("expected libvirt config")
	}
	if got := createResp.Msg.GetProfile().GetConfig().GetLibvirt().GetStartupScript(); got != "echo libvirt startup" {
		t.Fatalf("libvirt startup script = %q", got)
	}
	if createResp.Msg.GetProfile().GetBootConfigHash() == "" {
		t.Fatalf("expected non-empty boot config hash")
	}
	profileID := createResp.Msg.GetProfile().GetId()

	listResp, err := service.ListMachineProfiles(ctx, authRequest(arcav1.ListMachineProfilesRequest{}, adminToken))
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(listResp.Msg.GetProfiles()) != 1 {
		t.Fatalf("list profiles len = %d, want 1", len(listResp.Msg.GetProfiles()))
	}

	updateResp, err := service.UpdateMachineProfile(ctx, authRequest(arcav1.UpdateMachineProfileRequest{
		ProfileId: profileID,
		Name:      "edge-gce",
		Type:      arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE,
		Config: &arcav1.MachineProfileConfig{
			Provider: &arcav1.MachineProfileConfig_Gce{
				Gce: &arcav1.GceProfileConfig{
					Project:             "my-project",
					Zone:                "us-central1-a",
					Network:             "vpc-main",
					Subnetwork:          "subnet-main",
					ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
					StartupScript:       "echo gce startup",
					AllowedMachineTypes: []string{"e2-standard-2"},
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("update profile: %v", err)
	}
	if got := updateResp.Msg.GetProfile().GetType(); got != arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE {
		t.Fatalf("updated profile type = %v, want gce", got)
	}
	if updateResp.Msg.GetProfile().GetConfig().GetGce() == nil {
		t.Fatalf("expected gce config")
	}
	if got := updateResp.Msg.GetProfile().GetConfig().GetGce().GetStartupScript(); got != "echo gce startup" {
		t.Fatalf("gce startup script = %q", got)
	}

	if _, err := service.DeleteMachineProfile(ctx, authRequest(arcav1.DeleteMachineProfileRequest{ProfileId: profileID}, adminToken)); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := service.DeleteMachineProfile(ctx, authRequest(arcav1.DeleteMachineProfileRequest{ProfileId: profileID}, adminToken)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("second delete code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
}

func TestValidateProfileRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     *arcav1.CreateMachineProfileRequest
		wantErr string
	}{
		{
			name: "valid libvirt",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "libvirt-main",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Libvirt{
						Libvirt: &arcav1.LibvirtProfileConfig{
							Uri:         "qemu:///system",
							Network:     "default",
							StoragePool: "default",
						},
					},
				},
			},
		},
		{
			name: "reject unsupported type",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "bad-type",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_UNSPECIFIED,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Libvirt{
						Libvirt: &arcav1.LibvirtProfileConfig{
							Uri:         "qemu:///system",
							Network:     "default",
							StoragePool: "default",
						},
					},
				},
			},
			wantErr: "profile type is unsupported",
		},
		{
			name: "reject missing config",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "bad-config",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
			},
			wantErr: "profile config is required",
		},
		{
			name: "reject type and config mismatch",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "mismatch",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Gce{
						Gce: &arcav1.GceProfileConfig{
							Project:             "my-project",
							Zone:                "us-central1-a",
							Network:             "vpc-main",
							Subnetwork:          "subnet-main",
							ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
						},
					},
				},
			},
			wantErr: "libvirt profile requires libvirt config only",
		},
		{
			name: "reject incomplete gce config",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "gce-incomplete",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Gce{
						Gce: &arcav1.GceProfileConfig{
							Project: "my-project",
							Zone:    "us-central1-a",
						},
					},
				},
			},
			wantErr: "gce config requires project, zone, network, subnetwork, and service account email",
		},
		{
			name: "reject oversized libvirt startup script",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "libvirt-large-script",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Libvirt{
						Libvirt: &arcav1.LibvirtProfileConfig{
							Uri:           "qemu:///system",
							Network:       "default",
							StoragePool:   "default",
							StartupScript: strings.Repeat("a", 8*1024+1),
						},
					},
				},
			},
			wantErr: "libvirt startup script must be 8KB or less",
		},
		{
			name: "reject oversized gce startup script",
			req: &arcav1.CreateMachineProfileRequest{
				Name: "gce-large-script",
				Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_GCE,
				Config: &arcav1.MachineProfileConfig{
					Provider: &arcav1.MachineProfileConfig_Gce{
						Gce: &arcav1.GceProfileConfig{
							Project:             "my-project",
							Zone:                "us-central1-a",
							Network:             "vpc-main",
							Subnetwork:          "subnet-main",
							ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
							StartupScript:       strings.Repeat("b", 8*1024+1),
						},
					},
				},
			},
			wantErr: "gce startup script must be 8KB or less",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateProfileRequest(tt.req.GetName(), tt.req.GetType(), tt.req.GetConfig())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateProfileRequest unexpected err: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if got := err.Error(); got != tt.wantErr {
				t.Fatalf("error = %q, want %q", got, tt.wantErr)
			}
		})
	}
}

func TestMachineProfileConnectService_DeleteProfileFailsWhenInUse(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineProfileConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "profile-admin3@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}
	adminToken := loginToken(t, authenticator, "profile-admin3@example.com", "admin-password")

	createResp, err := service.CreateMachineProfile(ctx, authRequest(arcav1.CreateMachineProfileRequest{
		Name: "edge-profile-in-use",
		Type: arcav1.MachineProfileType_MACHINE_PROFILE_TYPE_LIBVIRT,
		Config: &arcav1.MachineProfileConfig{
			Provider: &arcav1.MachineProfileConfig_Libvirt{
				Libvirt: &arcav1.LibvirtProfileConfig{
					Uri:         "qemu:///system",
					Network:     "default",
					StoragePool: "default",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	profileID := createResp.Msg.GetProfile().GetId()

	if _, err := store.CreateMachineWithOwner(ctx, adminID, "machine-profile-in-use", profileID, currentSetupVersion()); err != nil {
		t.Fatalf("create machine: %v", err)
	}

	_, err = service.DeleteMachineProfile(ctx, authRequest(arcav1.DeleteMachineProfileRequest{ProfileId: profileID}, adminToken))
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Fatalf("delete profile code = %v, want %v", got, connect.CodeFailedPrecondition)
	}
}
