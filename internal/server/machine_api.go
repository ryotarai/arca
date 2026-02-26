package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ryotarai/hayai/internal/db"
)

type machineAPI struct {
	authenticator Authenticator
	store         MachineStore
}

type machinePayload struct {
	Name string `json:"name"`
}

type machineResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	DesiredStatus string `json:"desiredStatus"`
	LastError     string `json:"lastError,omitempty"`
}

func newMachineRouter(authenticator Authenticator, store MachineStore) func(r chi.Router) {
	api := &machineAPI{
		authenticator: authenticator,
		store:         store,
	}
	return func(r chi.Router) {
		r.Get("/", api.list)
		r.Post("/", api.create)
		r.Put("/{machineID}", api.update)
		r.Post("/{machineID}/start", api.start)
		r.Post("/{machineID}/stop", api.stop)
		r.Delete("/{machineID}", api.delete)
	}
}

func (a *machineAPI) list(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	machines, err := a.store.ListMachinesByUser(req.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list machines")
		return
	}

	items := make([]machineResponse, 0, len(machines))
	for _, machine := range machines {
		items = append(items, toMachineResponse(machine))
	}
	writeJSON(w, http.StatusOK, map[string]any{"machines": items})
}

func (a *machineAPI) create(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	payload, err := decodeMachinePayload(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	machine, err := a.store.CreateMachineWithOwner(req.Context(), userID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create machine")
		return
	}

	writeJSON(w, http.StatusCreated, toMachineResponse(machine))
}

func (a *machineAPI) update(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	machineID := strings.TrimSpace(chi.URLParam(req, "machineID"))
	if machineID == "" {
		writeError(w, http.StatusBadRequest, "machine id is required")
		return
	}

	payload, err := decodeMachinePayload(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	updated, err := a.store.UpdateMachineNameByIDForOwner(req.Context(), userID, machineID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update machine")
		return
	}
	if !updated {
		writeError(w, http.StatusNotFound, "machine not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": machineID, "name": name})
}

func (a *machineAPI) start(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	machineID := strings.TrimSpace(chi.URLParam(req, "machineID"))
	if machineID == "" {
		writeError(w, http.StatusBadRequest, "machine id is required")
		return
	}

	updated, err := a.store.RequestStartMachineByIDForOwner(req.Context(), userID, machineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start machine")
		return
	}
	if !updated {
		writeError(w, http.StatusNotFound, "machine not found")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"id": machineID, "status": "pending", "desiredStatus": "running"})
}

func (a *machineAPI) stop(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	machineID := strings.TrimSpace(chi.URLParam(req, "machineID"))
	if machineID == "" {
		writeError(w, http.StatusBadRequest, "machine id is required")
		return
	}

	updated, err := a.store.RequestStopMachineByIDForOwner(req.Context(), userID, machineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stop machine")
		return
	}
	if !updated {
		writeError(w, http.StatusNotFound, "machine not found")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"id": machineID, "status": "stopping", "desiredStatus": "stopped"})
}

func (a *machineAPI) delete(w http.ResponseWriter, req *http.Request) {
	userID, ok := a.authenticate(w, req)
	if !ok {
		return
	}

	machineID := strings.TrimSpace(chi.URLParam(req, "machineID"))
	if machineID == "" {
		writeError(w, http.StatusBadRequest, "machine id is required")
		return
	}

	deleted, err := a.store.DeleteMachineByIDForOwner(req.Context(), userID, machineID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete machine")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "machine not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *machineAPI) authenticate(w http.ResponseWriter, req *http.Request) (string, bool) {
	sessionToken, err := sessionTokenFromHeader(req.Header)
	if err != nil || sessionToken == "" {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return "", false
	}

	userID, _, err := a.authenticator.Authenticate(req.Context(), sessionToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated")
		return "", false
	}
	return userID, true
}

func decodeMachinePayload(req *http.Request) (machinePayload, error) {
	defer req.Body.Close()
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	var payload machinePayload
	if err := decoder.Decode(&payload); err != nil {
		return machinePayload{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return machinePayload{}, errors.New("unexpected trailing data")
	}
	return payload, nil
}

func toMachineResponse(machine db.Machine) machineResponse {
	return machineResponse{
		ID:            machine.ID,
		Name:          machine.Name,
		Status:        machine.Status,
		DesiredStatus: machine.DesiredStatus,
		LastError:     machine.LastError,
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
