package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	issuer            = "qazna"
	secretEnvVariable = "QAZNA_AUTH_SECRET"
)

var (
	errMissingSecret = errors.New("auth secret is not configured")

	secretMu sync.Mutex
	secret   cachedSecret
)

type cachedSecret struct {
	value []byte
	err   error
	ready bool
}

// ErrInvalidToken indicates the token failed validation.
var ErrInvalidToken = errors.New("invalid token")

// Claims represents JWT claims used across the service.
type Claims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

// GenerateToken signs a JWT for the given user and roles using HS256.
func GenerateToken(userID string, roles []string, ttl time.Duration) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", errors.New("userID is required")
	}
	if ttl <= 0 {
		return "", errors.New("ttl must be greater than zero")
	}
	secretBytes, err := loadSecret()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	claims := Claims{
		Roles: dedupeRoles(roles),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secretBytes)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ParseAndValidate verifies the token signature and required claims.
func ParseAndValidate(token string) (*Claims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrInvalidToken
	}
	secretBytes, err := loadSecret()
	if err != nil {
		return nil, err
	}

	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return secretBytes, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	if err := validateClaims(claims); err != nil {
		return nil, ErrInvalidToken
	}
	claims.Roles = dedupeRoles(claims.Roles)
	return claims, nil
}

func validateClaims(claims *Claims) error {
	if claims.Issuer != issuer {
		return fmt.Errorf("unexpected issuer: %s", claims.Issuer)
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return errors.New("subject missing")
	}
	if claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return errors.New("timestamps missing")
	}
	now := time.Now().UTC()
	if now.After(claims.ExpiresAt.Time) {
		return errors.New("token expired")
	}
	if claims.NotBefore != nil && now.Before(claims.NotBefore.Time) {
		return errors.New("token not yet valid")
	}
	// Allow a small clock skew of 5 seconds when validating issued-at.
	if claims.IssuedAt.Time.After(now.Add(5 * time.Second)) {
		return errors.New("token issued in the future")
	}
	if claims.ExpiresAt.Time.Before(claims.IssuedAt.Time) {
		return errors.New("token expiry precedes issued-at")
	}
	return nil
}

func dedupeRoles(roles []string) []string {
	if len(roles) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(roles))
	var normalized []string
	for _, role := range roles {
		role = strings.TrimSpace(strings.ToLower(role))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	return normalized
}

func loadSecret() ([]byte, error) {
	secretMu.Lock()
	defer secretMu.Unlock()
	if secret.ready {
		return secret.value, secret.err
	}
	raw := strings.TrimSpace(os.Getenv(secretEnvVariable))
	if raw == "" {
		secret.err = errMissingSecret
		secret.ready = true
		return nil, secret.err
	}
	secret.value = []byte(raw)
	secret.err = nil
	secret.ready = true
	return secret.value, nil
}

// ResetSecretForTests clears the cached secret value. Only intended for test use.
func ResetSecretForTests() {
	secretMu.Lock()
	defer secretMu.Unlock()
	secret = cachedSecret{}
}

type ctxKey string

const (
	userIDKey ctxKey = "auth_user_id"
	rolesKey  ctxKey = "auth_roles"
)

// ContextWithUser stores user identity in the context.
func ContextWithUser(ctx context.Context, userID string, roles []string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, strings.TrimSpace(userID))
	if len(roles) > 0 {
		ctx = context.WithValue(ctx, rolesKey, dedupeRoles(roles))
	}
	return ctx
}

// UserIDFromContext extracts the authenticated user ID from context.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

// RolesFromContext returns the roles stored in context (deduplicated and lower-cased).
func RolesFromContext(ctx context.Context) []string {
	v, ok := ctx.Value(rolesKey).([]string)
	if !ok {
		return nil
	}
	if len(v) == 0 {
		return nil
	}
	out := make([]string, len(v))
	copy(out, v)
	return out
}

// HasRole checks whether the context contains the specified role.
func HasRole(ctx context.Context, role string) bool {
	role = strings.TrimSpace(strings.ToLower(role))
	if role == "" {
		return false
	}
	for _, r := range RolesFromContext(ctx) {
		if r == role {
			return true
		}
	}
	return false
}
