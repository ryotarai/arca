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
	ServerExposureMethodManual = "manual"
)

func NormalizeServerExposureMethod(method string) string {
	return ServerExposureMethodManual
}

type SetupState struct {
	Completed                      bool
	HasAdmin                       bool
	BaseDomain                     string
	DomainPrefix                   string
	MachineRuntime                 string
	InternetPublicExposureDisabled bool
	OIDCEnabled                    bool
	OIDCIssuerURL                  string
	OIDCClientID                   string
	OIDCClientSecret               string
	OIDCAllowedEmailDomains        []string
	PasswordLoginDisabled          bool
	IAPEnabled                     bool
	IAPAudience                    string
	IAPAutoProvisioning            bool
	OIDCAutoProvisioning           bool
	ServerDomain                   string
	AgentPrompt                    string
	UpdatedAtUnix                  int64
}

type VerifiedTicket struct {
	UserID     string
	UserEmail  string
	MachineID  string
	ExposureID string
}

func (s *Store) GetSetupState(ctx context.Context) (SetupState, error) {
	machineRuntime, err := s.getMetaValue(ctx, setupMetaMachineRuntime)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	machineRuntime = NormalizeMachineTemplate(machineRuntime)
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
	iapEnabledRaw, err := s.getMetaValue(ctx, setupMetaIAPEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	iapEnabled := parseBoolMetaValue(iapEnabledRaw)
	iapAudience, err := s.getMetaValue(ctx, setupMetaIAPAudience)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	iapAutoProvisioningRaw, err := s.getMetaValue(ctx, setupMetaIAPAutoProvisioning)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	iapAutoProvisioning := parseBoolMetaValue(iapAutoProvisioningRaw)
	oidcAutoProvisioningRaw, err := s.getMetaValue(ctx, setupMetaOIDCAutoProvisioning)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	oidcAutoProvisioning := parseBoolMetaValue(oidcAutoProvisioningRaw)
	serverDomain, err := s.getMetaValue(ctx, setupMetaServerDomain)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return SetupState{}, err
	}
	agentPrompt, err := s.getMetaValue(ctx, setupMetaAgentPrompt)
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
			MachineRuntime:                 machineRuntime,
			InternetPublicExposureDisabled: internetPublicExposureDisabled,
			OIDCEnabled:                    oidcEnabled,
			OIDCIssuerURL:                  strings.TrimSpace(oidcIssuerURL),
			OIDCClientID:                   strings.TrimSpace(oidcClientID),
			OIDCClientSecret:               oidcClientSecret,
			OIDCAllowedEmailDomains:        oidcAllowedDomains,
			PasswordLoginDisabled:          passwordLoginDisabled,
			IAPEnabled:                     iapEnabled,
			IAPAudience:                    strings.TrimSpace(iapAudience),
			IAPAutoProvisioning:            iapAutoProvisioning,
			OIDCAutoProvisioning:           oidcAutoProvisioning,
			ServerDomain:                   strings.TrimSpace(serverDomain),
			AgentPrompt:                    agentPrompt,
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
			MachineRuntime:                 machineRuntime,
			InternetPublicExposureDisabled: internetPublicExposureDisabled,
			OIDCEnabled:                    oidcEnabled,
			OIDCIssuerURL:                  strings.TrimSpace(oidcIssuerURL),
			OIDCClientID:                   strings.TrimSpace(oidcClientID),
			OIDCClientSecret:               oidcClientSecret,
			OIDCAllowedEmailDomains:        oidcAllowedDomains,
			PasswordLoginDisabled:          passwordLoginDisabled,
			IAPEnabled:                     iapEnabled,
			IAPAudience:                    strings.TrimSpace(iapAudience),
			IAPAutoProvisioning:            iapAutoProvisioning,
			OIDCAutoProvisioning:           oidcAutoProvisioning,
			ServerDomain:                   strings.TrimSpace(serverDomain),
			AgentPrompt:                    agentPrompt,
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
	state.MachineRuntime = NormalizeMachineTemplate(state.MachineRuntime)

	var err error
	switch s.driver {
	case DriverSQLite:
		err = s.sqliteQueries.UpsertSetupState(ctx, sqlitesqlc.UpsertSetupStateParams{
			Completed:    state.Completed,
			BaseDomain:   strings.TrimSpace(state.BaseDomain),
			DomainPrefix: strings.TrimSpace(state.DomainPrefix),
			UpdatedAt:    nowUnix,
		})
	case DriverPostgres:
		err = s.pgQueries.UpsertSetupState(ctx, postgresqlsqlc.UpsertSetupStateParams{
			Completed:    state.Completed,
			BaseDomain:   strings.TrimSpace(state.BaseDomain),
			DomainPrefix: strings.TrimSpace(state.DomainPrefix),
			UpdatedAt:    nowUnix,
		})
	default:
		return unsupportedDriverError(s.driver)
	}

	if err != nil {
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
	if err := s.upsertMetaValue(ctx, setupMetaIAPEnabled, boolMetaValue(state.IAPEnabled)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaIAPAudience, strings.TrimSpace(state.IAPAudience)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaIAPAutoProvisioning, boolMetaValue(state.IAPAutoProvisioning)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaOIDCAutoProvisioning, boolMetaValue(state.OIDCAutoProvisioning)); err != nil {
		return err
	}
	if err := s.upsertMetaValue(ctx, setupMetaServerDomain, strings.TrimSpace(state.ServerDomain)); err != nil {
		return err
	}
	return s.upsertMetaValue(ctx, setupMetaAgentPrompt, state.AgentPrompt)
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
const setupMetaMachineRuntime = "setup.machine_runtime"
const setupMetaDisableInternetPublicExposure = "setup.disable_internet_public_exposure"
const setupMetaOIDCEnabled = "setup.oidc.enabled"
const setupMetaOIDCIssuerURL = "setup.oidc.issuer_url"
const setupMetaOIDCClientID = "setup.oidc.client_id"
const setupMetaOIDCClientSecret = "setup.oidc.client_secret"
const setupMetaOIDCAllowedEmailDomains = "setup.oidc.allowed_email_domains"
const setupMetaPasswordLoginDisabled = "setup.password_login_disabled"
const setupMetaIAPEnabled = "setup.iap.enabled"
const setupMetaIAPAudience = "setup.iap.audience"
const setupMetaIAPAutoProvisioning = "setup.iap.auto_provisioning"
const setupMetaOIDCAutoProvisioning = "setup.oidc.auto_provisioning"
const setupMetaServerDomain = "setup.server_domain"
const setupMetaAgentPrompt = "agent_prompt"

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

