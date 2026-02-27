package arcad

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Session struct {
	UserID    string
	ExpiresAt time.Time
}

type SessionManager struct {
	secret []byte
}

func NewSessionManager(machineToken string) *SessionManager {
	return &SessionManager{secret: []byte(machineToken)}
}

func (m *SessionManager) Encode(s Session) (string, error) {
	if s.UserID == "" {
		return "", fmt.Errorf("empty user id")
	}
	if s.ExpiresAt.IsZero() {
		return "", fmt.Errorf("empty expiry")
	}
	payload := s.UserID + "|" + strconv.FormatInt(s.ExpiresAt.Unix(), 10)
	sig := m.sign(payload)
	return base64.RawURLEncoding.EncodeToString([]byte(payload + "|" + sig)), nil
}

func (m *SessionManager) Decode(token string) (Session, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Session{}, fmt.Errorf("decode session token: %w", err)
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return Session{}, fmt.Errorf("invalid session format")
	}
	payload := parts[0] + "|" + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(m.sign(payload))) {
		return Session{}, fmt.Errorf("invalid session signature")
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return Session{}, fmt.Errorf("invalid session expiry: %w", err)
	}
	s := Session{UserID: parts[0], ExpiresAt: time.Unix(expiresUnix, 0).UTC()}
	if time.Now().After(s.ExpiresAt) {
		return Session{}, fmt.Errorf("session expired")
	}
	return s, nil
}

func (m *SessionManager) sign(payload string) string {
	h := hmac.New(sha256.New, m.secret)
	_, _ = h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
