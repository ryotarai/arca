package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type SetupState struct {
	Completed             bool
	AdminUserID           string
	BaseDomain            string
	DomainPrefix          string
	CloudflareAPIToken    string
	CloudflareZoneID      string
	DockerProviderEnabled bool
	UpdatedAtUnix         int64
}

type VerifiedTicket struct {
	UserID     string
	UserEmail  string
	MachineID  string
	ExposureID string
}

type MachineTunnel struct {
	MachineID   string
	AccountID   string
	TunnelID    string
	TunnelName  string
	TunnelToken string
	CreatedAt   int64
	UpdatedAt   int64
}

type MachineExposure struct {
	ID        string
	MachineID string
	Name      string
	Hostname  string
	Service   string
	IsPublic  bool
	CreatedAt int64
	UpdatedAt int64
}

func (s *Store) GetSetupState(ctx context.Context) (SetupState, error) {
	zoneID, err := s.getMetaValue(ctx, setupMetaCloudflareZoneID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}

	switch s.driver {
	case DriverSQLite:
		state, err := s.sqliteQueries.GetSetupState(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return SetupState{}, nil
			}
			return SetupState{}, err
		}
		return SetupState{
			Completed:             state.Completed,
			AdminUserID:           state.AdminUserID.String,
			BaseDomain:            state.BaseDomain,
			DomainPrefix:          state.DomainPrefix,
			CloudflareAPIToken:    state.CloudflareApiToken,
			CloudflareZoneID:      zoneID,
			DockerProviderEnabled: state.DockerProviderEnabled,
			UpdatedAtUnix:         state.UpdatedAt,
		}, nil
	case DriverPostgres:
		state, err := s.pgQueries.GetSetupState(ctx)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return SetupState{}, nil
			}
			return SetupState{}, err
		}
		return SetupState{
			Completed:             state.Completed,
			AdminUserID:           state.AdminUserID.String,
			BaseDomain:            state.BaseDomain,
			DomainPrefix:          state.DomainPrefix,
			CloudflareAPIToken:    state.CloudflareApiToken,
			CloudflareZoneID:      zoneID,
			DockerProviderEnabled: state.DockerProviderEnabled,
			UpdatedAtUnix:         state.UpdatedAt,
		}, nil
	default:
		return SetupState{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertSetupState(ctx context.Context, state SetupState) error {
	nowUnix := time.Now().Unix()
	adminUserID := sql.NullString{String: strings.TrimSpace(state.AdminUserID), Valid: strings.TrimSpace(state.AdminUserID) != ""}

	var err error
	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.UpsertSetupState(ctx, sqlitesqlc.UpsertSetupStateParams{
			Completed:             state.Completed,
			AdminUserID:           adminUserID,
			BaseDomain:            strings.TrimSpace(state.BaseDomain),
			DomainPrefix:          strings.TrimSpace(state.DomainPrefix),
			CloudflareApiToken:    state.CloudflareAPIToken,
			DockerProviderEnabled: state.DockerProviderEnabled,
			UpdatedAt:             nowUnix,
		})
	case DriverPostgres:
		err = s.pgQueries.UpsertSetupState(ctx, postgresqlsqlc.UpsertSetupStateParams{
			Completed:             state.Completed,
			AdminUserID:           adminUserID,
			BaseDomain:            strings.TrimSpace(state.BaseDomain),
			DomainPrefix:          strings.TrimSpace(state.DomainPrefix),
			CloudflareApiToken:    state.CloudflareAPIToken,
			DockerProviderEnabled: state.DockerProviderEnabled,
			UpdatedAt:             nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}

	if err != nil {
		return err
	}

	return s.upsertMetaValue(ctx, setupMetaCloudflareZoneID, strings.TrimSpace(state.CloudflareZoneID))
}

const setupMetaCloudflareZoneID = "setup.cloudflare_zone_id"

func (s *Store) getMetaValue(ctx context.Context, key string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetMeta(ctx, key)
	case DriverPostgres:
		return s.pgQueries.GetMeta(ctx, key)
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) upsertMetaValue(ctx context.Context, key, value string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertMeta(ctx, sqlitesqlc.UpsertMetaParams{Key: key, Value: value})
	case DriverPostgres:
		return s.pgQueries.UpsertMeta(ctx, postgresqlsqlc.UpsertMetaParams{Key: key, Value: value})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateAuthTicket(ctx context.Context, userID, machineID, exposureID string, expiresAtUnix int64) (string, error) {
	ticket, err := randomToken()
	if err != nil {
		return "", err
	}
	ticket = "tk_" + ticket
	ticketHash := hashToken(ticket)
	ticketID, err := randomID()
	if err != nil {
		return "", err
	}
	nowUnix := time.Now().Unix()

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateAuthTicket(ctx, sqlitesqlc.CreateAuthTicketParams{
			ID:         ticketID,
			TicketHash: ticketHash,
			UserID:     userID,
			MachineID:  machineID,
			ExposureID: exposureID,
			ExpiresAt:  expiresAtUnix,
			CreatedAt:  nowUnix,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateAuthTicket(ctx, postgresqlsqlc.CreateAuthTicketParams{
			ID:         ticketID,
			TicketHash: ticketHash,
			UserID:     userID,
			MachineID:  machineID,
			ExposureID: exposureID,
			ExpiresAt:  expiresAtUnix,
			CreatedAt:  nowUnix,
		})
	default:
		return "", unsupportedDriverError(s.driver)
	}
	if err != nil {
		return "", err
	}
	return ticket, nil
}

func (s *Store) VerifyAndConsumeAuthTicket(ctx context.Context, machineToken, ticket string, nowUnix int64) (VerifiedTicket, error) {
	machineID, err := s.GetMachineIDByMachineToken(ctx, machineToken)
	if err != nil {
		return VerifiedTicket{}, err
	}
	return s.verifyAndConsumeAuthTicketByMachineID(ctx, machineID, ticket, nowUnix)
}

func (s *Store) VerifyAndConsumeAuthTicketByMachineID(ctx context.Context, machineID, ticket string, nowUnix int64) (VerifiedTicket, error) {
	return s.verifyAndConsumeAuthTicketByMachineID(ctx, machineID, ticket, nowUnix)
}

func (s *Store) verifyAndConsumeAuthTicketByMachineID(ctx context.Context, machineID, ticket string, nowUnix int64) (VerifiedTicket, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return VerifiedTicket{}, sql.ErrNoRows
	}
	ticketHash := hashToken(ticket)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VerifiedTicket{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var verified VerifiedTicket
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		row, qErr := q.GetValidAuthTicketByHashAndMachine(ctx, sqlitesqlc.GetValidAuthTicketByHashAndMachineParams{
			TicketHash: ticketHash,
			MachineID:  machineID,
			NowUnix:    nowUnix,
		})
		if qErr != nil {
			err = qErr
			return VerifiedTicket{}, err
		}
		updated, qErr := q.MarkAuthTicketUsed(ctx, sqlitesqlc.MarkAuthTicketUsedParams{UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true}, ID: row.ID})
		if qErr != nil {
			err = qErr
			return VerifiedTicket{}, err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return VerifiedTicket{}, err
		}
		verified = VerifiedTicket{UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		row, qErr := q.GetValidAuthTicketByHashAndMachine(ctx, postgresqlsqlc.GetValidAuthTicketByHashAndMachineParams{
			TicketHash: ticketHash,
			MachineID:  machineID,
			NowUnix:    nowUnix,
		})
		if qErr != nil {
			err = qErr
			return VerifiedTicket{}, err
		}
		updated, qErr := q.MarkAuthTicketUsed(ctx, postgresqlsqlc.MarkAuthTicketUsedParams{UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true}, ID: row.ID})
		if qErr != nil {
			err = qErr
			return VerifiedTicket{}, err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return VerifiedTicket{}, err
		}
		verified = VerifiedTicket{UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID}
	default:
		return VerifiedTicket{}, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return VerifiedTicket{}, err
	}
	return verified, nil
}

func (s *Store) GetMachineIDByMachineToken(ctx context.Context, machineToken string) (string, error) {
	tokenHash := hashToken(machineToken)
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetMachineIDByActiveTokenHash(ctx, tokenHash)
	case DriverPostgres:
		return s.pgQueries.GetMachineIDByActiveTokenHash(ctx, tokenHash)
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertMachineTunnel(ctx context.Context, tunnel MachineTunnel) error {
	nowUnix := time.Now().Unix()
	tunnel.CreatedAt = nowUnix
	tunnel.UpdatedAt = nowUnix

	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertMachineTunnel(ctx, sqlitesqlc.UpsertMachineTunnelParams{
			MachineID:   tunnel.MachineID,
			AccountID:   tunnel.AccountID,
			TunnelID:    tunnel.TunnelID,
			TunnelName:  tunnel.TunnelName,
			TunnelToken: tunnel.TunnelToken,
			CreatedAt:   tunnel.CreatedAt,
			UpdatedAt:   tunnel.UpdatedAt,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertMachineTunnel(ctx, postgresqlsqlc.UpsertMachineTunnelParams{
			MachineID:   tunnel.MachineID,
			AccountID:   tunnel.AccountID,
			TunnelID:    tunnel.TunnelID,
			TunnelName:  tunnel.TunnelName,
			TunnelToken: tunnel.TunnelToken,
			CreatedAt:   tunnel.CreatedAt,
			UpdatedAt:   tunnel.UpdatedAt,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineTunnelByMachineID(ctx context.Context, machineID string) (MachineTunnel, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineTunnelByMachineID(ctx, machineID)
		if err != nil {
			return MachineTunnel{}, err
		}
		return MachineTunnel{
			MachineID: row.MachineID, AccountID: row.AccountID, TunnelID: row.TunnelID,
			TunnelName: row.TunnelName, TunnelToken: row.TunnelToken, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineTunnelByMachineID(ctx, machineID)
		if err != nil {
			return MachineTunnel{}, err
		}
		return MachineTunnel{
			MachineID: row.MachineID, AccountID: row.AccountID, TunnelID: row.TunnelID,
			TunnelName: row.TunnelName, TunnelToken: row.TunnelToken, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
		}, nil
	default:
		return MachineTunnel{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertMachineExposure(ctx context.Context, machineID, name, hostname, service string, isPublic bool) (MachineExposure, error) {
	nowUnix := time.Now().Unix()
	exposureID, err := randomID()
	if err != nil {
		return MachineExposure{}, err
	}

	switch s.driver {
	case DriverSQLite:
		if err := s.sqliteQueries.UpsertMachineExposure(ctx, sqlitesqlc.UpsertMachineExposureParams{
			ID:        exposureID,
			MachineID: machineID,
			Name:      name,
			Hostname:  hostname,
			Service:   service,
			IsPublic:  isPublic,
			CreatedAt: nowUnix,
			UpdatedAt: nowUnix,
		}); err != nil {
			return MachineExposure{}, err
		}
		row, err := s.sqliteQueries.GetMachineExposureByMachineIDAndName(ctx, sqlitesqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		return toMachineExposure(row), nil
	case DriverPostgres:
		if err := s.pgQueries.UpsertMachineExposure(ctx, postgresqlsqlc.UpsertMachineExposureParams{
			ID:        exposureID,
			MachineID: machineID,
			Name:      name,
			Hostname:  hostname,
			Service:   service,
			IsPublic:  isPublic,
			CreatedAt: nowUnix,
			UpdatedAt: nowUnix,
		}); err != nil {
			return MachineExposure{}, err
		}
		row, err := s.pgQueries.GetMachineExposureByMachineIDAndName(ctx, postgresqlsqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		return toMachineExposurePG(row), nil
	default:
		return MachineExposure{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListMachineExposuresByMachineID(ctx context.Context, machineID string) ([]MachineExposure, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineExposuresByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		out := make([]MachineExposure, 0, len(rows))
		for _, row := range rows {
			out = append(out, toMachineExposure(row))
		}
		return out, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineExposuresByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		out := make([]MachineExposure, 0, len(rows))
		for _, row := range rows {
			out = append(out, toMachineExposurePG(row))
		}
		return out, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineExposureByHostname(ctx context.Context, hostname string) (MachineExposure, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineExposureByHostname(ctx, hostname)
		if err != nil {
			return MachineExposure{}, err
		}
		return toMachineExposure(row), nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineExposureByHostname(ctx, hostname)
		if err != nil {
			return MachineExposure{}, err
		}
		return toMachineExposurePG(row), nil
	default:
		return MachineExposure{}, unsupportedDriverError(s.driver)
	}
}

func toMachineExposure(row sqlitesqlc.MachineExposure) MachineExposure {
	return MachineExposure{
		ID: row.ID, MachineID: row.MachineID, Name: row.Name, Hostname: row.Hostname,
		Service: row.Service, IsPublic: row.IsPublic, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func toMachineExposurePG(row postgresqlsqlc.MachineExposure) MachineExposure {
	return MachineExposure{
		ID: row.ID, MachineID: row.MachineID, Name: row.Name, Hostname: row.Hostname,
		Service: row.Service, IsPublic: row.IsPublic, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
