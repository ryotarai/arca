package db

import (
	"context"
	"database/sql"
	"strings"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type ArcadSession struct {
	SessionID  string
	UserID     string
	UserEmail  string
	MachineID  string
	ExposureID string
	ExpiresAt  int64
}

func (s *Store) CreateArcadExchangeToken(ctx context.Context, userID, machineID, exposureID string, expiresAtUnix int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	token = "atk_" + token
	tokenHash := hashToken(token)
	tokenID, err := randomID()
	if err != nil {
		return "", err
	}
	nowUnix := time.Now().Unix()

	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.CreateArcadExchangeToken(ctx, sqlitesqlc.CreateArcadExchangeTokenParams{
			ID:         tokenID,
			TokenHash:  tokenHash,
			UserID:     strings.TrimSpace(userID),
			MachineID:  strings.TrimSpace(machineID),
			ExposureID: strings.TrimSpace(exposureID),
			ExpiresAt:  expiresAtUnix,
			CreatedAt:  nowUnix,
		})
	case DriverPostgres:
		err = s.pgQueries.CreateArcadExchangeToken(ctx, postgresqlsqlc.CreateArcadExchangeTokenParams{
			ID:         tokenID,
			TokenHash:  tokenHash,
			UserID:     strings.TrimSpace(userID),
			MachineID:  strings.TrimSpace(machineID),
			ExposureID: strings.TrimSpace(exposureID),
			ExpiresAt:  expiresAtUnix,
			CreatedAt:  nowUnix,
		})
	default:
		return "", unsupportedDriverError(s.driver)
	}
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Store) ExchangeArcadTokenByMachineID(ctx context.Context, machineID, token string, nowUnix, sessionExpiresAtUnix int64) (ArcadSession, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return ArcadSession{}, sql.ErrNoRows
	}
	tokenHash := hashToken(token)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ArcadSession{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	sessionID, err := randomToken()
	if err != nil {
		return ArcadSession{}, err
	}
	sessionID = "as_" + sessionID
	sessionHash := hashToken(sessionID)
	sessionRowID, err := randomID()
	if err != nil {
		return ArcadSession{}, err
	}

	var out ArcadSession
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		row, qErr := q.GetValidArcadExchangeTokenByHashAndMachine(ctx, sqlitesqlc.GetValidArcadExchangeTokenByHashAndMachineParams{
			TokenHash: tokenHash,
			MachineID: machineID,
			NowUnix:   nowUnix,
		})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		updated, qErr := q.MarkArcadExchangeTokenUsed(ctx, sqlitesqlc.MarkArcadExchangeTokenUsedParams{UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true}, ID: row.ID})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return ArcadSession{}, err
		}
		qErr = q.CreateArcadSession(ctx, sqlitesqlc.CreateArcadSessionParams{
			ID:          sessionRowID,
			SessionHash: sessionHash,
			UserID:      row.UserID,
			MachineID:   row.MachineID,
			ExposureID:  row.ExposureID,
			ExpiresAt:   sessionExpiresAtUnix,
			CreatedAt:   nowUnix,
		})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		out = ArcadSession{SessionID: sessionID, UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID, ExpiresAt: sessionExpiresAtUnix}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		row, qErr := q.GetValidArcadExchangeTokenByHashAndMachine(ctx, postgresqlsqlc.GetValidArcadExchangeTokenByHashAndMachineParams{
			TokenHash: tokenHash,
			MachineID: machineID,
			NowUnix:   nowUnix,
		})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		updated, qErr := q.MarkArcadExchangeTokenUsed(ctx, postgresqlsqlc.MarkArcadExchangeTokenUsedParams{UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true}, ID: row.ID})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return ArcadSession{}, err
		}
		qErr = q.CreateArcadSession(ctx, postgresqlsqlc.CreateArcadSessionParams{
			ID:          sessionRowID,
			SessionHash: sessionHash,
			UserID:      row.UserID,
			MachineID:   row.MachineID,
			ExposureID:  row.ExposureID,
			ExpiresAt:   sessionExpiresAtUnix,
			CreatedAt:   nowUnix,
		})
		if qErr != nil {
			err = qErr
			return ArcadSession{}, err
		}
		out = ArcadSession{SessionID: sessionID, UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID, ExpiresAt: sessionExpiresAtUnix}
	default:
		return ArcadSession{}, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return ArcadSession{}, err
	}
	return out, nil
}

func (s *Store) GetActiveArcadSessionByMachineID(ctx context.Context, machineID, sessionID string, nowUnix int64) (ArcadSession, error) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return ArcadSession{}, sql.ErrNoRows
	}
	sessionHash := hashToken(strings.TrimSpace(sessionID))
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetActiveArcadSessionByHashAndMachine(ctx, sqlitesqlc.GetActiveArcadSessionByHashAndMachineParams{
			SessionHash: sessionHash,
			MachineID:   machineID,
			NowUnix:     nowUnix,
		})
		if err != nil {
			return ArcadSession{}, err
		}
		return ArcadSession{UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID, ExpiresAt: row.ExpiresAt}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetActiveArcadSessionByHashAndMachine(ctx, postgresqlsqlc.GetActiveArcadSessionByHashAndMachineParams{
			SessionHash: sessionHash,
			MachineID:   machineID,
			NowUnix:     nowUnix,
		})
		if err != nil {
			return ArcadSession{}, err
		}
		return ArcadSession{UserID: row.UserID, UserEmail: row.Email, MachineID: row.MachineID, ExposureID: row.ExposureID, ExpiresAt: row.ExpiresAt}, nil
	default:
		return ArcadSession{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) RevokeArcadSessionByID(ctx context.Context, sessionID string, revokedAtUnix int64) error {
	sessionHash := hashToken(strings.TrimSpace(sessionID))
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.RevokeArcadSessionByHash(ctx, sqlitesqlc.RevokeArcadSessionByHashParams{RevokedAt: sql.NullInt64{Int64: revokedAtUnix, Valid: true}, SessionHash: sessionHash})
	case DriverPostgres:
		return s.pgQueries.RevokeArcadSessionByHash(ctx, postgresqlsqlc.RevokeArcadSessionByHashParams{RevokedAt: sql.NullInt64{Int64: revokedAtUnix, Valid: true}, SessionHash: sessionHash})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteArcadSessionsByUserID(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.DeleteArcadSessionsByUserID(ctx, userID)
	case DriverPostgres:
		return s.pgQueries.DeleteArcadSessionsByUserID(ctx, userID)
	default:
		return unsupportedDriverError(s.driver)
	}
}
