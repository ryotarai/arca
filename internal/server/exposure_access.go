package server

import (
	"context"
	"strings"

	"github.com/ryotarai/arca/internal/db"
)

func canUserAccessMachine(ctx context.Context, store *db.Store, machineID, userID, targetPath string) bool {
	role := store.ResolveMachineRole(ctx, userID, machineID)
	if role == db.MachineRoleNone {
		return false
	}
	// /__arca/* paths (ttyd/shelley/claudecodeui) require admin or editor
	if isPrivilegedArcaPath(targetPath) {
		return role == db.MachineRoleAdmin || role == db.MachineRoleEditor
	}
	// Regular endpoints: viewer+ can access
	return true
}

func isPrivilegedArcaPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "/__arca/readyz" {
		return false
	}
	return path == "/__arca" || strings.HasPrefix(path, "/__arca/")
}
