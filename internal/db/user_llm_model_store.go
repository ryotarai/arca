package db

import (
	"context"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

// UserLLMModel represents a user's LLM model configuration.
type UserLLMModel struct {
	ID               string
	UserID           string
	ConfigName       string
	EndpointType     string
	CustomEndpoint   string
	ModelName        string
	APIKeyEncrypted  string
	MaxContextTokens int64
	CreatedAt        int64
	UpdatedAt        int64
}

// UserLLMModelSummary is the same as UserLLMModel but without the encrypted API key (for list responses).
type UserLLMModelSummary struct {
	ID               string
	UserID           string
	ConfigName       string
	EndpointType     string
	CustomEndpoint   string
	ModelName        string
	MaxContextTokens int64
	CreatedAt        int64
	UpdatedAt        int64
}

func (s *Store) ListUserLLMModels(ctx context.Context, userID string) ([]UserLLMModelSummary, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListUserLLMModels(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]UserLLMModelSummary, len(rows))
		for i, row := range rows {
			result[i] = UserLLMModelSummary{
				ID:               row.ID,
				UserID:           row.UserID,
				ConfigName:       row.ConfigName,
				EndpointType:     row.EndpointType,
				CustomEndpoint:   row.CustomEndpoint,
				ModelName:        row.ModelName,
				MaxContextTokens: row.MaxContextTokens,
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
			}
		}
		return result, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListUserLLMModels(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]UserLLMModelSummary, len(rows))
		for i, row := range rows {
			result[i] = UserLLMModelSummary{
				ID:               row.ID,
				UserID:           row.UserID,
				ConfigName:       row.ConfigName,
				EndpointType:     row.EndpointType,
				CustomEndpoint:   row.CustomEndpoint,
				ModelName:        row.ModelName,
				MaxContextTokens: int64(row.MaxContextTokens),
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
			}
		}
		return result, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetUserLLMModel(ctx context.Context, id, userID string) (UserLLMModel, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetUserLLMModel(ctx, sqlitesqlc.GetUserLLMModelParams{
			ID:     id,
			UserID: userID,
		})
		if err != nil {
			return UserLLMModel{}, err
		}
		return UserLLMModel{
			ID:               row.ID,
			UserID:           row.UserID,
			ConfigName:       row.ConfigName,
			EndpointType:     row.EndpointType,
			CustomEndpoint:   row.CustomEndpoint,
			ModelName:        row.ModelName,
			APIKeyEncrypted:  row.ApiKeyEncrypted,
			MaxContextTokens: row.MaxContextTokens,
			CreatedAt:        row.CreatedAt,
			UpdatedAt:        row.UpdatedAt,
		}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetUserLLMModel(ctx, postgresqlsqlc.GetUserLLMModelParams{
			ID:     id,
			UserID: userID,
		})
		if err != nil {
			return UserLLMModel{}, err
		}
		return UserLLMModel{
			ID:               row.ID,
			UserID:           row.UserID,
			ConfigName:       row.ConfigName,
			EndpointType:     row.EndpointType,
			CustomEndpoint:   row.CustomEndpoint,
			ModelName:        row.ModelName,
			APIKeyEncrypted:  row.ApiKeyEncrypted,
			MaxContextTokens: int64(row.MaxContextTokens),
			CreatedAt:        row.CreatedAt,
			UpdatedAt:        row.UpdatedAt,
		}, nil
	default:
		return UserLLMModel{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateUserLLMModel(ctx context.Context, model UserLLMModel) error {
	now := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateUserLLMModel(ctx, sqlitesqlc.CreateUserLLMModelParams{
			ID:               model.ID,
			UserID:           model.UserID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			ApiKeyEncrypted:  model.APIKeyEncrypted,
			MaxContextTokens: model.MaxContextTokens,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	case DriverPostgres:
		return s.pgQueries.CreateUserLLMModel(ctx, postgresqlsqlc.CreateUserLLMModelParams{
			ID:               model.ID,
			UserID:           model.UserID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			ApiKeyEncrypted:  model.APIKeyEncrypted,
			MaxContextTokens: int32(model.MaxContextTokens),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateUserLLMModel(ctx context.Context, model UserLLMModel) (bool, error) {
	now := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.UpdateUserLLMModel(ctx, sqlitesqlc.UpdateUserLLMModelParams{
			ID:               model.ID,
			UserID:           model.UserID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			ApiKeyEncrypted:  model.APIKeyEncrypted,
			MaxContextTokens: model.MaxContextTokens,
			UpdatedAt:        now,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.UpdateUserLLMModel(ctx, postgresqlsqlc.UpdateUserLLMModelParams{
			ID:               model.ID,
			UserID:           model.UserID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			ApiKeyEncrypted:  model.APIKeyEncrypted,
			MaxContextTokens: int32(model.MaxContextTokens),
			UpdatedAt:        now,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteUserLLMModel(ctx context.Context, id, userID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DeleteUserLLMModel(ctx, sqlitesqlc.DeleteUserLLMModelParams{
			ID:     id,
			UserID: userID,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DeleteUserLLMModel(ctx, postgresqlsqlc.DeleteUserLLMModelParams{
			ID:     id,
			UserID: userID,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListUserLLMModelsWithAPIKey(ctx context.Context, userID string) ([]UserLLMModel, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListUserLLMModelsWithAPIKey(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]UserLLMModel, len(rows))
		for i, row := range rows {
			result[i] = UserLLMModel{
				ID:               row.ID,
				UserID:           row.UserID,
				ConfigName:       row.ConfigName,
				EndpointType:     row.EndpointType,
				CustomEndpoint:   row.CustomEndpoint,
				ModelName:        row.ModelName,
				APIKeyEncrypted:  row.ApiKeyEncrypted,
				MaxContextTokens: row.MaxContextTokens,
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
			}
		}
		return result, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListUserLLMModelsWithAPIKey(ctx, userID)
		if err != nil {
			return nil, err
		}
		result := make([]UserLLMModel, len(rows))
		for i, row := range rows {
			result[i] = UserLLMModel{
				ID:               row.ID,
				UserID:           row.UserID,
				ConfigName:       row.ConfigName,
				EndpointType:     row.EndpointType,
				CustomEndpoint:   row.CustomEndpoint,
				ModelName:        row.ModelName,
				APIKeyEncrypted:  row.ApiKeyEncrypted,
				MaxContextTokens: int64(row.MaxContextTokens),
				CreatedAt:        row.CreatedAt,
				UpdatedAt:        row.UpdatedAt,
			}
		}
		return result, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}
