package server

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestRuntimeConnectService_AdminOnlyOperations(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newRuntimeConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "runtime-admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, _, err := authenticator.Register(ctx, "runtime-member@example.com", "member-password"); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}

	adminToken := loginToken(t, authenticator, "runtime-admin@example.com", "admin-password")
	memberToken := loginToken(t, authenticator, "runtime-member@example.com", "member-password")
	created, err := service.CreateRuntime(ctx, authRequest(arcav1.CreateRuntimeRequest{
		Name: "runtime-edit-target",
		Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
		Config: &arcav1.RuntimeConfig{
			Provider: &arcav1.RuntimeConfig_Libvirt{
				Libvirt: &arcav1.LibvirtRuntimeConfig{
					Uri:         "qemu:///system",
					Network:     "default",
					StoragePool: "default",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create runtime as admin: %v", err)
	}
	runtimeID := created.Msg.GetRuntime().GetId()

	tests := []struct {
		name     string
		call     func(token string) error
		wantCode connect.Code
	}{
		{
			name: "list",
			call: func(token string) error {
				_, err := service.ListRuntimes(ctx, authRequest(arcav1.ListRuntimesRequest{}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "create",
			call: func(token string) error {
				_, err := service.CreateRuntime(ctx, authRequest(arcav1.CreateRuntimeRequest{
					Name: "member-runtime",
					Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
					Config: &arcav1.RuntimeConfig{
						Provider: &arcav1.RuntimeConfig_Libvirt{
							Libvirt: &arcav1.LibvirtRuntimeConfig{
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
				_, err := service.UpdateRuntime(ctx, authRequest(arcav1.UpdateRuntimeRequest{
					RuntimeId: runtimeID,
					Name:      "member-update",
					Type:      arcav1.RuntimeType_RUNTIME_TYPE_GCE,
					Config: &arcav1.RuntimeConfig{
						Provider: &arcav1.RuntimeConfig_Gce{
							Gce: &arcav1.GceRuntimeConfig{
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
				_, err := service.DeleteRuntime(ctx, authRequest(arcav1.DeleteRuntimeRequest{RuntimeId: runtimeID}, token))
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

func TestRuntimeConnectService_CRUD(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newRuntimeConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "runtime-admin2@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}
	adminToken := loginToken(t, authenticator, "runtime-admin2@example.com", "admin-password")

	createResp, err := service.CreateRuntime(ctx, authRequest(arcav1.CreateRuntimeRequest{
		Name: "edge-libvirt",
		Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
		Config: &arcav1.RuntimeConfig{
			Provider: &arcav1.RuntimeConfig_Libvirt{
				Libvirt: &arcav1.LibvirtRuntimeConfig{
					Uri:         "qemu:///system",
					Network:     "default",
					StoragePool: "default",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	if createResp.Msg.GetRuntime().GetType() != arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT {
		t.Fatalf("created runtime type = %v", createResp.Msg.GetRuntime().GetType())
	}
	if createResp.Msg.GetRuntime().GetConfig().GetLibvirt() == nil {
		t.Fatalf("expected libvirt config")
	}
	runtimeID := createResp.Msg.GetRuntime().GetId()

	listResp, err := service.ListRuntimes(ctx, authRequest(arcav1.ListRuntimesRequest{}, adminToken))
	if err != nil {
		t.Fatalf("list runtimes: %v", err)
	}
	if len(listResp.Msg.GetRuntimes()) != 1 {
		t.Fatalf("list runtimes len = %d, want 1", len(listResp.Msg.GetRuntimes()))
	}

	updateResp, err := service.UpdateRuntime(ctx, authRequest(arcav1.UpdateRuntimeRequest{
		RuntimeId: runtimeID,
		Name:      "edge-gce",
		Type:      arcav1.RuntimeType_RUNTIME_TYPE_GCE,
		Config: &arcav1.RuntimeConfig{
			Provider: &arcav1.RuntimeConfig_Gce{
				Gce: &arcav1.GceRuntimeConfig{
					Project:             "my-project",
					Zone:                "us-central1-a",
					Network:             "vpc-main",
					Subnetwork:          "subnet-main",
					ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("update runtime: %v", err)
	}
	if got := updateResp.Msg.GetRuntime().GetType(); got != arcav1.RuntimeType_RUNTIME_TYPE_GCE {
		t.Fatalf("updated runtime type = %v, want gce", got)
	}
	if updateResp.Msg.GetRuntime().GetConfig().GetGce() == nil {
		t.Fatalf("expected gce config")
	}

	if _, err := service.DeleteRuntime(ctx, authRequest(arcav1.DeleteRuntimeRequest{RuntimeId: runtimeID}, adminToken)); err != nil {
		t.Fatalf("delete runtime: %v", err)
	}
	if _, err := service.DeleteRuntime(ctx, authRequest(arcav1.DeleteRuntimeRequest{RuntimeId: runtimeID}, adminToken)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("second delete code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
}

func TestValidateRuntimeRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     *arcav1.CreateRuntimeRequest
		wantErr string
	}{
		{
			name: "valid libvirt",
			req: &arcav1.CreateRuntimeRequest{
				Name: "libvirt-main",
				Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
				Config: &arcav1.RuntimeConfig{
					Provider: &arcav1.RuntimeConfig_Libvirt{
						Libvirt: &arcav1.LibvirtRuntimeConfig{
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
			req: &arcav1.CreateRuntimeRequest{
				Name: "bad-type",
				Type: arcav1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED,
				Config: &arcav1.RuntimeConfig{
					Provider: &arcav1.RuntimeConfig_Libvirt{
						Libvirt: &arcav1.LibvirtRuntimeConfig{
							Uri:         "qemu:///system",
							Network:     "default",
							StoragePool: "default",
						},
					},
				},
			},
			wantErr: "runtime type is unsupported",
		},
		{
			name: "reject missing config",
			req: &arcav1.CreateRuntimeRequest{
				Name: "bad-config",
				Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
			},
			wantErr: "runtime config is required",
		},
		{
			name: "reject type and config mismatch",
			req: &arcav1.CreateRuntimeRequest{
				Name: "mismatch",
				Type: arcav1.RuntimeType_RUNTIME_TYPE_LIBVIRT,
				Config: &arcav1.RuntimeConfig{
					Provider: &arcav1.RuntimeConfig_Gce{
						Gce: &arcav1.GceRuntimeConfig{
							Project:             "my-project",
							Zone:                "us-central1-a",
							Network:             "vpc-main",
							Subnetwork:          "subnet-main",
							ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
						},
					},
				},
			},
			wantErr: "libvirt runtime requires libvirt config only",
		},
		{
			name: "reject incomplete gce config",
			req: &arcav1.CreateRuntimeRequest{
				Name: "gce-incomplete",
				Type: arcav1.RuntimeType_RUNTIME_TYPE_GCE,
				Config: &arcav1.RuntimeConfig{
					Provider: &arcav1.RuntimeConfig_Gce{
						Gce: &arcav1.GceRuntimeConfig{
							Project: "my-project",
							Zone:    "us-central1-a",
						},
					},
				},
			},
			wantErr: "gce config requires project, zone, network, subnetwork, and service account email",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateRuntimeRequest(tt.req.GetName(), tt.req.GetType(), tt.req.GetConfig())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRuntimeRequest unexpected err: %v", err)
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
