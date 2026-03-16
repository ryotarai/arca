package db

import (
	"context"

	postgresqlsqlc "github.com/ryotarai/arca/internal/db/sqlc/postgresql"
	sqlitesqlc "github.com/ryotarai/arca/internal/db/sqlc/sqlite"
)

type UserGroup struct {
	ID          string
	Name        string
	MemberCount int64
}

type UserGroupMember struct {
	UserID string
	Email  string
}

type MachineGroupAccess struct {
	MachineID string
	GroupID   string
	GroupName string
	Role      string
}

func (s *Store) ListUserGroups(ctx context.Context) ([]UserGroup, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListUserGroups(ctx)
		if err != nil {
			return nil, err
		}
		groups := make([]UserGroup, 0, len(rows))
		for _, row := range rows {
			groups = append(groups, UserGroup{
				ID:          row.ID,
				Name:        row.Name,
				MemberCount: row.MemberCount,
			})
		}
		return groups, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListUserGroups(ctx)
		if err != nil {
			return nil, err
		}
		groups := make([]UserGroup, 0, len(rows))
		for _, row := range rows {
			groups = append(groups, UserGroup{
				ID:          row.ID,
				Name:        row.Name,
				MemberCount: row.MemberCount,
			})
		}
		return groups, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetUserGroup(ctx context.Context, id string) (UserGroup, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetUserGroup(ctx, id)
		if err != nil {
			return UserGroup{}, err
		}
		return UserGroup{ID: row.ID, Name: row.Name}, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetUserGroup(ctx, id)
		if err != nil {
			return UserGroup{}, err
		}
		return UserGroup{ID: row.ID, Name: row.Name}, nil
	default:
		return UserGroup{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) CreateUserGroup(ctx context.Context, id, name string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.CreateUserGroup(ctx, sqlitesqlc.CreateUserGroupParams{
			ID:   id,
			Name: name,
		})
	case DriverPostgres:
		return s.pgQueries.CreateUserGroup(ctx, postgresqlsqlc.CreateUserGroupParams{
			ID:   id,
			Name: name,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteUserGroup(ctx context.Context, id string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.DeleteUserGroup(ctx, id)
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.DeleteUserGroup(ctx, id)
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListUserGroupMembers(ctx context.Context, groupID string) ([]UserGroupMember, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListUserGroupMembers(ctx, groupID)
		if err != nil {
			return nil, err
		}
		members := make([]UserGroupMember, 0, len(rows))
		for _, row := range rows {
			members = append(members, UserGroupMember{
				UserID: row.UserID,
				Email:  row.Email,
			})
		}
		return members, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListUserGroupMembers(ctx, groupID)
		if err != nil {
			return nil, err
		}
		members := make([]UserGroupMember, 0, len(rows))
		for _, row := range rows {
			members = append(members, UserGroupMember{
				UserID: row.UserID,
				Email:  row.Email,
			})
		}
		return members, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) AddUserGroupMember(ctx context.Context, groupID, userID string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.AddUserGroupMember(ctx, sqlitesqlc.AddUserGroupMemberParams{
			GroupID: groupID,
			UserID:  userID,
		})
	case DriverPostgres:
		return s.pgQueries.AddUserGroupMember(ctx, postgresqlsqlc.AddUserGroupMemberParams{
			GroupID: groupID,
			UserID:  userID,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) RemoveUserGroupMember(ctx context.Context, groupID, userID string) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		n, err := s.sqliteQueries.RemoveUserGroupMember(ctx, sqlitesqlc.RemoveUserGroupMemberParams{
			GroupID: groupID,
			UserID:  userID,
		})
		return n > 0, err
	case DriverPostgres:
		n, err := s.pgQueries.RemoveUserGroupMember(ctx, postgresqlsqlc.RemoveUserGroupMemberParams{
			GroupID: groupID,
			UserID:  userID,
		})
		return n > 0, err
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) ListMachineGroupAccess(ctx context.Context, machineID string) ([]MachineGroupAccess, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.ListMachineGroupAccess(ctx, machineID)
		if err != nil {
			return nil, err
		}
		access := make([]MachineGroupAccess, 0, len(rows))
		for _, row := range rows {
			access = append(access, MachineGroupAccess{
				MachineID: row.MachineID,
				GroupID:   row.GroupID,
				GroupName: row.GroupName,
				Role:      row.Role,
			})
		}
		return access, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineGroupAccess(ctx, machineID)
		if err != nil {
			return nil, err
		}
		access := make([]MachineGroupAccess, 0, len(rows))
		for _, row := range rows {
			access = append(access, MachineGroupAccess{
				MachineID: row.MachineID,
				GroupID:   row.GroupID,
				GroupName: row.GroupName,
				Role:      row.Role,
			})
		}
		return access, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertMachineGroupAccess(ctx context.Context, machineID, groupID, role string) error {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.UpsertMachineGroupAccess(ctx, sqlitesqlc.UpsertMachineGroupAccessParams{
			MachineID: machineID,
			GroupID:   groupID,
			Role:      role,
		})
	case DriverPostgres:
		return s.pgQueries.UpsertMachineGroupAccess(ctx, postgresqlsqlc.UpsertMachineGroupAccessParams{
			MachineID: machineID,
			GroupID:   groupID,
			Role:      role,
		})
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) DeleteMachineGroupAccess(ctx context.Context, machineID, groupID string) error {
	switch s.driver {
	case DriverSQLite:
		_, err := s.sqliteQueries.DeleteMachineGroupAccess(ctx, sqlitesqlc.DeleteMachineGroupAccessParams{
			MachineID: machineID,
			GroupID:   groupID,
		})
		return err
	case DriverPostgres:
		_, err := s.pgQueries.DeleteMachineGroupAccess(ctx, postgresqlsqlc.DeleteMachineGroupAccessParams{
			MachineID: machineID,
			GroupID:   groupID,
		})
		return err
	default:
		return unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineGroupRoleByUserID(ctx context.Context, machineID, userID string) (string, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.GetMachineGroupRoleByUserID(ctx, sqlitesqlc.GetMachineGroupRoleByUserIDParams{
			MachineID: machineID,
			UserID:    userID,
		})
	case DriverPostgres:
		return s.pgQueries.GetMachineGroupRoleByUserID(ctx, postgresqlsqlc.GetMachineGroupRoleByUserIDParams{
			MachineID: machineID,
			UserID:    userID,
		})
	default:
		return "", unsupportedDriverError(s.driver)
	}
}

func (s *Store) SearchUserGroups(ctx context.Context, query string, limit int64) ([]UserGroup, error) {
	switch s.driver {
	case DriverSQLite:
		rows, err := s.sqliteQueries.SearchUserGroups(ctx, sqlitesqlc.SearchUserGroupsParams{
			Query:      query,
			LimitCount: limit,
		})
		if err != nil {
			return nil, err
		}
		groups := make([]UserGroup, 0, len(rows))
		for _, row := range rows {
			groups = append(groups, UserGroup{
				ID:   row.ID,
				Name: row.Name,
			})
		}
		return groups, nil
	case DriverPostgres:
		rows, err := s.pgQueries.SearchUserGroups(ctx, postgresqlsqlc.SearchUserGroupsParams{
			Query:      query,
			LimitCount: int32(limit),
		})
		if err != nil {
			return nil, err
		}
		groups := make([]UserGroup, 0, len(rows))
		for _, row := range rows {
			groups = append(groups, UserGroup{
				ID:   row.ID,
				Name: row.Name,
			})
		}
		return groups, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}
