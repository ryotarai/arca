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

const (
	ServerExposureMethodCloudflareTunnel = "cloudflare_tunnel"
	ServerExposureMethodManual           = "manual"
)

func NormalizeServerExposureMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case ServerExposureMethodManual:
		return ServerExposureMethodManual
	default:
		return ServerExposureMethodCloudflareTunnel
	}
}

type SetupState struct {
	Completed                      bool
	HasAdmin                       bool
	BaseDomain                     string
	DomainPrefix                   string
	CloudflareAPIToken             string
	CloudflareZoneID               string
	MachineRuntime                 string
	InternetPublicExposureDisabled bool
	OIDCEnabled                    bool
	OIDCIssuerURL                  string
	OIDCClientID                   string
	OIDCClientSecret               string
	OIDCAllowedEmailDomains        []string
	PasswordLoginDisabled          bool
	ServerExposureMethod           string
	ServerDomain                   string
	UpdatedAtUnix                  int64
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
	ID              string
	MachineID       string
	Name            string
	Hostname        string
	Service         string
	IsPublic        bool
	Visibility      string
	SelectedUserIDs []string
	CreatedAt       int64
	UpdatedAt       int64
}

func (s *Store) GetSetupState(ctx context.Context) (SetupState, error) {
	zoneID, err := s.getMetaValue(ctx, setupMetaCloudflareZoneID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	machineRuntime, err := s.getMetaValue(ctx, setupMetaMachineRuntime)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	machineRuntime = NormalizeMachineRuntime(machineRuntime)
	internetPublicExposureDisabledRaw, err := s.getMetaValue(ctx, setupMetaDisableInternetPublicExposure)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	internetPublicExposureDisabled := parseBoolMetaValue(internetPublicExposureDisabledRaw)
	oidcEnabledRaw, err := s.getMetaValue(ctx, setupMetaOIDCEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcEnabled := parseBoolMetaValue(oidcEnabledRaw)
	oidcIssuerURL, err := s.getMetaValue(ctx, setupMetaOIDCIssuerURL)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcClientID, err := s.getMetaValue(ctx, setupMetaOIDCClientID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcClientSecret, err := s.getMetaValue(ctx, setupMetaOIDCClientSecret)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcAllowedDomainsRaw, err := s.getMetaValue(ctx, setupMetaOIDCAllowedEmailDomains)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcAllowedDomains := parseCSVMetaValue(oidcAllowedDomainsRaw)
	passwordLoginDisabledRaw, err := s.getMetaValue(ctx, setupMetaPasswordLoginDisabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	passwordLoginDisabled := parseBoolMetaValue(passwordLoginDisabledRaw)
	serverExposureMethod, err := s.getMetaValue(ctx, setupMetaServerExposureMethod)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	serverExposureMethod = NormalizeServerExposureMethod(serverExposureMethod)
	serverDomain, err := s.getMetaValue(ctx, setupMetaServerDomain)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}

	hasAdmin, err := s.HasAdminUser(ctx)
	if err != nil {
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
			Completed:                      state.Completed,
			HasAdmin:                       hasAdmin,
			BaseDomain:                     state.BaseDomain,
			DomainPrefix:                   state.DomainPrefix,
			CloudflareAPIToken:             state.CloudflareApiToken,
			CloudflareZoneID:               zoneID,
			MachineRuntime:                 machineRuntime,
			InternetPublicExposureDisabled: internetPublicExposureDisabled,
			OIDCEnabled:                    oidcEnabled,
			OIDCIssuerURL:                  strings.TrimSpace(oidcIssuerURL),
			OIDCClientID:                   strings.TrimSpace(oidcClientID),
			OIDCClientSecret:               oidcClientSecret,
			OIDCAllowedEmailDomains:        oidcAllowedDomains,
			PasswordLoginDisabled:          passwordLoginDisabled,
			ServerExposureMethod:           serverExposureMethod,
			ServerDomain:                   strings.TrimSpace(serverDomain),
			UpdatedAtUnix:                  state.UpdatedAt,
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
			Completed:                      state.Completed,
			HasAdmin:                       hasAdmin,
			BaseDomain:                     state.BaseDomain,
			DomainPrefix:                   state.DomainPrefix,
			CloudflareAPIToken:             state.CloudflareApiToken,
			CloudflareZoneID:               zoneID,
			MachineRuntime:                 machineRuntime,
			InternetPublicExposureDisabled: internetPublicExposureDisabled,
			OIDCEnabled:                    oidcEnabled,
			OIDCIssuerURL:                  strings.TrimSpace(oidcIssuerURL),
			OIDCClientID:                   strings.TrimSpace(oidcClientID),
			OIDCClientSecret:               oidcClientSecret,
			OIDCAllowedEmailDomains:        oidcAllowedDomains,
			PasswordLoginDisabled:          passwordLoginDisabled,
			ServerExposureMethod:           serverExposureMethod,
			ServerDomain:                   strings.TrimSpace(serverDomain),
			UpdatedAtUnix:                  state.UpdatedAt,
		}, nil
	default:
		return SetupState{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) HasAdminUser(ctx context.Context) (bool, error) {
	switch s.driver {
	case DriverSQLite:
		return s.sqliteQueries.HasAdminUser(ctx)
	case DriverPostgres:
		return s.pgQueries.HasAdminUser(ctx)
	default:
		return false, unsupportedDriverError(s.driver)
	}
}

func (s *Store) UpsertSetupState(ctx context.Context, state SetupState) error {
	nowUnix := time.Now().Unix()
	state.MachineRuntime = NormalizeMachineRuntime(state.MachineRuntime)

	var err error
	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.UpsertSetupState(ctx, sqlitesqlc.UpsertSetupStateParams{
			Completed:          state.Completed,
			BaseDomain:         strings.TrimSpace(state.BaseDomain),
			DomainPrefix:       strings.TrimSpace(state.DomainPrefix),
			CloudflareApiToken: state.CloudflareAPIToken,
			UpdatedAt:          nowUnix,
		})
	case DriverPostgres:
		err = s.pgQueries.UpsertSetupState(ctx, postgresqlsqlc.UpsertSetupStateParams{
			Completed:          state.Completed,
			BaseDomain:         strings.TrimSpace(state.BaseDomain),
			DomainPrefix:       strings.TrimSpace(state.DomainPrefix),
			CloudflareApiToken: state.CloudflareAPIToken,
			UpdatedAt:          nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}

	if err != nil {
		return err
	}

	if err := s.upsertMetaValue(ctx, setupMetaCloudflareZoneID, strings.TrimSpace(state.CloudflareZoneID)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaMachineRuntime, state.MachineRuntime); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaDisableInternetPublicExposure, boolMetaValue(state.InternetPublicExposureDisabled)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCEnabled, boolMetaValue(state.OIDCEnabled)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCIssuerURL, strings.TrimSpace(state.OIDCIssuerURL)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCClientID, strings.TrimSpace(state.OIDCClientID)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCClientSecret, state.OIDCClientSecret); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCAllowedEmailDomains, csvMetaValue(state.OIDCAllowedEmailDomains)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaPasswordLoginDisabled, boolMetaValue(state.PasswordLoginDisabled)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaServerExposureMethod, NormalizeServerExposureMethod(state.ServerExposureMethod)); err != nil {
		return err
	}
	return s.upsertMetaValue(ctx, setupMetaServerDomain, strings.TrimSpace(state.ServerDomain))
}

func (s *Store) GetSetupPassword(ctx context.Context) (string, error) {
	v, err := s.getMetaValue(ctx, setupMetaSetupPassword)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

func (s *Store) SetSetupPassword(ctx context.Context, password string) error {
	return s.upsertMetaValue(ctx, setupMetaSetupPassword, password)
}

const setupMetaSetupPassword = "setup.password"
const setupMetaCloudflareZoneID = "setup.cloudflare_zone_id"
const setupMetaMachineRuntime = "setup.machine_runtime"
const setupMetaDisableInternetPublicExposure = "setup.disable_internet_public_exposure"
const setupMetaOIDCEnabled = "setup.oidc.enabled"
const setupMetaOIDCIssuerURL = "setup.oidc.issuer_url"
const setupMetaOIDCClientID = "setup.oidc.client_id"
const setupMetaOIDCClientSecret = "setup.oidc.client_secret"
const setupMetaOIDCAllowedEmailDomains = "setup.oidc.allowed_email_domains"
const setupMetaPasswordLoginDisabled = "setup.password_login_disabled"
const setupMetaServerExposureMethod = "setup.server_exposure_method"
const setupMetaServerDomain = "setup.server_domain"

func parseBoolMetaValue(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "1" || value == "true" || value == "yes"
}

func boolMetaValue(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func parseCSVMetaValue(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func csvMetaValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return strings.Join(normalized, ",")
}

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

func (s *Store) UpsertMachineExposure(ctx context.Context, machineID, name, hostname, service, visibility string, selectedUserIDs []string) (MachineExposure, error) {
	nowUnix := time.Now().Unix()
	exposureID, err := randomID()
	if err != nil {
		return MachineExposure{}, err
	}
	visibility = NormalizeEndpointVisibility(visibility)
	isPublic := IsInternetPublicVisibility(visibility)

	switch s.driver {
	case DriverSQLite:
		if err := s.sqliteQueries.UpsertMachineExposure(ctx, sqlitesqlc.UpsertMachineExposureParams{
			ID:         exposureID,
			MachineID:  machineID,
			Name:       name,
			Hostname:   hostname,
			Service:    service,
			IsPublic:   isPublic,
			Visibility: visibility,
			CreatedAt:  nowUnix,
			UpdatedAt:  nowUnix,
		}); err != nil {
			return MachineExposure{}, err
		}
		row, err := s.sqliteQueries.GetMachineExposureByMachineIDAndName(ctx, sqlitesqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		if err := s.syncMachineExposureACLUsersSQLite(ctx, row.ID, selectedUserIDs, nowUnix); err != nil {
			return MachineExposure{}, err
		}
		selected, err := s.listMachineExposureACLUsersSQLite(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposure(row)
		exposure.SelectedUserIDs = selected
		return exposure, nil
	case DriverPostgres:
		if err := s.pgQueries.UpsertMachineExposure(ctx, postgresqlsqlc.UpsertMachineExposureParams{
			ID:         exposureID,
			MachineID:  machineID,
			Name:       name,
			Hostname:   hostname,
			Service:    service,
			IsPublic:   isPublic,
			Visibility: visibility,
			CreatedAt:  nowUnix,
			UpdatedAt:  nowUnix,
		}); err != nil {
			return MachineExposure{}, err
		}
		row, err := s.pgQueries.GetMachineExposureByMachineIDAndName(ctx, postgresqlsqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		if err := s.syncMachineExposureACLUsersPostgres(ctx, row.ID, selectedUserIDs, nowUnix); err != nil {
			return MachineExposure{}, err
		}
		selected, err := s.listMachineExposureACLUsersPostgres(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposurePG(row)
		exposure.SelectedUserIDs = selected
		return exposure, nil
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
			item := toMachineExposure(row)
			selected, aclErr := s.listMachineExposureACLUsersSQLite(ctx, row.ID)
			if aclErr != nil {
				return nil, aclErr
			}
			item.SelectedUserIDs = selected
			out = append(out, item)
		}
		return out, nil
	case DriverPostgres:
		rows, err := s.pgQueries.ListMachineExposuresByMachineID(ctx, machineID)
		if err != nil {
			return nil, err
		}
		out := make([]MachineExposure, 0, len(rows))
		for _, row := range rows {
			item := toMachineExposurePG(row)
			selected, aclErr := s.listMachineExposureACLUsersPostgres(ctx, row.ID)
			if aclErr != nil {
				return nil, aclErr
			}
			item.SelectedUserIDs = selected
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineExposureByMachineIDAndName(ctx context.Context, machineID, name string) (MachineExposure, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineExposureByMachineIDAndName(ctx, sqlitesqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposure(row)
		selected, err := s.listMachineExposureACLUsersSQLite(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure.SelectedUserIDs = selected
		return exposure, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineExposureByMachineIDAndName(ctx, postgresqlsqlc.GetMachineExposureByMachineIDAndNameParams{MachineID: machineID, Name: name})
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposurePG(row)
		selected, err := s.listMachineExposureACLUsersPostgres(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure.SelectedUserIDs = selected
		return exposure, nil
	default:
		return MachineExposure{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) GetMachineExposureByHostname(ctx context.Context, hostname string) (MachineExposure, error) {
	switch s.driver {
	case DriverSQLite:
		row, err := s.sqliteQueries.GetMachineExposureByHostname(ctx, hostname)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposure(row)
		selected, err := s.listMachineExposureACLUsersSQLite(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure.SelectedUserIDs = selected
		return exposure, nil
	case DriverPostgres:
		row, err := s.pgQueries.GetMachineExposureByHostname(ctx, hostname)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure := toMachineExposurePG(row)
		selected, err := s.listMachineExposureACLUsersPostgres(ctx, row.ID)
		if err != nil {
			return MachineExposure{}, err
		}
		exposure.SelectedUserIDs = selected
		return exposure, nil
	default:
		return MachineExposure{}, unsupportedDriverError(s.driver)
	}
}

func (s *Store) syncMachineExposureACLUsersSQLite(ctx context.Context, exposureID string, userIDs []string, nowUnix int64) error {
	if err := s.sqliteQueries.DeleteMachineExposureACLUsersByExposureID(ctx, exposureID); err != nil {
		return err
	}
	for _, userID := range userIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		if _, err := s.sqliteQueries.GetUserByID(ctx, trimmed); err != nil {
			return err
		}
		if err := s.sqliteQueries.InsertMachineExposureACLUser(ctx, sqlitesqlc.InsertMachineExposureACLUserParams{
			ExposureID: exposureID,
			UserID:     trimmed,
			CreatedAt:  nowUnix,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) syncMachineExposureACLUsersPostgres(ctx context.Context, exposureID string, userIDs []string, nowUnix int64) error {
	if err := s.pgQueries.DeleteMachineExposureACLUsersByExposureID(ctx, exposureID); err != nil {
		return err
	}
	for _, userID := range userIDs {
		trimmed := strings.TrimSpace(userID)
		if trimmed == "" {
			continue
		}
		if _, err := s.pgQueries.GetUserByID(ctx, trimmed); err != nil {
			return err
		}
		if err := s.pgQueries.InsertMachineExposureACLUser(ctx, postgresqlsqlc.InsertMachineExposureACLUserParams{
			ExposureID: exposureID,
			UserID:     trimmed,
			CreatedAt:  nowUnix,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) listMachineExposureACLUsersSQLite(ctx context.Context, exposureID string) ([]string, error) {
	rows, err := s.sqliteQueries.ListMachineExposureACLUsersByExposureID(ctx, exposureID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Store) listMachineExposureACLUsersPostgres(ctx context.Context, exposureID string) ([]string, error) {
	rows, err := s.pgQueries.ListMachineExposureACLUsersByExposureID(ctx, exposureID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func toMachineExposure(row sqlitesqlc.MachineExposure) MachineExposure {
	return MachineExposure{
		ID: row.ID, MachineID: row.MachineID, Name: row.Name, Hostname: row.Hostname,
		Service: row.Service, IsPublic: row.IsPublic, Visibility: NormalizeEndpointVisibility(row.Visibility), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func toMachineExposurePG(row postgresqlsqlc.MachineExposure) MachineExposure {
	return MachineExposure{
		ID: row.ID, MachineID: row.MachineID, Name: row.Name, Hostname: row.Hostname,
		Service: row.Service, IsPublic: row.IsPublic, Visibility: NormalizeEndpointVisibility(row.Visibility), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
