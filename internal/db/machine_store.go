package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	postgresqlsqlc "github.com/ryotarai/hayai/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/hayai/internal/db/sqlc/sqlite"
)

type Machine struct {
	ID   string
	Name string
}

func (s *Store) CreateMachineWithOwner(ctx context.Context, userID, name string) (Machine, error) {
	machineID, err := randomID()
	if err != nil {
		return Machine{}, err
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

	switch s.driver {
	case DriverSQLite:
		q := s.sqliteQueries.WithTx(tx)
		if err = q.CreateMachine(ctx, sqlitesqlc.CreateMachineParams{ID: machineID, Name: name}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, sqlitesqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "owner",
		}); err != nil {
			return Machine{}, err
		}
	case DriverPostgres:
		q := s.pgQueries.WithTx(tx)
		if err = q.CreateMachine(ctx, postgresqlsqlc.CreateMachineParams{ID: machineID, Name: name}); err != nil {
			return Machine{}, err
		}
		if err = q.CreateUserMachine(ctx, postgresqlsqlc.CreateUserMachineParams{
			UserID:    userID,
			MachineID: machineID,
			Role:      "owner",
		}); err != nil {
			return Machine{}, err
		}
	default:
		return Machine{}, unsupportedDriverError(s.driver)
	}

	if err = tx.Commit(); err != nil {
		return Machine{}, err
	}

	return Machine{ID: machineID, Name: name}, nil
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
				ID:   row.ID,
				Name: row.Name,
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
				ID:   row.ID,
				Name: row.Name,
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
		return updated > 0, err
	case DriverPostgres:
		updated, err := s.pgQueries.UpdateMachineNameByIDForOwner(ctx, postgresqlsqlc.UpdateMachineNameByIDForOwnerParams{
			Name:      name,
			MachineID: machineID,
			UserID:    userID,
		})
		return updated > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
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

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
