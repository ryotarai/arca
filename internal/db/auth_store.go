package db

import (
	"context"

	postgresqlsqlc "github.com/ryotarai/hayai/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/hayai/internal/db/sqlc/sqlite"
)

type AuthUser struct {
	ID           string
	Email        string
	PasswordHash string
}

func (s *Store) CreateUser(ctx context.Context, id, email, passwordHash string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateUser(ctx, sqlitesqlc.CreateUserParams{
			ID:           id,
			Email:        email,
			PasswordHash: passwordHash,
		})
	case DriverPostgres:
		return s.pgQueries.CreateUser(ctx, postgresqlsqlc.CreateUserParams{
			ID:           id,
			Email:        email,
			PasswordHash: passwordHash,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (AuthUser, error) {
	switch s.driver {
	case DriverSQLite:
		u, err := s.sqliteQueries.GetUserByEmail(ctx, email)
		if err != nil {
			return AuthUser{}, err
		}
		return AuthUser{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, nil
	case DriverPostgres:
		u, err := s.pgQueries.GetUserByEmail(ctx, email)
		if err != nil {
			return AuthUser{}, err
		}
		return AuthUser{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, nil
	default:
		return AuthUser{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetUserByActiveSessionTokenHash(ctx context.Context, tokenHash string, nowUnix int64) (AuthUser, error) {
	switch s.driver {
	case DriverSQLite:
		u, err := s.sqliteQueries.GetUserByActiveSessionTokenHash(ctx, sqlitesqlc.GetUserByActiveSessionTokenHashParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if err != nil {
			return AuthUser{}, err
		}
		return AuthUser{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, nil
	case DriverPostgres:
		u, err := s.pgQueries.GetUserByActiveSessionTokenHash(ctx, postgresqlsqlc.GetUserByActiveSessionTokenHashParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if err != nil {
			return AuthUser{}, err
		}
		return AuthUser{ID: u.ID, Email: u.Email, PasswordHash: u.PasswordHash}, nil
	default:
		return AuthUser{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateSession(ctx context.Context, id, userID, tokenHash string, expiresAtUnix int64) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateSession(ctx, sqlitesqlc.CreateSessionParams{
			ID:            id,
			UserID:        userID,
			TokenHash:     tokenHash,
			ExpiresAtUnix: expiresAtUnix,
		})
	case DriverPostgres:
		return s.pgQueries.CreateSession(ctx, postgresqlsqlc.CreateSessionParams{
			ID:            id,
			UserID:        userID,
			TokenHash:     tokenHash,
			ExpiresAtUnix: expiresAtUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.RevokeSessionByTokenHash(ctx, tokenHash)
	case DriverPostgres:
		return s.pgQueries.RevokeSessionByTokenHash(ctx, tokenHash)
	default:
		return unsupportedDriverError(s.driver)
	}
}
