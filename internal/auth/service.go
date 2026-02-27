package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ryotarai/arca/internal/db"
	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailAlreadyUsed   = errors.New("email already used")
	ErrInvalidInput       = errors.New("invalid input")
	ErrUnauthenticated    = errors.New("unauthenticated")
)

type Service struct {
	store      *db.Store
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(store *db.Store) *Service {
	return &Service{
		store:      store,
		sessionTTL: 7 * 24 * time.Hour,
		now:        time.Now,
	}
}

func (s *Service) Register(ctx context.Context, email, password string) (string, string, error) {
	email, password, err := validateAndNormalize(email, password)
	if err != nil {
		return "", "", err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return "", "", err
	}

	userID, err := randomID()
	if err != nil {
		return "", "", err
	}

	if err := s.store.CreateUser(ctx, userID, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return "", "", ErrEmailAlreadyUsed
		}
		return "", "", err
	}

	return userID, email, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (string, string, string, time.Time, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return "", "", "", time.Time{}, ErrInvalidCredentials
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", "", time.Time{}, ErrInvalidCredentials
		}
		return "", "", "", time.Time{}, err
	}

	ok, err := verifyPassword(user.PasswordHash, password)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	if !ok {
		return "", "", "", time.Time{}, ErrInvalidCredentials
	}

	sessionToken, err := randomToken()
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	tokenHash := hashSessionToken(sessionToken)

	sessionID, err := randomID()
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	expiresAt := s.now().Add(s.sessionTTL)
	if err := s.store.CreateSession(ctx, sessionID, user.ID, tokenHash, expiresAt.Unix()); err != nil {
		return "", "", "", time.Time{}, err
	}

	return user.ID, user.Email, sessionToken, expiresAt, nil
}

func (s *Service) Authenticate(ctx context.Context, sessionToken string) (string, string, error) {
	if sessionToken == "" {
		return "", "", ErrUnauthenticated
	}
	tokenHash := hashSessionToken(sessionToken)
	user, err := s.store.GetUserByActiveSessionTokenHash(ctx, tokenHash, s.now().Unix())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrUnauthenticated
		}
		return "", "", err
	}
	return user.ID, user.Email, nil
}

func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	if sessionToken == "" {
		return nil
	}
	tokenHash := hashSessionToken(sessionToken)
	return s.store.RevokeSessionByTokenHash(ctx, tokenHash)
}

func validateAndNormalize(email, password string) (string, string, error) {
	email = normalizeEmail(email)
	password = strings.TrimSpace(password)

	if email == "" || !strings.Contains(email, "@") {
		return "", "", ErrInvalidInput
	}
	if len(password) < 8 {
		return "", "", ErrInvalidInput
	}

	return email, password, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	const (
		memory      = 64 * 1024
		iterations  = 3
		parallelism = 1
		keyLength   = 32
	)

	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	return fmt.Sprintf(
		"argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(encodedHash, password string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 || parts[0] != "argon2id" {
		return false, errors.New("invalid password hash format")
	}

	if parts[1] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errors.New("unsupported argon2 version")
	}

	var memory uint32
	var iterations uint32
	var parallelism uint8

	for _, param := range strings.Split(parts[2], ",") {
		keyValue := strings.SplitN(param, "=", 2)
		if len(keyValue) != 2 {
			return false, errors.New("invalid argon2 params")
		}

		switch keyValue[0] {
		case "m":
			value, err := strconv.ParseUint(keyValue[1], 10, 32)
			if err != nil {
				return false, err
			}
			memory = uint32(value)
		case "t":
			value, err := strconv.ParseUint(keyValue[1], 10, 32)
			if err != nil {
				return false, err
			}
			iterations = uint32(value)
		case "p":
			value, err := strconv.ParseUint(keyValue[1], 10, 8)
			if err != nil {
				return false, err
			}
			parallelism = uint8(value)
		default:
			return false, errors.New("unknown argon2 parameter")
		}
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil {
		return false, err
	}
	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}

	computed := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(expectedHash, computed) == 1, nil
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}
