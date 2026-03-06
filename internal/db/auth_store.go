package db

import (
	"context"
	"database/sql"
	"errors"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type AuthUser struct {
	ID                    string
	Email                 string
	PasswordHash          string
	PasswordSetupRequired bool
}

type ManagedUser struct {
	ID                    string
	Email                 string
	PasswordSetupRequired bool
	SetupTokenExpiresAt   int64
	CreatedAt             int64
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
		return toAuthUser(u), nil
	case DriverPostgres:
		u, err := s.pgQueries.GetUserByEmail(ctx, email)
		if err != nil {
			return AuthUser{}, err
		}
		return toAuthUserPG(u), nil
	default:
		return AuthUser{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetUserByID(ctx context.Context, id string) (AuthUser, error) {
	switch s.driver {
	case DriverSQLite:
		u, err := s.sqliteQueries.GetUserByID(ctx, id)
		if err != nil {
			return AuthUser{}, err
		}
		return toAuthUser(u), nil
	case DriverPostgres:
		u, err := s.pgQueries.GetUserByID(ctx, id)
		if err != nil {
			return AuthUser{}, err
		}
		return toAuthUserPG(u), nil
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
		return toAuthUser(u), nil
	case DriverPostgres:
		u, err := s.pgQueries.GetUserByActiveSessionTokenHash(ctx, postgresqlsqlc.GetUserByActiveSessionTokenHashParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if err != nil {
			return AuthUser{}, err
		}
		return toAuthUserPG(u), nil
	default:
		return AuthUser{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListUsers(ctx context.Context, nowUnix int64) ([]ManagedUser, error) {
	switch s.driver {
	case DriverSQLite:
		users, err := s.sqliteQueries.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]ManagedUser, 0, len(users))
		for _, user := range users {
			item := ManagedUser{
				ID:                    user.ID,
				Email:                 user.Email,
				PasswordSetupRequired: user.PasswordSetupRequired,
				CreatedAt:             user.CreatedAt.Unix(),
			}
			if user.PasswordSetupRequired {
				token, tokenErr := s.sqliteQueries.GetActiveUserSetupTokenByUserID(ctx, sqlitesqlc.GetActiveUserSetupTokenByUserIDParams{
					UserID:  user.ID,
					NowUnix: nowUnix,
				})
				if tokenErr != nil && !errors.Is(tokenErr, sql.ErrNoRows) {
					return nil, tokenErr
				}
				if tokenErr == nil {
					item.SetupTokenExpiresAt = token.ExpiresAt
				}
			}
			items = append(items, item)
		}
		return items, nil
	case DriverPostgres:
		users, err := s.pgQueries.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		items := make([]ManagedUser, 0, len(users))
		for _, user := range users {
			item := ManagedUser{
				ID:                    user.ID,
				Email:                 user.Email,
				PasswordSetupRequired: user.PasswordSetupRequired,
				CreatedAt:             user.CreatedAt.Unix(),
			}
			if user.PasswordSetupRequired {
				token, tokenErr := s.pgQueries.GetActiveUserSetupTokenByUserID(ctx, postgresqlsqlc.GetActiveUserSetupTokenByUserIDParams{
					UserID:  user.ID,
					NowUnix: nowUnix,
				})
				if tokenErr != nil && !errors.Is(tokenErr, sql.ErrNoRows) {
					return nil, tokenErr
				}
				if tokenErr == nil {
					item.SetupTokenExpiresAt = token.ExpiresAt
				}
			}
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) IssueUserSetupToken(ctx context.Context, tokenID, tokenHash, userID, createdByUserID string, expiresAtUnix, nowUnix int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	createdBy := sql.NullString{String: createdByUserID, Valid: createdByUserID != ""}
	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		updated, updateErr := q.UpdateUserPasswordSetupRequiredByID(ctx, sqlitesqlc.UpdateUserPasswordSetupRequiredByIDParams{
			PasswordSetupRequired: true,
			ID:                    userID,
		})
		if updateErr != nil {
			err = updateErr
			return err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return err
		}
		if invalidateErr := q.InvalidateUserSetupTokensByUserID(ctx, sqlitesqlc.InvalidateUserSetupTokensByUserIDParams{
			UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true},
			UserID: userID,
		}); invalidateErr != nil {
			err = invalidateErr
			return err
		}
		if createErr := q.CreateUserSetupToken(ctx, sqlitesqlc.CreateUserSetupTokenParams{
			ID:              tokenID,
			TokenHash:       tokenHash,
			UserID:          userID,
			CreatedByUserID: createdBy,
			ExpiresAt:       expiresAtUnix,
			CreatedAt:       nowUnix,
		}); createErr != nil {
			err = createErr
			return err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		updated, updateErr := q.UpdateUserPasswordSetupRequiredByID(ctx, postgresqlsqlc.UpdateUserPasswordSetupRequiredByIDParams{
			PasswordSetupRequired: true,
			ID:                    userID,
		})
		if updateErr != nil {
			err = updateErr
			return err
		}
		if updated == 0 {
			err = sql.ErrNoRows
			return err
		}
		if invalidateErr := q.InvalidateUserSetupTokensByUserID(ctx, postgresqlsqlc.InvalidateUserSetupTokensByUserIDParams{
			UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true},
			UserID: userID,
		}); invalidateErr != nil {
			err = invalidateErr
			return err
		}
		if createErr := q.CreateUserSetupToken(ctx, postgresqlsqlc.CreateUserSetupTokenParams{
			ID:              tokenID,
			TokenHash:       tokenHash,
			UserID:          userID,
			CreatedByUserID: createdBy,
			ExpiresAt:       expiresAtUnix,
			CreatedAt:       nowUnix,
		}); createErr != nil {
			err = createErr
			return err
		}
	default:
		return unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) CompleteUserSetup(ctx context.Context, tokenHash, passwordHash string, nowUnix int64) (AuthUser, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthUser{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		row, getErr := q.GetValidUserSetupTokenByHash(ctx, sqlitesqlc.GetValidUserSetupTokenByHashParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if getErr != nil {
			err = getErr
			return AuthUser{}, err
		}
		updatedToken, markErr := q.MarkUserSetupTokenUsed(ctx, sqlitesqlc.MarkUserSetupTokenUsedParams{
			UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true},
			ID:     row.ID,
		})
		if markErr != nil {
			err = markErr
			return AuthUser{}, err
		}
		if updatedToken == 0 {
			err = sql.ErrNoRows
			return AuthUser{}, err
		}
		updatedPassword, updateErr := q.UpdateUserPasswordHashByID(ctx, sqlitesqlc.UpdateUserPasswordHashByIDParams{
			PasswordHash: passwordHash,
			ID:           row.UserID,
		})
		if updateErr != nil {
			err = updateErr
			return AuthUser{}, err
		}
		if updatedPassword == 0 {
			err = sql.ErrNoRows
			return AuthUser{}, err
		}
		if _, updateErr = q.UpdateUserPasswordSetupRequiredByID(ctx, sqlitesqlc.UpdateUserPasswordSetupRequiredByIDParams{
			PasswordSetupRequired: false,
			ID:                    row.UserID,
		}); updateErr != nil {
			err = updateErr
			return AuthUser{}, err
		}
		if err = tx.Commit(); err != nil {
			return AuthUser{}, err
		}
		return AuthUser{
			ID:                    row.UserID,
			Email:                 row.Email,
			PasswordHash:          passwordHash,
			PasswordSetupRequired: false,
		}, nil
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		row, getErr := q.GetValidUserSetupTokenByHash(ctx, postgresqlsqlc.GetValidUserSetupTokenByHashParams{
			TokenHash: tokenHash,
			NowUnix:   nowUnix,
		})
		if getErr != nil {
			err = getErr
			return AuthUser{}, err
		}
		updatedToken, markErr := q.MarkUserSetupTokenUsed(ctx, postgresqlsqlc.MarkUserSetupTokenUsedParams{
			UsedAt: sql.NullInt64{Int64: nowUnix, Valid: true},
			ID:     row.ID,
		})
		if markErr != nil {
			err = markErr
			return AuthUser{}, err
		}
		if updatedToken == 0 {
			err = sql.ErrNoRows
			return AuthUser{}, err
		}
		updatedPassword, updateErr := q.UpdateUserPasswordHashByID(ctx, postgresqlsqlc.UpdateUserPasswordHashByIDParams{
			PasswordHash: passwordHash,
			ID:           row.UserID,
		})
		if updateErr != nil {
			err = updateErr
			return AuthUser{}, err
		}
		if updatedPassword == 0 {
			err = sql.ErrNoRows
			return AuthUser{}, err
		}
		if _, updateErr = q.UpdateUserPasswordSetupRequiredByID(ctx, postgresqlsqlc.UpdateUserPasswordSetupRequiredByIDParams{
			PasswordSetupRequired: false,
			ID:                    row.UserID,
		}); updateErr != nil {
			err = updateErr
			return AuthUser{}, err
		}
		if err = tx.Commit(); err != nil {
			return AuthUser{}, err
		}
		return AuthUser{
			ID:                    row.UserID,
			Email:                 row.Email,
			PasswordHash:          passwordHash,
			PasswordSetupRequired: false,
		}, nil
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

func toAuthUser(user sqlitesqlc.User) AuthUser {
	return AuthUser{
		ID:                    user.ID,
		Email:                 user.Email,
		PasswordHash:          user.PasswordHash,
		PasswordSetupRequired: user.PasswordSetupRequired,
	}
}

func toAuthUserPG(user postgresqlsqlc.User) AuthUser {
	return AuthUser{
		ID:                    user.ID,
		Email:                 user.Email,
		PasswordHash:          user.PasswordHash,
		PasswordSetupRequired: user.PasswordSetupRequired,
	}
}
