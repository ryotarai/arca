package server

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/ryotarai/arca/internal/db"
	"github.com/ryotarai/arca/internal/version"
)

func newArcadVersionHandler(store *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := machineTokenFromHeader(r.Header)
		if token == "" {
			http.Error(w, "machine token is required", http.StatusUnauthorized)
			return
		}

		_, err := store.GetMachineIDByMachineToken(r.Context(), token)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.Error(w, "invalid machine token", http.StatusUnauthorized)
				return
			}
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(version.Version))
	}
}
