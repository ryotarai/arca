package server

import (
	"context"
	"strings"

	"github.com/ryotarai/arca/internal/db"
)

func canUserAccessExposure(ctx context.Context, store *db.Store, exposure db.MachineExposure, userID, targetPath string) bool {
	if isOwnerOnlyArcaPath(targetPath) {
		_, err := store.GetMachineByIDForUser(ctx, userID, exposure.MachineID)
		return err == nil
	}

	visibility := db.NormalizeEndpointVisibility(exposure.Visibility)
	if visibility == db.EndpointVisibilityAllArcaUsers || visibility == db.EndpointVisibilityInternetPublic {
		return true
	}
	if _, err := store.GetMachineByIDForUser(ctx, userID, exposure.MachineID); err == nil {
		return true
	}
	if visibility == db.EndpointVisibilitySelectedUsers {
		for _, selected := range exposure.SelectedUserIDs {
			if selected == userID {
				return true
			}
		}
	}
	return false
}

func isOwnerOnlyArcaPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "/__arca/readyz" {
		return false
	}
	return path == "/__arca" || strings.HasPrefix(path, "/__arca/")
}
