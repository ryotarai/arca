package db

import (
	"context"
	"fmt"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

func (s *Store) GetUserAgentPrompt(ctx context.Context, userID string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetUserAgentPromptByID(ctx, userID)
	case DriverPostgres:
		return s.pgQueries.GetUserAgentPromptByID(ctx, userID)
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) SetUserAgentPrompt(ctx context.Context, userID, prompt string) error {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.UpdateUserAgentPromptByID(ctx, sqlitesqlc.UpdateUserAgentPromptByIDParams{
			AgentPrompt: prompt,
			ID:          userID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("user not found: %s", userID)
		}
		return nil
	case DriverPostgres:
		rows, err := s.pgQueries.UpdateUserAgentPromptByID(ctx, postgresqlsqlc.UpdateUserAgentPromptByIDParams{
			AgentPrompt: prompt,
			ID:          userID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("user not found: %s", userID)
		}
		return nil
	default:
		return unsupportedDriverError(s.driver)
	}
}
