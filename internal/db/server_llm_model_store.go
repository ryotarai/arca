package db

import (
	"context"
	"time"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

// ServerLLMModel represents a server-wide LLM model configuration.
type ServerLLMModel struct {
	ID               string
	ConfigName       string
	EndpointType     string
	CustomEndpoint   string
	ModelName        string
	TokenCommand     string
	MaxContextTokens int64
	CreatedAt        int64
	UpdatedAt        int64
}

func (s *Store) ListServerLLMModels(ctx context.Context) ([]ServerLLMModel, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListServerLLMModels(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]ServerLLMModel, len(rows))
		for i, row := range rows {
			result[i] = serverLLMModelFromSQLite(row)
		}
		return result, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListServerLLMModels(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]ServerLLMModel, len(rows))
		for i, row := range rows {
			result[i] = serverLLMModelFromPG(row)
		}
		return result, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetServerLLMModel(ctx context.Context, id string) (ServerLLMModel, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetServerLLMModel(ctx, id)
		if err != nil {
			return ServerLLMModel{}, err
		}
		return serverLLMModelFromSQLite(row), nil
	case DriverPostgres:
		row, err := s.pgQueries.GetServerLLMModel(ctx, id)
		if err != nil {
			return ServerLLMModel{}, err
		}
		return serverLLMModelFromPG(row), nil
	default:
		return ServerLLMModel{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateServerLLMModel(ctx context.Context, model ServerLLMModel) error {
	now := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateServerLLMModel(ctx, sqlitesqlc.CreateServerLLMModelParams{
			ID:               model.ID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			TokenCommand:     model.TokenCommand,
			MaxContextTokens: model.MaxContextTokens,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	case DriverPostgres:
		return s.pgQueries.CreateServerLLMModel(ctx, postgresqlsqlc.CreateServerLLMModelParams{
			ID:               model.ID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			TokenCommand:     model.TokenCommand,
			MaxContextTokens: int32(model.MaxContextTokens),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpdateServerLLMModel(ctx context.Context, model ServerLLMModel) (bool, error) {
	now := time.Now().Unix()
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.UpdateServerLLMModel(ctx, sqlitesqlc.UpdateServerLLMModelParams{
			ID:               model.ID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			TokenCommand:     model.TokenCommand,
			MaxContextTokens: model.MaxContextTokens,
			UpdatedAt:        now,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.UpdateServerLLMModel(ctx, postgresqlsqlc.UpdateServerLLMModelParams{
			ID:               model.ID,
			ConfigName:       model.ConfigName,
			EndpointType:     model.EndpointType,
			CustomEndpoint:   model.CustomEndpoint,
			ModelName:        model.ModelName,
			TokenCommand:     model.TokenCommand,
			MaxContextTokens: int32(model.MaxContextTokens),
			UpdatedAt:        now,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteServerLLMModel(ctx context.Context, id string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DeleteServerLLMModel(ctx, id)
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DeleteServerLLMModel(ctx, id)
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func serverLLMModelFromSQLite(row sqlitesqlc.ServerLlmModel) ServerLLMModel {
	return ServerLLMModel{
		ID:               row.ID,
		ConfigName:       row.ConfigName,
		EndpointType:     row.EndpointType,
		CustomEndpoint:   row.CustomEndpoint,
		ModelName:        row.ModelName,
		TokenCommand:     row.TokenCommand,
		MaxContextTokens: row.MaxContextTokens,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}

func serverLLMModelFromPG(row postgresqlsqlc.ServerLlmModel) ServerLLMModel {
	return ServerLLMModel{
		ID:               row.ID,
		ConfigName:       row.ConfigName,
		EndpointType:     row.EndpointType,
		CustomEndpoint:   row.CustomEndpoint,
		ModelName:        row.ModelName,
		TokenCommand:     row.TokenCommand,
		MaxContextTokens: int64(row.MaxContextTokens),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
