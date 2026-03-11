package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"google.golang.org/api/idtoken"
)

var (
	ErrIAPNotConfigured = errors.New("iap not configured")
	ErrIAPRejected      = errors.New("iap login rejected")
)

// AuthenticateIAPJWT validates a Google Cloud IAP JWT assertion and returns the
// matching user's ID, email, and role. The user must already exist (pre-provisioned).
func (s *Service) AuthenticateIAPJWT(ctx context.Context, jwtToken string) (string, string, string, error) {
	jwtToken = strings.TrimSpace(jwtToken)
	if jwtToken == "" {
		return "", "", "", ErrIAPNotConfigured
	}

	setup, err := s.store.GetSetupState(ctx)
	if err != nil {
		return "", "", "", err
	}
	if !setup.IAPEnabled {
		return "", "", "", ErrIAPNotConfigured
	}
	audience := strings.TrimSpace(setup.IAPAudience)
	if audience == "" {
		return "", "", "", ErrIAPNotConfigured
	}

	payload, err := idtoken.Validate(ctx, jwtToken, audience)
	if err != nil {
		return "", "", "", ErrIAPRejected
	}

	emailRaw, _ := payload.Claims["email"].(string)
	email := normalizeEmail(emailRaw)
	if email == "" {
		return "", "", "", ErrIAPRejected
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if !setup.IAPAutoProvisioning {
				return "", "", "", ErrIAPRejected
			}
			userID, createErr := s.autoProvisionUser(ctx, email)
			if createErr != nil {
				return "", "", "", createErr
			}
			return userID, email, "user", nil
		}
		return "", "", "", err
	}

	return user.ID, user.Email, user.Role, nil
}
