package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/ryotarai/arca/internal/auth"
	"github.com/ryotarai/arca/internal/db"
	arcav1 "github.com/ryotarai/arca/internal/gen/arca/v1"
)

type visibilityIntegrationFixture struct {
	store             *db.Store
	authenticator     *auth.Service
	service           *tunnelConnectService
	ownerID           string
	selectedUserID    string
	unrelatedUserID   string
	ownerToken        string
	selectedUserToken string
	unrelatedToken    string
	machineID         string
	exposureName      string
	hostname          string
}

func newVisibilityIntegrationFixture(t *testing.T) visibilityIntegrationFixture {
	t.Helper()

	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	ownerID, _, err := authenticator.Register(ctx, "owner@example.com", "owner-password")
	if err != nil {
		t.Fatalf("register owner: %v", err)
	}
	selectedUserID, _, err := authenticator.Register(ctx, "selected@example.com", "selected-password")
	if err != nil {
		t.Fatalf("register selected user: %v", err)
	}
	unrelatedUserID, _, err := authenticator.Register(ctx, "other@example.com", "other-password")
	if err != nil {
		t.Fatalf("register unrelated user: %v", err)
	}

	if err := store.UpsertSetupState(ctx, db.SetupState{
		Completed: true,
	}); err != nil {
		t.Fatalf("upsert setup state: %v", err)
	}

	machine, err := store.CreateMachineWithOwner(ctx, ownerID, "visibility-integration-machine", db.MachineRuntimeLibvirt, "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}

	exposureName := "default"
	hostname := "visibility.example.test"
	if _, err := store.UpsertMachineExposure(
		ctx,
		machine.ID,
		exposureName,
		hostname,
		"http://localhost:8080",
		db.EndpointVisibilityOwnerOnly,
		nil,
	); err != nil {
		t.Fatalf("create initial exposure: %v", err)
	}

	return visibilityIntegrationFixture{
		store:             store,
		authenticator:     authenticator,
		service:           newTunnelConnectService(store, authenticator),
		ownerID:           ownerID,
		selectedUserID:    selectedUserID,
		unrelatedUserID:   unrelatedUserID,
		ownerToken:        loginToken(t, authenticator, "owner@example.com", "owner-password"),
		selectedUserToken: loginToken(t, authenticator, "selected@example.com", "selected-password"),
		unrelatedToken:    loginToken(t, authenticator, "other@example.com", "other-password"),
		machineID:         machine.ID,
		exposureName:      exposureName,
		hostname:          hostname,
	}
}

func TestTunnelVisibility_UpsertAndAuthorizeAcrossModes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fx := newVisibilityIntegrationFixture(t)
	handler := newConsoleAuthorizeHandler(fx.store, fx.authenticator)

	tests := []struct {
		name                    string
		visibility              arcav1.EndpointVisibility
		selectedUserIDs         []string
		wantStoredSelectedUsers []string
		wantAccessByToken       map[string]bool
	}{
		{
			name:                    "owner only",
			visibility:              arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_OWNER_ONLY,
			selectedUserIDs:         []string{fx.selectedUserID},
			wantStoredSelectedUsers: nil,
			wantAccessByToken: map[string]bool{
				fx.ownerToken:        true,
				fx.selectedUserToken: false,
				fx.unrelatedToken:    false,
			},
		},
		{
			name:                    "selected users",
			visibility:              arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_SELECTED_USERS,
			selectedUserIDs:         []string{" " + fx.selectedUserID + " ", "", fx.selectedUserID},
			wantStoredSelectedUsers: []string{fx.selectedUserID, fx.ownerID},
			wantAccessByToken: map[string]bool{
				fx.ownerToken:        true,
				fx.selectedUserToken: true,
				fx.unrelatedToken:    false,
			},
		},
		{
			name:                    "all arca users",
			visibility:              arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_ALL_ARCA_USERS,
			selectedUserIDs:         []string{fx.selectedUserID},
			wantStoredSelectedUsers: nil,
			wantAccessByToken: map[string]bool{
				fx.ownerToken:        true,
				fx.selectedUserToken: true,
				fx.unrelatedToken:    true,
			},
		},
		{
			name:                    "internet public",
			visibility:              arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_INTERNET_PUBLIC,
			selectedUserIDs:         []string{fx.selectedUserID},
			wantStoredSelectedUsers: nil,
			wantAccessByToken: map[string]bool{
				fx.ownerToken:        true,
				fx.selectedUserToken: true,
				fx.unrelatedToken:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := authRequest(arcav1.UpsertMachineExposureRequest{
				MachineId:       fx.machineID,
				Name:            fx.exposureName,
				Visibility:      tt.visibility,
				SelectedUserIds: tt.selectedUserIDs,
			}, fx.ownerToken)
			res, err := fx.service.UpsertMachineExposure(ctx, req)
			if err != nil {
				t.Fatalf("upsert exposure: %v", err)
			}

			if res.Msg.GetExposure().GetVisibility() != tt.visibility {
				t.Fatalf("response visibility = %v, want %v", res.Msg.GetExposure().GetVisibility(), tt.visibility)
			}
			assertStringSlicesEqual(t, res.Msg.GetExposure().GetSelectedUserIds(), tt.wantStoredSelectedUsers)

			stored, err := fx.store.GetMachineExposureByMachineIDAndName(ctx, fx.machineID, fx.exposureName)
			if err != nil {
				t.Fatalf("get stored exposure: %v", err)
			}
			assertStringSlicesEqual(t, stored.SelectedUserIDs, tt.wantStoredSelectedUsers)

			for token, wantAllowed := range tt.wantAccessByToken {
				status, location := authorizeStatusForToken(t, handler, fx.hostname, token)
				if wantAllowed {
					if status != http.StatusFound {
						t.Fatalf("authorize status = %d, want %d", status, http.StatusFound)
					}
					targetURL, err := url.Parse(location)
					if err != nil {
						t.Fatalf("parse location %q: %v", location, err)
					}
					if strings.TrimSpace(targetURL.Query().Get("token")) == "" {
						t.Fatalf("authorization redirect missing token query: %q", location)
					}
				} else if status != http.StatusNotFound {
					t.Fatalf("authorize status = %d, want %d", status, http.StatusNotFound)
				}
			}
		})
	}
}

func TestTunnelVisibility_AdminGuardrailDeniesInternetPublic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fx := newVisibilityIntegrationFixture(t)

	if err := fx.store.UpsertSetupState(ctx, db.SetupState{
		Completed:                      true,
		InternetPublicExposureDisabled: true,
	}); err != nil {
		t.Fatalf("upsert setup state: %v", err)
	}

	_, err := fx.service.UpsertMachineExposure(ctx, authRequest(arcav1.UpsertMachineExposureRequest{
		MachineId:  fx.machineID,
		Name:       fx.exposureName,
		Visibility: arcav1.EndpointVisibility_ENDPOINT_VISIBILITY_INTERNET_PUBLIC,
	}, fx.ownerToken))
	if err == nil {
		t.Fatalf("expected internet-public upsert to be denied")
	}
	if got := connect.CodeOf(err); got != connect.CodePermissionDenied {
		t.Fatalf("error code = %v, want %v", got, connect.CodePermissionDenied)
	}
	if !strings.Contains(err.Error(), "internet public visibility is disabled by admin policy") {
		t.Fatalf("error = %v, want admin policy denial message", err)
	}
}

func authorizeStatusForToken(t *testing.T, handler http.Handler, hostname, token string) (int, string) {
	t.Helper()

	target := "https://" + hostname + "/"
	req := httptest.NewRequest(http.MethodGet, "/console/authorize?target="+url.QueryEscape(target), nil)
	req.Header.Set("Cookie", sessionCookieName+"="+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	return rec.Code, rec.Header().Get("Location")
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	slices.Sort(got)
	slices.Sort(want)
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got=%v want=%v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
