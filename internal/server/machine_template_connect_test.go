package server

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

func TestMachineTemplateConnectService_AdminOnlyOperations(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineTemplateConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "template-admin@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, _, err := authenticator.Register(ctx, "template-member@example.com", "member-password"); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}

	adminToken := loginToken(t, authenticator, "template-admin@example.com", "admin-password")
	memberToken := loginToken(t, authenticator, "template-member@example.com", "member-password")
	created, err := service.CreateMachineTemplate(ctx, authRequest(arcav1.CreateMachineTemplateRequest{
		Name: "template-edit-target",
		Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
		Config: &arcav1.MachineTemplateConfig{
			Provider: &arcav1.MachineTemplateConfig_Libvirt{
				Libvirt: &arcav1.LibvirtTemplateConfig{
					Uri:           "qemu:///system",
					Network:       "default",
					StoragePool:   "default",
					StartupScript: "echo libvirt startup",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create template as admin: %v", err)
	}
	templateID := created.Msg.GetTemplate().GetId()

	tests := []struct {
		name     string
		call     func(token string) error
		wantCode connect.Code
	}{
		{
			name: "list",
			call: func(token string) error {
				_, err := service.ListMachineTemplates(ctx, authRequest(arcav1.ListMachineTemplatesRequest{}, token))
				return err
			},
			wantCode: connect.CodePermissionDenied,
		},
		{
			name: "create",
			call: func(token string) error {
				_, err := service.CreateMachineTemplate(ctx, authRequest(arcav1.CreateMachineTemplateRequest{
					Name: "member-template",
					Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
					Config: &arcav1.MachineTemplateConfig{
						Provider: &arcav1.MachineTemplateConfig_Libvirt{
							Libvirt: &arcav1.LibvirtTemplateConfig{
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
				_, err := service.UpdateMachineTemplate(ctx, authRequest(arcav1.UpdateMachineTemplateRequest{
					TemplateId: templateID,
					Name:       "member-update",
					Type:       arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE,
					Config: &arcav1.MachineTemplateConfig{
						Provider: &arcav1.MachineTemplateConfig_Gce{
							Gce: &arcav1.GceTemplateConfig{
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
				_, err := service.DeleteMachineTemplate(ctx, authRequest(arcav1.DeleteMachineTemplateRequest{TemplateId: templateID}, token))
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

func TestMachineTemplateConnectService_CRUD(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineTemplateConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "template-admin2@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}
	adminToken := loginToken(t, authenticator, "template-admin2@example.com", "admin-password")

	createResp, err := service.CreateMachineTemplate(ctx, authRequest(arcav1.CreateMachineTemplateRequest{
		Name: "edge-libvirt",
		Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
		Config: &arcav1.MachineTemplateConfig{
			Provider: &arcav1.MachineTemplateConfig_Libvirt{
				Libvirt: &arcav1.LibvirtTemplateConfig{
					Uri:           "qemu:///system",
					Network:       "default",
					StoragePool:   "default",
					StartupScript: "echo libvirt startup",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	if createResp.Msg.GetTemplate().GetType() != arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT {
		t.Fatalf("created template type = %v", createResp.Msg.GetTemplate().GetType())
	}
	if createResp.Msg.GetTemplate().GetConfig().GetLibvirt() == nil {
		t.Fatalf("expected libvirt config")
	}
	if got := createResp.Msg.GetTemplate().GetConfig().GetLibvirt().GetStartupScript(); got != "echo libvirt startup" {
		t.Fatalf("libvirt startup script = %q", got)
	}
	templateID := createResp.Msg.GetTemplate().GetId()

	listResp, err := service.ListMachineTemplates(ctx, authRequest(arcav1.ListMachineTemplatesRequest{}, adminToken))
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if len(listResp.Msg.GetTemplates()) != 1 {
		t.Fatalf("list templates len = %d, want 1", len(listResp.Msg.GetTemplates()))
	}

	updateResp, err := service.UpdateMachineTemplate(ctx, authRequest(arcav1.UpdateMachineTemplateRequest{
		TemplateId: templateID,
		Name:       "edge-gce",
		Type:       arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE,
		Config: &arcav1.MachineTemplateConfig{
			Provider: &arcav1.MachineTemplateConfig_Gce{
				Gce: &arcav1.GceTemplateConfig{
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
		t.Fatalf("update template: %v", err)
	}
	if got := updateResp.Msg.GetTemplate().GetType(); got != arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE {
		t.Fatalf("updated template type = %v, want gce", got)
	}
	if updateResp.Msg.GetTemplate().GetConfig().GetGce() == nil {
		t.Fatalf("expected gce config")
	}
	if got := updateResp.Msg.GetTemplate().GetConfig().GetGce().GetStartupScript(); got != "echo gce startup" {
		t.Fatalf("gce startup script = %q", got)
	}

	if _, err := service.DeleteMachineTemplate(ctx, authRequest(arcav1.DeleteMachineTemplateRequest{TemplateId: templateID}, adminToken)); err != nil {
		t.Fatalf("delete template: %v", err)
	}
	if _, err := service.DeleteMachineTemplate(ctx, authRequest(arcav1.DeleteMachineTemplateRequest{TemplateId: templateID}, adminToken)); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("second delete code = %v, want %v", connect.CodeOf(err), connect.CodeNotFound)
	}
}

func TestValidateTemplateRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     *arcav1.CreateMachineTemplateRequest
		wantErr string
	}{
		{
			name: "valid libvirt",
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "libvirt-main",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Libvirt{
						Libvirt: &arcav1.LibvirtTemplateConfig{
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
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "bad-type",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_UNSPECIFIED,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Libvirt{
						Libvirt: &arcav1.LibvirtTemplateConfig{
							Uri:         "qemu:///system",
							Network:     "default",
							StoragePool: "default",
						},
					},
				},
			},
			wantErr: "template type is unsupported",
		},
		{
			name: "reject missing config",
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "bad-config",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
			},
			wantErr: "template config is required",
		},
		{
			name: "reject type and config mismatch",
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "mismatch",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Gce{
						Gce: &arcav1.GceTemplateConfig{
							Project:             "my-project",
							Zone:                "us-central1-a",
							Network:             "vpc-main",
							Subnetwork:          "subnet-main",
							ServiceAccountEmail: "svc@example.iam.gserviceaccount.com",
						},
					},
				},
			},
			wantErr: "libvirt template requires libvirt config only",
		},
		{
			name: "reject incomplete gce config",
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "gce-incomplete",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Gce{
						Gce: &arcav1.GceTemplateConfig{
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
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "libvirt-large-script",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Libvirt{
						Libvirt: &arcav1.LibvirtTemplateConfig{
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
			req: &arcav1.CreateMachineTemplateRequest{
				Name: "gce-large-script",
				Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_GCE,
				Config: &arcav1.MachineTemplateConfig{
					Provider: &arcav1.MachineTemplateConfig_Gce{
						Gce: &arcav1.GceTemplateConfig{
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
			_, err := validateTemplateRequest(tt.req.GetName(), tt.req.GetType(), tt.req.GetConfig())
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTemplateRequest unexpected err: %v", err)
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

func TestMachineTemplateConnectService_DeleteTemplateFailsWhenInUse(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)
	service := newMachineTemplateConnectService(store, authenticator)

	adminID, _, err := authenticator.Register(ctx, "template-admin3@example.com", "admin-password")
	if err != nil {
		t.Fatalf("register admin: %v", err)
	}
	if _, err := store.UpdateUserRoleByID(ctx, adminID, db.UserRoleAdmin); err != nil {
		t.Fatalf("set admin role: %v", err)
	}
	adminToken := loginToken(t, authenticator, "template-admin3@example.com", "admin-password")

	createResp, err := service.CreateMachineTemplate(ctx, authRequest(arcav1.CreateMachineTemplateRequest{
		Name: "edge-template-in-use",
		Type: arcav1.MachineTemplateType_MACHINE_TEMPLATE_TYPE_LIBVIRT,
		Config: &arcav1.MachineTemplateConfig{
			Provider: &arcav1.MachineTemplateConfig_Libvirt{
				Libvirt: &arcav1.LibvirtTemplateConfig{
					Uri:         "qemu:///system",
					Network:     "default",
					StoragePool: "default",
				},
			},
		},
	}, adminToken))
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	templateID := createResp.Msg.GetTemplate().GetId()

	if _, err := store.CreateMachineWithOwner(ctx, adminID, "machine-template-in-use", templateID, currentSetupVersion()); err != nil {
		t.Fatalf("create machine: %v", err)
	}

	_, err = service.DeleteMachineTemplate(ctx, authRequest(arcav1.DeleteMachineTemplateRequest{TemplateId: templateID}, adminToken))
	if got := connect.CodeOf(err); got != connect.CodeFailedPrecondition {
		t.Fatalf("delete template code = %v, want %v", got, connect.CodeFailedPrecondition)
	}
}
