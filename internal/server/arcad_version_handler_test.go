package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/version"
)

func TestArcadVersionHandler(t *testing.T) {
	ctx := context.Background()
	store, authenticator := newUserServiceForTest(t)

	userID, _, err := authenticator.Register(ctx, "version-handler@example.com", "password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	profile, err := store.CreateMachineProfile(ctx, "test-profile", db.ProviderTypeLibvirt, `{"libvirt":{"uri":"qemu:///system","network":"default","storagePool":"default"}}`)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	machine, err := store.CreateMachineWithOwner(ctx, userID, "version-test", profile.ID, "v1")
	if err != nil {
		t.Fatalf("create machine: %v", err)
	}

	handler := newArcadVersionHandler(store)

	t.Run("returns version with valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/arcad/version", nil)
		req.Header.Set("Authorization", "Bearer "+machine.MachineToken)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		body, _ := io.ReadAll(w.Body)
		if string(body) != version.Version {
			t.Fatalf("body = %q, want %q", string(body), version.Version)
		}
	})

	t.Run("rejects missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/arcad/version", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})

	t.Run("rejects invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/arcad/version", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
		}
	})
}
