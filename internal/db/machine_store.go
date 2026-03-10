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
	ID              string
	Name            string
	RuntimeID       string
	SetupVersion    string
	Endpoint        string
	Status          string
	DesiredStatus   string
	ContainerID     string
	LastError       string
	Ready           bool
	ReadyReportedAt int64
	ReadyReason     string
	MachineToken    string
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

func (s *Store) CreateMachineWithOwner(ctx context.Context, userID, name, runtimeID, setupVersion string) (Machine, error) {
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
	runtimeID = NormalizeMachineRuntime(runtimeID)
	setupVersion = strings.TrimSpace(setupVersion)

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
		if err = q.CreateMachine(ctx, sqlitesqlc.CreateMachineParams{ID: machineID, Name: name, RuntimeID: runtimeID, SetupVersion: setupVersion}); err != nil {
			if isMachineNameUniqueConstraintError(err) {
				return Machine{}, ErrMachineNameAlreadyExists
			}
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, sqlitesqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "owner",
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
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		if err = q.CreateMachine(ctx, postgresqlsqlc.CreateMachineParams{ID: machineID, Name: name, RuntimeID: runtimeID, SetupVersion: setupVersion}); err != nil {
			if isMachineNameUniqueConstraintError(err) {
				return Machine{}, ErrMachineNameAlreadyExists
			}
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, postgresqlsqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "owner",
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
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return Machine{}, err
	}

	return Machine{
		ID:            machineID,
		Name:          name,
		RuntimeID:     runtimeID,
		SetupVersion:  setupVersion,
		Endpoint:      "",
		Status:        MachineStatusPending,
		DesiredStatus: MachineDesiredRunning,
		MachineToken:  machineToken,
	}, nil
}

func (s *Store) ListMachinesByUser(ctx context.Context, userID string) ([]Machine, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachinesByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			machines = append(machines, Machine{
				ID:              row.ID,
				Name:            row.Name,
				RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
				SetupVersion:    strings.TrimSpace(row.SetupVersion),
				Endpoint:        strings.TrimSpace(row.Endpoint),
				Status:          row.Status,
				DesiredStatus:   row.DesiredStatus,
				ContainerID:     row.ContainerID,
				LastError:       row.LastError,
				Ready:           row.Ready,
				ReadyReportedAt: row.ReadyReportedAt,
				ReadyReason:     row.ReadyReason,
			})
		}
		return machines, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachinesByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		machines := make([]Machine, 0, len(rows))
		for _, row := range rows {
			machines = append(machines, Machine{
				ID:              row.ID,
				Name:            row.Name,
				RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
				SetupVersion:    strings.TrimSpace(row.SetupVersion),
				Endpoint:        strings.TrimSpace(row.Endpoint),
				Status:          row.Status,
				DesiredStatus:   row.DesiredStatus,
				ContainerID:     row.ContainerID,
				LastError:       row.LastError,
				Ready:           row.Ready,
				ReadyReportedAt: row.ReadyReportedAt,
				ReadyReason:     row.ReadyReason,
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

func (s *Store) UpdateMachineRuntimeByIDForOwner(ctx context.Context, userID, machineID, runtimeID, setupVersion string) (bool, error) {
	runtimeID = NormalizeMachineRuntime(runtimeID)
	setupVersion = strings.TrimSpace(setupVersion)
	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.UpdateMachineRuntimeByIDForOwner(ctx, sqlitesqlc.UpdateMachineRuntimeByIDForOwnerParams{
			RuntimeID:    runtimeID,
			SetupVersion: setupVersion,
			MachineID:    machineID,
			UserID:       userID,
		})
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.UpdateMachineRuntimeByIDForOwner(ctx, postgresqlsqlc.UpdateMachineRuntimeByIDForOwnerParams{
			RuntimeID:    runtimeID,
			SetupVersion: setupVersion,
			MachineID:    machineID,
			UserID:       userID,
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
			ID:              row.ID,
			Name:            row.Name,
			RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
			SetupVersion:    strings.TrimSpace(row.SetupVersion),
			Endpoint:        strings.TrimSpace(row.Endpoint),
			Status:          row.Status,
			DesiredStatus:   row.DesiredStatus,
			ContainerID:     row.ContainerID,
			LastError:       row.LastError,
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			ReadyReason:     row.ReadyReason,
			MachineToken:    row.MachineToken,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineByID(ctx, machineID)
		if err != nil {
			return Machine{}, err
		}
		return Machine{
			ID:              row.ID,
			Name:            row.Name,
			RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
			SetupVersion:    strings.TrimSpace(row.SetupVersion),
			Endpoint:        strings.TrimSpace(row.Endpoint),
			Status:          row.Status,
			DesiredStatus:   row.DesiredStatus,
			ContainerID:     row.ContainerID,
			LastError:       row.LastError,
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			ReadyReason:     row.ReadyReason,
			MachineToken:    row.MachineToken,
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
			ID:              row.ID,
			Name:            row.Name,
			RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
			SetupVersion:    strings.TrimSpace(row.SetupVersion),
			Endpoint:        strings.TrimSpace(row.Endpoint),
			Status:          row.Status,
			DesiredStatus:   row.DesiredStatus,
			ContainerID:     row.ContainerID,
			LastError:       row.LastError,
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			ReadyReason:     row.ReadyReason,
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
			ID:              row.ID,
			Name:            row.Name,
			RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
			SetupVersion:    strings.TrimSpace(row.SetupVersion),
			Endpoint:        strings.TrimSpace(row.Endpoint),
			Status:          row.Status,
			DesiredStatus:   row.DesiredStatus,
			ContainerID:     row.ContainerID,
			LastError:       row.LastError,
			Ready:           row.Ready,
			ReadyReportedAt: row.ReadyReportedAt,
			ReadyReason:     row.ReadyReason,
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

func (s *Store) UpdateMachineEndpointByID(ctx context.Context, machineID, endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpdateMachineEndpointByID(ctx, sqlitesqlc.UpdateMachineEndpointByIDParams{
			Endpoint:  endpoint,
			MachineID: machineID,
		})
	case DriverPostgres:
		return s.pgQueries.UpdateMachineEndpointByID(ctx, postgresqlsqlc.UpdateMachineEndpointByIDParams{
			Endpoint:  endpoint,
			MachineID: machineID,
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
				ID:              row.ID,
				Name:            row.Name,
				RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
				Status:          row.Status,
				DesiredStatus:   row.DesiredStatus,
				ContainerID:     row.ContainerID,
				LastError:       row.LastError,
				Ready:           row.Ready,
				ReadyReportedAt: row.ReadyReportedAt,
				ReadyReason:     row.ReadyReason,
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
				ID:              row.ID,
				Name:            row.Name,
				RuntimeID:       NormalizeMachineRuntime(row.RuntimeID),
				Status:          row.Status,
				DesiredStatus:   row.DesiredStatus,
				ContainerID:     row.ContainerID,
				LastError:       row.LastError,
				Ready:           row.Ready,
				ReadyReportedAt: row.ReadyReportedAt,
				ReadyReason:     row.ReadyReason,
			})
		}
		return machines, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ReportMachineReadinessByMachineID(ctx context.Context, machineID string, ready bool, reason, containerID string) (bool, error) {
	nowUnix := time.Now().Unix()
	reason = strings.TrimSpace(reason)
	containerID = strings.TrimSpace(containerID)
	switch s.driver {
	case DriverSQLite:
		updated, err := s.sqliteQueries.ReportMachineReadinessByMachineID(ctx, sqlitesqlc.ReportMachineReadinessByMachineIDParams{
			Ready:           ready,
			ReadyReportedAt: nowUnix,
			ReadyReason:     reason,
			ContainerID:     containerID,
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
