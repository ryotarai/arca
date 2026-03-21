package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

const (
	MachineStatusPending  = "pending"
	MachineStatusStarting = "starting"
	MachineStatusRunning  = "running"
	MachineStatusStopping = "stopping"
	MachineStatusStopped  = "stopped"
	MachineStatusDeleting = "deleting"
	MachineStatusFailed   = "failed"

	MachineDesiredRunning = "running"
	MachineDesiredStopped = "stopped"
	MachineDesiredDeleted = "deleted"

	MachineJobStart     = "start"
	MachineJobStop      = "stop"
	MachineJobReconcile = "reconcile"
	MachineJobDelete    = "delete"
)

type Machine struct {
	ID               string
	Name             string
	TemplateID        string
	TemplateType      string
	TemplateConfigJSON string
	SetupVersion     string
	OptionsJSON      string
	CustomImageID    string
	Status           string
	DesiredStatus    string
	ContainerID      string
	LastError        string
	Ready            bool
	ReadyReportedAt  int64
	ReadyReason      string
	ArcadVersion     string
	MachineToken     string
	UserRole         string
	LastActivityAt   int64
}

const (
	MachineRoleAdmin  = "admin"
	MachineRoleEditor = "editor"
	MachineRoleViewer = "viewer"
	MachineRoleNone   = ""

	GeneralAccessScopeNone      = "none"
	GeneralAccessScopeArcaUsers = "arca_users"
	GeneralAccessScopeAnonymous = "anonymous"

	GeneralAccessRoleNone   = "none"
	GeneralAccessRoleViewer = "viewer"
)

type MachineSharing struct {
	GeneralAccessScope string
	GeneralAccessRole  string
}

type MachineSharingMember struct {
	UserID string
	Email  string
	Role   string
}

type MachineReadiness struct {
	Ready           bool
	ReadyReportedAt int64
	DesiredStatus   string
}

type MachineJob struct {
	ID        string
	MachineID string
	Kind      string
	Attempt   int64
}

type MachineEvent struct {
	ID        string
	MachineID string
	JobID     string
	Level     string
	EventType string
	Message   string
	CreatedAt int64
}

type MachineEventInput struct {
	MachineID string
	JobID     string
	Level     string
	EventType string
	Message   string
}

var ErrMachineNameAlreadyExists = errors.New("machine name already exists")

type CreateMachineOptions struct {
	OptionsJSON    string
	CustomImageID  string
	TemplateType    string
	TemplateConfigJSON string
}

func (s *Store) CreateMachineWithOwner(ctx context.Context, userID, name, templateID, setupVersion string, extra ...string) (Machine, error) {
	opts := CreateMachineOptions{}
	if len(extra) > 0 && extra[0] != "" {
		opts.OptionsJSON = extra[0]
	}
	if len(extra) > 1 {
		opts.CustomImageID = strings.TrimSpace(extra[1])
	}
	if len(extra) > 2 {
		opts.TemplateType = strings.TrimSpace(extra[2])
	}
	if len(extra) > 3 {
		opts.TemplateConfigJSON = strings.TrimSpace(extra[3])
	}
	return s.createMachineWithOwnerOpts(ctx, userID, name, templateID, setupVersion, opts)
}

func (s *Store) createMachineWithOwnerOpts(ctx context.Context, userID, name, templateID, setupVersion string, opts CreateMachineOptions) (Machine, error) {
	machineID, err := randomID()
	if err != nil {
		return Machine{}, err
	}
	machineToken, err := randomToken()
	if err != nil {
		return Machine{}, err
	}
	machineTokenHash := hashToken(machineToken)
	machineTokenID, err := randomID()
	if err != nil {
		return Machine{}, err
	}
	nowUnix := time.Now().Unix()
	templateID = NormalizeMachineTemplate(templateID)
	setupVersion = strings.TrimSpace(setupVersion)
	optionsJSON := opts.OptionsJSON
	if optionsJSON == "" {
		optionsJSON = "{}"
	}
	customImageID := opts.CustomImageID
	templateType := opts.TemplateType
	templateConfigJSON := opts.TemplateConfigJSON
	if templateConfigJSON == "" {
		templateConfigJSON = "{}"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Machine{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	jobID, err := randomID()
	if err != nil {
		return Machine{}, err
	}
	eventID, err := randomID()
	if err != nil {
		return Machine{}, err
	}

	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		if err = q.CreateMachine(ctx, sqlitesqlc.CreateMachineParams{ID: machineID, Name: name, TemplateID: templateID, TemplateType: templateType, TemplateConfigJson: templateConfigJSON, SetupVersion: setupVersion, OptionsJson: optionsJSON, CustomImageID: customImageID}); err != nil {
			if isMachineNameUniqueConstraintError(err) {
				return Machine{}, ErrMachineNameAlreadyExists
			}
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, sqlitesqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "admin",
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineState(ctx, sqlitesqlc.CreateMachineStateParams{
			MachineID:     machineID,
			Status:        MachineStatusPending,
			DesiredStatus: MachineDesiredRunning,
			UpdatedAt:     nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineToken(ctx, sqlitesqlc.CreateMachineTokenParams{
			ID:        machineTokenID,
			MachineID: machineID,
			TokenHash: machineTokenHash,
			Token:     machineToken,
			CreatedAt: nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.EnqueueMachineJob(ctx, sqlitesqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobStart,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineEvent(ctx, sqlitesqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: "start_requested",
			Message:   "machine created and start requested",
			CreatedAt: nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.UpsertMachineSharing(ctx, sqlitesqlc.UpsertMachineSharingParams{
			MachineID:          machineID,
			GeneralAccessScope: GeneralAccessScopeNone,
			GeneralAccessRole:  GeneralAccessRoleNone,
			UpdatedAt:          nowUnix,
		}); err != nil {
			return Machine{}, err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		if err = q.CreateMachine(ctx, postgresqlsqlc.CreateMachineParams{ID: machineID, Name: name, TemplateID: templateID, TemplateType: templateType, TemplateConfigJson: templateConfigJSON, SetupVersion: setupVersion, OptionsJson: optionsJSON, CustomImageID: customImageID}); err != nil {
			if isMachineNameUniqueConstraintError(err) {
				return Machine{}, ErrMachineNameAlreadyExists
			}
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, postgresqlsqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "admin",
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineState(ctx, postgresqlsqlc.CreateMachineStateParams{
			MachineID:     machineID,
			Status:        MachineStatusPending,
			DesiredStatus: MachineDesiredRunning,
			UpdatedAt:     nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineToken(ctx, postgresqlsqlc.CreateMachineTokenParams{
			ID:        machineTokenID,
			MachineID: machineID,
			TokenHash: machineTokenHash,
			Token:     machineToken,
			CreatedAt: nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.EnqueueMachineJob(ctx, postgresqlsqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobStart,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateMachineEvent(ctx, postgresqlsqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: "start_requested",
			Message:   "machine created and start requested",
			CreatedAt: nowUnix,
		}); err != nil {
			return Machine{}, err
		}
		if err = q.UpsertMachineSharing(ctx, postgresqlsqlc.UpsertMachineSharingParams{
			MachineID:          machineID,
			GeneralAccessScope: GeneralAccessScopeNone,
			GeneralAccessRole:  GeneralAccessRoleNone,
			UpdatedAt:          nowUnix,
		}); err != nil {
			return Machine{}, err
		}
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return Machine{}, err
	}

	return Machine{
		ID:               machineID,
		Name:             name,
		TemplateID:        templateID,
		TemplateType:      templateType,
		TemplateConfigJSON: templateConfigJSON,
		SetupVersion:     setupVersion,
		OptionsJSON:      optionsJSON,
		CustomImageID:    customImageID,
		Status:           MachineStatusPending,
		DesiredStatus:    MachineDesiredRunning,
		MachineToken:     machineToken,
	}, nil
}

func (s *Store) ListMachinesByUser(ctx context.Context, userID string) ([]Machine, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachinesAccessibleByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			userRole := row.UserRole
			if userRole == "" {
				userRole = MachineRoleViewer
			}
			machines = append(machines, Machine{
				ID:               row.ID,
				Name:             row.Name,
				TemplateID:        NormalizeMachineTemplate(row.TemplateID),
				TemplateType:      row.TemplateType,
				TemplateConfigJSON: row.TemplateConfigJson,
				SetupVersion:     strings.TrimSpace(row.SetupVersion),
				OptionsJSON:      row.OptionsJson,
				CustomImageID:    row.CustomImageID,
				Status:           row.Status,
				DesiredStatus:    row.DesiredStatus,
				ContainerID:      row.ContainerID,
				LastError:        row.LastError,
				Ready:            row.Ready,
				ReadyReportedAt:  row.ReadyReportedAt,
				ReadyReason:      row.ReadyReason,
				ArcadVersion:     row.ArcadVersion,
				UserRole:         userRole,
			})
		}
		return machines, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachinesAccessibleByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			userRole := row.UserRole
			if userRole == "" {
				userRole = MachineRoleViewer
			}
			machines = append(machines, Machine{
				ID:               row.ID,
				Name:             row.Name,
				TemplateID:        NormalizeMachineTemplate(row.TemplateID),
				TemplateType:      row.TemplateType,
				TemplateConfigJSON: row.TemplateConfigJson,
				SetupVersion:     strings.TrimSpace(row.SetupVersion),
				OptionsJSON:      row.OptionsJson,
				CustomImageID:    row.CustomImageID,
				Status:           row.Status,
				DesiredStatus:    row.DesiredStatus,
				ContainerID:      row.ContainerID,
				LastError:        row.LastError,
				Ready:            row.Ready,
				ReadyReportedAt:  row.ReadyReportedAt,
				ReadyReason:      row.ReadyReason,
				ArcadVersion:     row.ArcadVersion,
				UserRole:         userRole,
			})
		}
		return machines, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateMachineNameByIDForOwner(ctx context.Context, userID, machineID, name string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.UpdateMachineNameByIDForOwner(ctx, sqlitesqlc.UpdateMachineNameByIDForOwnerParams{
			Name:      name,
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil && isMachineNameUniqueConstraintError(err) {
			return false, ErrMachineNameAlreadyExists
		}
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.UpdateMachineNameByIDForOwner(ctx, postgresqlsqlc.UpdateMachineNameByIDForOwnerParams{
			Name:      name,
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil && isMachineNameUniqueConstraintError(err) {
			return false, ErrMachineNameAlreadyExists
		}
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateMachineOptionsByID(ctx context.Context, machineID, optionsJSON string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.UpdateMachineOptionsByID(ctx, sqlitesqlc.UpdateMachineOptionsByIDParams{
			OptionsJson: optionsJSON,
			MachineID:   machineID,
		})
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.UpdateMachineOptionsByID(ctx, postgresqlsqlc.UpdateMachineOptionsByIDParams{
			OptionsJson: optionsJSON,
			MachineID:   machineID,
		})
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) RequestStartMachineByIDForOwner(ctx context.Context, userID, machineID string) (bool, error) {
	return s.requestStateTransition(ctx, userID, machineID, MachineStatusPending, MachineDesiredRunning, MachineJobStart)
}

func (s *Store) RequestStopMachineByIDForOwner(ctx context.Context, userID, machineID string) (bool, error) {
	return s.requestStateTransition(ctx, userID, machineID, MachineStatusStopping, MachineDesiredStopped, MachineJobStop)
}

func (s *Store) RequestDeleteMachineByIDForOwner(ctx context.Context, userID, machineID string) (bool, error) {
	return s.requestStateTransition(ctx, userID, machineID, MachineStatusDeleting, MachineDesiredDeleted, MachineJobDelete)
}

func (s *Store) requestStateTransition(ctx context.Context, userID, machineID, status, desiredStatus, jobKind string) (bool, error) {
	nowUnix := time.Now().Unix()
	jobID, err := randomID()
	if err != nil {
		return false, err
	}
	eventID, err := randomID()
	if err != nil {
		return false, err
	}
	eventType := requestedEventType(jobKind)
	message := "desired state set to " + desiredStatus

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var updated int64
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		updated, err = q.UpdateMachineStateForOwner(ctx, sqlitesqlc.UpdateMachineStateForOwnerParams{
			Status:        status,
			DesiredStatus: desiredStatus,
			UpdatedAt:     nowUnix,
			MachineID:     machineID,
			UserID:        userID,
		})
		if err != nil {
			return false, err
		}
		if updated == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.EnqueueMachineJob(ctx, sqlitesqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      jobKind,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return false, err
		}
		if err = q.CreateMachineEvent(ctx, sqlitesqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: eventType,
			Message:   message,
			CreatedAt: nowUnix,
		}); err != nil {
			return false, err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		updated, err = q.UpdateMachineStateForOwner(ctx, postgresqlsqlc.UpdateMachineStateForOwnerParams{
			Status:        status,
			DesiredStatus: desiredStatus,
			UpdatedAt:     nowUnix,
			MachineID:     machineID,
			UserID:        userID,
		})
		if err != nil {
			return false, err
		}
		if updated == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.EnqueueMachineJob(ctx, postgresqlsqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      jobKind,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return false, err
		}
		if err = q.CreateMachineEvent(ctx, postgresqlsqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: eventType,
			Message:   message,
			CreatedAt: nowUnix,
		}); err != nil {
			return false, err
		}
	default:
		return false, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) GetMachineByID(ctx context.Context, machineID string) (Machine, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineByID(ctx, machineID)
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
			MachineToken:     row.MachineToken,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineByID(ctx, machineID)
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
			MachineToken:     row.MachineToken,
		}, nil
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineByIDForUser(ctx context.Context, userID, machineID string) (Machine, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineByIDForUser(ctx, sqlitesqlc.GetMachineByIDForUserParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineByIDForUser(ctx, postgresqlsqlc.GetMachineByIDForUserParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
		}, nil
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineByName(ctx context.Context, name string) (Machine, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineByName(ctx, name)
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineByName(ctx, name)
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:               row.ID,
			Name:             row.Name,
			TemplateID:        NormalizeMachineTemplate(row.TemplateID),
			TemplateType:      row.TemplateType,
			TemplateConfigJSON: row.TemplateConfigJson,
			SetupVersion:     strings.TrimSpace(row.SetupVersion),
			OptionsJSON:      row.OptionsJson,
			CustomImageID:    row.CustomImageID,
			Status:           row.Status,
			DesiredStatus:    row.DesiredStatus,
			ContainerID:      row.ContainerID,
			LastError:        row.LastError,
			Ready:            row.Ready,
			ReadyReportedAt:  row.ReadyReportedAt,
			ReadyReason:      row.ReadyReason,
			ArcadVersion:     row.ArcadVersion,
		}, nil
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineOwnerUserID(ctx context.Context, machineID string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetMachineOwnerUserID(ctx, machineID)
	case DriverPostgres:
		return s.pgQueries.GetMachineOwnerUserID(ctx, machineID)
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateMachineRuntimeStateByMachineID(ctx context.Context, machineID, status, desiredStatus, containerID, lastError string) error {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpdateMachineRuntimeStateByMachineID(ctx, sqlitesqlc.UpdateMachineRuntimeStateByMachineIDParams{
			Status:        status,
			DesiredStatus: desiredStatus,
			ContainerID:   containerID,
			LastError:     lastError,
			UpdatedAt:     nowUnix,
			MachineID:     machineID,
		})
	case DriverPostgres:
		return s.pgQueries.UpdateMachineRuntimeStateByMachineID(ctx, postgresqlsqlc.UpdateMachineRuntimeStateByMachineIDParams{
			Status:        status,
			DesiredStatus: desiredStatus,
			ContainerID:   containerID,
			LastError:     lastError,
			UpdatedAt:     nowUnix,
			MachineID:     machineID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) RecoverExpiredMachineJobs(ctx context.Context, nowUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		_, err := s.sqliteQueries.RecoverExpiredMachineJobs(ctx, sqlitesqlc.RecoverExpiredMachineJobsParams{
			UpdatedAt: nowUnix,
			NowUnix:   sql.NullInt64{Int64: nowUnix, Valid: true},
		})
		return err
	case DriverPostgres:
		_, err := s.pgQueries.RecoverExpiredMachineJobs(ctx, postgresqlsqlc.RecoverExpiredMachineJobsParams{
			UpdatedAt: nowUnix,
			NowUnix:   sql.NullInt64{Int64: nowUnix, Valid: true},
		})
		return err
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ExtendMachineJobLease(ctx context.Context, jobID string, leaseOwner string, leaseUntil int64, nowUnix int64) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ExtendMachineJobLease(ctx, sqlitesqlc.ExtendMachineJobLeaseParams{
			LeaseUntil: sql.NullInt64{Int64: leaseUntil, Valid: true},
			UpdatedAt:  nowUnix,
			ID:         jobID,
			LeaseOwner: sql.NullString{String: leaseOwner, Valid: true},
		})
		return rows > 0, err
	case DriverPostgres:
		rows, err := s.pgQueries.ExtendMachineJobLease(ctx, postgresqlsqlc.ExtendMachineJobLeaseParams{
			LeaseUntil: sql.NullInt64{Int64: leaseUntil, Valid: true},
			UpdatedAt:  nowUnix,
			ID:         jobID,
			LeaseOwner: sql.NullString{String: leaseOwner, Valid: true},
		})
		return rows > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListMachinesByDesiredStatus(ctx context.Context, desiredStatus string, limit int64) ([]Machine, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachinesByDesiredStatus(ctx, sqlitesqlc.ListMachinesByDesiredStatusParams{
			DesiredStatus: desiredStatus,
			LimitN:        limit,
		})
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			machines = append(machines, Machine{
				ID:               row.ID,
				Name:             row.Name,
				TemplateID:        NormalizeMachineTemplate(row.TemplateID),
				TemplateType:      row.TemplateType,
				TemplateConfigJSON: row.TemplateConfigJson,
				OptionsJSON:      row.OptionsJson,
				CustomImageID:    row.CustomImageID,
				Status:           row.Status,
				DesiredStatus:    row.DesiredStatus,
				ContainerID:      row.ContainerID,
				LastError:        row.LastError,
				Ready:            row.Ready,
				ReadyReportedAt:  row.ReadyReportedAt,
				ReadyReason:      row.ReadyReason,
				ArcadVersion:     row.ArcadVersion,
				LastActivityAt:   row.LastActivityAt,
			})
		}
		return machines, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachinesByDesiredStatus(ctx, postgresqlsqlc.ListMachinesByDesiredStatusParams{
			DesiredStatus: desiredStatus,
			LimitN:        int32(limit),
		})
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			machines = append(machines, Machine{
				ID:               row.ID,
				Name:             row.Name,
				TemplateID:        NormalizeMachineTemplate(row.TemplateID),
				TemplateType:      row.TemplateType,
				TemplateConfigJSON: row.TemplateConfigJson,
				OptionsJSON:      row.OptionsJson,
				CustomImageID:    row.CustomImageID,
				Status:           row.Status,
				DesiredStatus:    row.DesiredStatus,
				ContainerID:      row.ContainerID,
				LastError:        row.LastError,
				Ready:            row.Ready,
				ReadyReportedAt:  row.ReadyReportedAt,
				ReadyReason:      row.ReadyReason,
				ArcadVersion:     row.ArcadVersion,
				LastActivityAt:   row.LastActivityAt,
			})
		}
		return machines, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ReportMachineReadinessByMachineID(ctx context.Context, machineID string, ready bool, reason, containerID, arcadVersion string) (bool, error) {
	nowUnix := time.Now().Unix()
	reason = strings.TrimSpace(reason)
	containerID = strings.TrimSpace(containerID)
	arcadVersion = strings.TrimSpace(arcadVersion)
	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.ReportMachineReadinessByMachineID(ctx, sqlitesqlc.ReportMachineReadinessByMachineIDParams{
			Ready:           ready,
			ReadyReportedAt: nowUnix,
			ReadyReason:     reason,
			ContainerID:     containerID,
			ArcadVersion:    arcadVersion,
			UpdatedAt:       nowUnix,
			MachineID:       machineID,
		})
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.ReportMachineReadinessByMachineID(ctx, postgresqlsqlc.ReportMachineReadinessByMachineIDParams{
			Ready:           ready,
			ReadyReportedAt: nowUnix,
			ReadyReason:     reason,
			ContainerID:     containerID,
			ArcadVersion:    arcadVersion,
			UpdatedAt:       nowUnix,
			MachineID:       machineID,
		})
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineReadinessByMachineID(ctx context.Context, machineID string) (MachineReadiness, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineReadinessByMachineID(ctx, machineID)
		if err != nil {
			return MachineReadiness{}, err
		}
		return MachineReadiness{
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			DesiredStatus:   row.DesiredStatus,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineReadinessByMachineID(ctx, machineID)
		if err != nil {
			return MachineReadiness{}, err
		}
		return MachineReadiness{
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			DesiredStatus:   row.DesiredStatus,
		}, nil
	default:
		return MachineReadiness{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) HasActiveStartOrReconcileJob(ctx context.Context, machineID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		count, err := s.sqliteQueries.CountActiveStartOrReconcileJobsByMachineID(ctx, machineID)
		return count > 0, err
	case DriverPostgres:
		count, err := s.pgQueries.CountActiveStartOrReconcileJobsByMachineID(ctx, machineID)
		return count > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) EnqueueReconcileMachineJob(ctx context.Context, machineID string, nowUnix int64) error {
	jobID, err := randomID()
	if err != nil {
		return err
	}
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.EnqueueMachineJob(ctx, sqlitesqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobReconcile,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.EnqueueMachineJob(ctx, postgresqlsqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobReconcile,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ClaimNextMachineJob(ctx context.Context, leaseOwner string, leaseUntil int64, nowUnix int64) (MachineJob, bool, error) {
	switch s.driver {
	case DriverSQLite:
		jobs, err := s.sqliteQueries.ListRunnableMachineJobs(ctx, sqlitesqlc.ListRunnableMachineJobsParams{
			NowUnix: nowUnix,
			LimitN:  20,
		})
		if err != nil {
			return MachineJob{}, false, err
		}
		for _, job := range jobs {
			claimed, claimErr := s.sqliteQueries.ClaimMachineJob(ctx, sqlitesqlc.ClaimMachineJobParams{
				LeaseOwner: sql.NullString{String: leaseOwner, Valid: true},
				LeaseUntil: sql.NullInt64{Int64: leaseUntil, Valid: true},
				UpdatedAt:  nowUnix,
				ID:         job.ID,
			})
			if claimErr != nil {
				return MachineJob{}, false, claimErr
			}
			if claimed > 0 {
				return MachineJob{ID: job.ID, MachineID: job.MachineID, Kind: job.Kind, Attempt: int64(job.Attempt)}, true, nil
			}
		}
		return MachineJob{}, false, nil
	case DriverPostgres:
		jobs, err := s.pgQueries.ListRunnableMachineJobs(ctx, postgresqlsqlc.ListRunnableMachineJobsParams{
			NowUnix: nowUnix,
			LimitN:  20,
		})
		if err != nil {
			return MachineJob{}, false, err
		}
		for _, job := range jobs {
			claimed, claimErr := s.pgQueries.ClaimMachineJob(ctx, postgresqlsqlc.ClaimMachineJobParams{
				LeaseOwner: sql.NullString{String: leaseOwner, Valid: true},
				LeaseUntil: sql.NullInt64{Int64: leaseUntil, Valid: true},
				UpdatedAt:  nowUnix,
				ID:         job.ID,
			})
			if claimErr != nil {
				return MachineJob{}, false, claimErr
			}
			if claimed > 0 {
				return MachineJob{ID: job.ID, MachineID: job.MachineID, Kind: job.Kind, Attempt: int64(job.Attempt)}, true, nil
			}
		}
		return MachineJob{}, false, nil
	default:
		return MachineJob{}, false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) MarkMachineJobSucceeded(ctx context.Context, jobID string, nowUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.MarkMachineJobSucceeded(ctx, sqlitesqlc.MarkMachineJobSucceededParams{UpdatedAt: nowUnix, ID: jobID})
	case DriverPostgres:
		return s.pgQueries.MarkMachineJobSucceeded(ctx, postgresqlsqlc.MarkMachineJobSucceededParams{UpdatedAt: nowUnix, ID: jobID})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) RequeueMachineJob(ctx context.Context, jobID string, nextRunAt int64, lastError string, nowUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.RequeueMachineJob(ctx, sqlitesqlc.RequeueMachineJobParams{
			NextRunAt: nextRunAt,
			LastError: sql.NullString{String: lastError, Valid: true},
			UpdatedAt: nowUnix,
			ID:        jobID,
		})
	case DriverPostgres:
		return s.pgQueries.RequeueMachineJob(ctx, postgresqlsqlc.RequeueMachineJobParams{
			NextRunAt: nextRunAt,
			LastError: sql.NullString{String: lastError, Valid: true},
			UpdatedAt: nowUnix,
			ID:        jobID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteMachineByIDForOwner(ctx context.Context, userID, machineID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var deleted int64
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		deleted, err = q.DeleteUserMachineByMachineIDForOwner(ctx, sqlitesqlc.DeleteUserMachineByMachineIDForOwnerParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return false, err
		}
		if deleted == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.DeleteMachineIfNoUsers(ctx, machineID); err != nil {
			return false, err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		deleted, err = q.DeleteUserMachineByMachineIDForOwner(ctx, postgresqlsqlc.DeleteUserMachineByMachineIDForOwnerParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return false, err
		}
		if deleted == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.DeleteMachineIfNoUsers(ctx, machineID); err != nil {
			return false, err
		}
	default:
		return false, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) DeleteMachineByID(ctx context.Context, machineID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		deleted, err := s.sqliteQueries.DeleteMachineByID(ctx, machineID)
		return deleted > 0, err
	case DriverPostgres:
		deleted, err := s.pgQueries.DeleteMachineByID(ctx, machineID)
		return deleted > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateMachineEvent(ctx context.Context, input MachineEventInput) error {
	eventID, err := randomID()
	if err != nil {
		return err
	}
	nowUnix := time.Now().Unix()
	level := strings.TrimSpace(input.Level)
	if level == "" {
		level = "info"
	}

	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateMachineEvent(ctx, sqlitesqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: input.MachineID,
			JobID:     strings.TrimSpace(input.JobID),
			Level:     level,
			EventType: strings.TrimSpace(input.EventType),
			Message:   strings.TrimSpace(input.Message),
			CreatedAt: nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.CreateMachineEvent(ctx, postgresqlsqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: input.MachineID,
			JobID:     strings.TrimSpace(input.JobID),
			Level:     level,
			EventType: strings.TrimSpace(input.EventType),
			Message:   strings.TrimSpace(input.Message),
			CreatedAt: nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListMachineEventsByMachineIDForUser(ctx context.Context, userID, machineID string, limit int64) ([]MachineEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineEventsByMachineIDForUser(ctx, sqlitesqlc.ListMachineEventsByMachineIDForUserParams{
			MachineID: machineID,
			UserID:    userID,
			LimitN:    limit,
		})
		if err != nil {
			return nil, err
		}
		events := make([]MachineEvent, 0, len(rows))
		for _, row := range rows {
			events = append(events, MachineEvent{
				ID:        row.ID,
				MachineID: row.MachineID,
				JobID:     row.JobID,
				Level:     row.Level,
				EventType: row.EventType,
				Message:   row.Message,
				CreatedAt: row.CreatedAt,
			})
		}
		return events, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineEventsByMachineIDForUser(ctx, postgresqlsqlc.ListMachineEventsByMachineIDForUserParams{
			MachineID: machineID,
			UserID:    userID,
			LimitN:    int32(limit),
		})
		if err != nil {
			return nil, err
		}
		events := make([]MachineEvent, 0, len(rows))
		for _, row := range rows {
			events = append(events, MachineEvent{
				ID:        row.ID,
				MachineID: row.MachineID,
				JobID:     row.JobID,
				Level:     row.Level,
				EventType: row.EventType,
				Message:   row.Message,
				CreatedAt: row.CreatedAt,
			})
		}
		return events, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func requestedEventType(jobKind string) string {
	switch jobKind {
	case MachineJobStart:
		return "start_requested"
	case MachineJobStop:
		return "stop_requested"
	case MachineJobDelete:
		return "delete_requested"
	case MachineJobReconcile:
		return "reconcile_requested"
	default:
		return "state_change_requested"
	}
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) ResolveMachineRole(ctx context.Context, userID, machineID string) string {
	if userID == "" {
		// Anonymous: check general_access_scope for anonymous
		sharing, err := s.GetMachineSharing(ctx, machineID)
		if err != nil {
			return MachineRoleNone
		}
		if sharing.GeneralAccessScope == GeneralAccessScopeAnonymous && sharing.GeneralAccessRole == GeneralAccessRoleViewer {
			return MachineRoleViewer
		}
		return MachineRoleNone
	}
	// Check individual role
	role, err := s.GetUserMachineRole(ctx, userID, machineID)
	if err == nil && role != "" {
		return role
	}
	// Check group-based access
	groupRole, err := s.GetMachineGroupRoleByUserID(ctx, machineID, userID)
	if err == nil && groupRole != "" {
		return groupRole
	}
	// Check general access for arca users
	sharing, err := s.GetMachineSharing(ctx, machineID)
	if err != nil {
		return MachineRoleNone
	}
	if sharing.GeneralAccessScope == GeneralAccessScopeArcaUsers && sharing.GeneralAccessRole == GeneralAccessRoleViewer {
		return MachineRoleViewer
	}
	if sharing.GeneralAccessScope == GeneralAccessScopeAnonymous && sharing.GeneralAccessRole == GeneralAccessRoleViewer {
		return MachineRoleViewer
	}
	return MachineRoleNone
}

func (s *Store) GetUserMachineRole(ctx context.Context, userID, machineID string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetUserMachineRole(ctx, sqlitesqlc.GetUserMachineRoleParams{
			UserID:    userID,
			MachineID: machineID,
		})
	case DriverPostgres:
		return s.pgQueries.GetUserMachineRole(ctx, postgresqlsqlc.GetUserMachineRoleParams{
			UserID:    userID,
			MachineID: machineID,
		})
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineSharing(ctx context.Context, machineID string) (MachineSharing, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineSharingByMachineID(ctx, machineID)
		if err != nil {
			return MachineSharing{}, err
		}
		return MachineSharing{
			GeneralAccessScope: row.GeneralAccessScope,
			GeneralAccessRole:  row.GeneralAccessRole,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineSharingByMachineID(ctx, machineID)
		if err != nil {
			return MachineSharing{}, err
		}
		return MachineSharing{
			GeneralAccessScope: row.GeneralAccessScope,
			GeneralAccessRole:  row.GeneralAccessRole,
		}, nil
	default:
		return MachineSharing{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertMachineSharing(ctx context.Context, machineID string, sharing MachineSharing) error {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertMachineSharing(ctx, sqlitesqlc.UpsertMachineSharingParams{
			MachineID:          machineID,
			GeneralAccessScope: sharing.GeneralAccessScope,
			GeneralAccessRole:  sharing.GeneralAccessRole,
			UpdatedAt:          nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertMachineSharing(ctx, postgresqlsqlc.UpsertMachineSharingParams{
			MachineID:          machineID,
			GeneralAccessScope: sharing.GeneralAccessScope,
			GeneralAccessRole:  sharing.GeneralAccessRole,
			UpdatedAt:          nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListUserMachinesByMachineID(ctx context.Context, machineID string) ([]MachineSharingMember, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListUserMachinesByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		members := make([]MachineSharingMember, 0, len(rows))
		for _, row := range rows {
			members = append(members, MachineSharingMember{
				UserID: row.UserID,
				Email:  row.Email,
				Role:   row.Role,
			})
		}
		return members, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListUserMachinesByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		members := make([]MachineSharingMember, 0, len(rows))
		for _, row := range rows {
			members = append(members, MachineSharingMember{
				UserID: row.UserID,
				Email:  row.Email,
				Role:   row.Role,
			})
		}
		return members, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertUserMachine(ctx context.Context, userID, machineID, role string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertUserMachine(ctx, sqlitesqlc.UpsertUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      role,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertUserMachine(ctx, postgresqlsqlc.UpsertUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      role,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteUserMachine(ctx context.Context, userID, machineID string) error {
	switch s.driver {
	case DriverSQLite:
		_, err := s.sqliteQueries.DeleteUserMachine(ctx, sqlitesqlc.DeleteUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
		})
		return err
	case DriverPostgres:
		_, err := s.pgQueries.DeleteUserMachine(ctx, postgresqlsqlc.DeleteUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
		})
		return err
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListMachineEventsByMachineID(ctx context.Context, machineID string, limit int64) ([]MachineEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineEventsByMachineID(ctx, sqlitesqlc.ListMachineEventsByMachineIDParams{
			MachineID: machineID,
			LimitN:    limit,
		})
		if err != nil {
			return nil, err
		}
		events := make([]MachineEvent, 0, len(rows))
		for _, row := range rows {
			events = append(events, MachineEvent{
				ID: row.ID, MachineID: row.MachineID, JobID: row.JobID,
				Level: row.Level, EventType: row.EventType, Message: row.Message, CreatedAt: row.CreatedAt,
			})
		}
		return events, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineEventsByMachineID(ctx, postgresqlsqlc.ListMachineEventsByMachineIDParams{
			MachineID: machineID,
			LimitN:    int32(limit),
		})
		if err != nil {
			return nil, err
		}
		events := make([]MachineEvent, 0, len(rows))
		for _, row := range rows {
			events = append(events, MachineEvent{
				ID: row.ID, MachineID: row.MachineID, JobID: row.JobID,
				Level: row.Level, EventType: row.EventType, Message: row.Message, CreatedAt: row.CreatedAt,
			})
		}
		return events, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateMachineLastActivityAt(ctx context.Context, machineID string) error {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpdateMachineLastActivityAt(ctx, sqlitesqlc.UpdateMachineLastActivityAtParams{
			LastActivityAt: nowUnix,
			MachineID:      machineID,
		})
	case DriverPostgres:
		return s.pgQueries.UpdateMachineLastActivityAt(ctx, postgresqlsqlc.UpdateMachineLastActivityAtParams{
			LastActivityAt: nowUnix,
			MachineID:      machineID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) RequestSystemStopMachine(ctx context.Context, machineID string) (bool, error) {
	nowUnix := time.Now().Unix()
	jobID, err := randomID()
	if err != nil {
		return false, err
	}
	eventID, err := randomID()
	if err != nil {
		return false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var updated int64
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		updated, err = q.RequestSystemStopMachine(ctx, sqlitesqlc.RequestSystemStopMachineParams{
			UpdatedAt: nowUnix,
			MachineID: machineID,
		})
		if err != nil {
			return false, err
		}
		if updated == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.EnqueueMachineJob(ctx, sqlitesqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobStop,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return false, err
		}
		if err = q.CreateMachineEvent(ctx, sqlitesqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: "auto_stop_requested",
			Message:   "machine auto-stop requested due to idle timeout",
			CreatedAt: nowUnix,
		}); err != nil {
			return false, err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		updated, err = q.RequestSystemStopMachine(ctx, postgresqlsqlc.RequestSystemStopMachineParams{
			UpdatedAt: nowUnix,
			MachineID: machineID,
		})
		if err != nil {
			return false, err
		}
		if updated == 0 {
			if err = tx.Rollback(); err != nil && err != sql.ErrTxDone {
				return false, err
			}
			return false, nil
		}
		if err = q.EnqueueMachineJob(ctx, postgresqlsqlc.EnqueueMachineJobParams{
			ID:        jobID,
			MachineID: machineID,
			Kind:      MachineJobStop,
			NextRunAt: nowUnix,
			NowUnix:   nowUnix,
		}); err != nil {
			return false, err
		}
		if err = q.CreateMachineEvent(ctx, postgresqlsqlc.CreateMachineEventParams{
			ID:        eventID,
			MachineID: machineID,
			JobID:     jobID,
			Level:     "info",
			EventType: "auto_stop_requested",
			Message:   "machine auto-stop requested due to idle timeout",
			CreatedAt: nowUnix,
		}); err != nil {
			return false, err
		}
	default:
		return false, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type AccessRequest struct {
	ID            string
	MachineID     string
	UserID        string
	Email         string
	Status        string
	RequestedRole string
	Message       string
	CreatedAt     int64
	ResolvedAt    int64
}

func (s *Store) CreateMachineAccessRequest(ctx context.Context, machineID, userID, requestedRole, message string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateMachineAccessRequest(ctx, sqlitesqlc.CreateMachineAccessRequestParams{
			ID:            id,
			MachineID:     machineID,
			UserID:        userID,
			RequestedRole: requestedRole,
			Message:       message,
			CreatedAt:     nowUnix,
		})
	case DriverPostgres:
		return s.pgQueries.CreateMachineAccessRequest(ctx, postgresqlsqlc.CreateMachineAccessRequestParams{
			ID:            id,
			MachineID:     machineID,
			UserID:        userID,
			RequestedRole: requestedRole,
			Message:       message,
			CreatedAt:     nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetPendingMachineAccessRequest(ctx context.Context, machineID, userID string) (AccessRequest, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetPendingMachineAccessRequest(ctx, sqlitesqlc.GetPendingMachineAccessRequestParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return AccessRequest{}, err
		}
		return AccessRequest{
			ID:            row.ID,
			MachineID:     row.MachineID,
			UserID:        row.UserID,
			Status:        row.Status,
			RequestedRole: row.RequestedRole,
			Message:       row.Message,
			CreatedAt:     row.CreatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetPendingMachineAccessRequest(ctx, postgresqlsqlc.GetPendingMachineAccessRequestParams{
			MachineID: machineID,
			UserID:    userID,
		})
		if err != nil {
			return AccessRequest{}, err
		}
		return AccessRequest{
			ID:            row.ID,
			MachineID:     row.MachineID,
			UserID:        row.UserID,
			Status:        row.Status,
			RequestedRole: row.RequestedRole,
			Message:       row.Message,
			CreatedAt:     row.CreatedAt,
		}, nil
	default:
		return AccessRequest{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListPendingMachineAccessRequests(ctx context.Context, machineID string) ([]AccessRequest, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListPendingMachineAccessRequestsByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		requests := make([]AccessRequest, 0, len(rows))
		for _, row := range rows {
			requests = append(requests, AccessRequest{
				ID:            row.ID,
				MachineID:     row.MachineID,
				UserID:        row.UserID,
				Email:         row.Email,
				Status:        row.Status,
				RequestedRole: row.RequestedRole,
				Message:       row.Message,
				CreatedAt:     row.CreatedAt,
			})
		}
		return requests, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListPendingMachineAccessRequestsByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		requests := make([]AccessRequest, 0, len(rows))
		for _, row := range rows {
			requests = append(requests, AccessRequest{
				ID:            row.ID,
				MachineID:     row.MachineID,
				UserID:        row.UserID,
				Email:         row.Email,
				Status:        row.Status,
				RequestedRole: row.RequestedRole,
				Message:       row.Message,
				CreatedAt:     row.CreatedAt,
			})
		}
		return requests, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ResolveMachineAccessRequest(ctx context.Context, requestID, status, resolvedByUserID, resolvedRole string) (int64, error) {
	nowUnix := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.ResolveMachineAccessRequest(ctx, sqlitesqlc.ResolveMachineAccessRequestParams{
			Status:           status,
			ResolvedByUserID: sql.NullString{String: resolvedByUserID, Valid: resolvedByUserID != ""},
			ResolvedRole:     resolvedRole,
			ResolvedAt:       sql.NullInt64{Int64: nowUnix, Valid: true},
			ID:               requestID,
		})
	case DriverPostgres:
		return s.pgQueries.ResolveMachineAccessRequest(ctx, postgresqlsqlc.ResolveMachineAccessRequestParams{
			Status:           status,
			ResolvedByUserID: sql.NullString{String: resolvedByUserID, Valid: resolvedByUserID != ""},
			ResolvedRole:     resolvedRole,
			ResolvedAt:       sql.NullInt64{Int64: nowUnix, Valid: true},
			ID:               requestID,
		})
	default:
		return 0, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineAccessRequestByID(ctx context.Context, requestID string) (AccessRequest, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineAccessRequestByID(ctx, requestID)
		if err != nil {
			return AccessRequest{}, err
		}
		var resolvedAt int64
		if row.ResolvedAt.Valid {
			resolvedAt = row.ResolvedAt.Int64
		}
		return AccessRequest{
			ID:         row.ID,
			MachineID:  row.MachineID,
			UserID:     row.UserID,
			Status:     row.Status,
			Message:    row.Message,
			CreatedAt:  row.CreatedAt,
			ResolvedAt: resolvedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineAccessRequestByID(ctx, requestID)
		if err != nil {
			return AccessRequest{}, err
		}
		var resolvedAt int64
		if row.ResolvedAt.Valid {
			resolvedAt = row.ResolvedAt.Int64
		}
		return AccessRequest{
			ID:         row.ID,
			MachineID:  row.MachineID,
			UserID:     row.UserID,
			Status:     row.Status,
			Message:    row.Message,
			CreatedAt:  row.CreatedAt,
			ResolvedAt: resolvedAt,
		}, nil
	default:
		return AccessRequest{}, unsupportedDriverError(s.driver)
	}
}

func isMachineNameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(strings.ToLower(pgErr.Message), "name")
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "unique constraint failed") && strings.Contains(msg, "machines.name") {
		return true
	}
	return strings.Contains(msg, "duplicate key value") && strings.Contains(msg, "name")
}
