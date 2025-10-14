package auth

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"qazna.org/internal/ids"
)

const (
	defaultAccessTTL  = 15 * time.Minute
	defaultRefreshTTL = 24 * time.Hour * 14
)

// Service provides high level RBAC operations and token issuance.
type Service struct {
	store Store
	now   func() time.Time

	// HMAC fallback (legacy)
	tokenSecret []byte

	// RS256 configuration
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// TokenClaims represents minimal verified JWT claims.
type TokenClaims struct {
	Subject   string
	Issuer    string
	TokenType string
	IssuedAt  int64
	ExpiresAt int64
}

// ServiceOption configures Service behavior.
type ServiceOption func(*Service) error

// WithTokenSecret enables bearer token verification using the provided secret (legacy HS256).
func WithTokenSecret(secret string) ServiceOption {
	return func(s *Service) error {
		if strings.TrimSpace(secret) == "" {
			return nil
		}
		s.tokenSecret = []byte(secret)
		return nil
	}
}

// WithRS256Keys configures RSA keys used for signing and verifying JWTs.
func WithRS256Keys(privatePEM, publicPEM string) ServiceOption {
	return func(s *Service) error {
		privatePEM = strings.TrimSpace(privatePEM)
		publicPEM = strings.TrimSpace(publicPEM)
		if privatePEM == "" || publicPEM == "" {
			return errors.New("auth: both private and public keys are required")
		}
		priv, err := parseRSAPrivateKey(privatePEM)
		if err != nil {
			return fmt.Errorf("auth: parse private key: %w", err)
		}
		pub, err := parseRSAPublicKey(publicPEM)
		if err != nil {
			return fmt.Errorf("auth: parse public key: %w", err)
		}
		s.privateKey = priv
		s.publicKey = pub
		return nil
	}
}

// WithKeyID sets the key identifier embedded into JWT headers.
func WithKeyID(kid string) ServiceOption {
	return func(s *Service) error {
		s.keyID = strings.TrimSpace(kid)
		return nil
	}
}

// WithIssuer overrides the token issuer claim.
func WithIssuer(issuer string) ServiceOption {
	return func(s *Service) error {
		s.issuer = strings.TrimSpace(issuer)
		return nil
	}
}

// WithAccessTTL configures access token lifetime.
func WithAccessTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) error {
		if ttl > 0 {
			s.accessTTL = ttl
		}
		return nil
	}
}

// WithRefreshTTL configures refresh token lifetime.
func WithRefreshTTL(ttl time.Duration) ServiceOption {
	return func(s *Service) error {
		if ttl > 0 {
			s.refreshTTL = ttl
		}
		return nil
	}
}

// WithClock overrides time source (useful for tests).
func WithClock(fn func() time.Time) ServiceOption {
	return func(s *Service) error {
		if fn != nil {
			s.now = fn
		}
		return nil
	}
}

// NewService constructs Service with optional configuration.
func NewService(store Store, opts ...ServiceOption) (*Service, error) {
	svc := &Service{
		store:      store,
		now:        time.Now,
		accessTTL:  defaultAccessTTL,
		refreshTTL: defaultRefreshTTL,
	}
	for _, opt := range opts {
		if err := opt(svc); err != nil {
			return nil, err
		}
	}
	return svc, nil
}

// EnsureBuiltins ensures predefined permissions exist.
func (s *Service) EnsureBuiltins(ctx context.Context) error {
	return s.store.Permissions(ctx).Ensure(ctx, BuiltinPermissions)
}

// Principal loads user with resolved permissions.
func (s *Service) Principal(ctx context.Context, userID string) (Principal, error) {
	users := s.store.Users(ctx)
	roles := s.store.Roles(ctx)
	perms := s.store.Permissions(ctx)

	user, err := users.Find(ctx, userID)
	if err != nil {
		return Principal{}, err
	}
	assignments, err := roles.Assignments(ctx, userID)
	if err != nil {
		return Principal{}, err
	}
	permMap := make(map[string]struct{})
	for _, a := range assignments {
		list, err := perms.PermissionsForRole(ctx, a.RoleID)
		if err != nil {
			return Principal{}, err
		}
		for _, p := range list {
			permMap[p.Key] = struct{}{}
		}
	}
	return Principal{User: user, Assignments: assignments, Permissions: permMap}, nil
}

// Require ensures user has a permission.
func (s *Service) Require(ctx context.Context, userID, perm string) (Principal, error) {
	principal, err := s.Principal(ctx, userID)
	if err != nil {
		return Principal{}, err
	}
	if !principal.HasPermission(perm) {
		return Principal{}, ErrUnauthorized
	}
	return principal, nil
}

// AssignRole ensures role assignments.
func (s *Service) AssignRole(ctx context.Context, assignment Assignment) error {
	return s.store.Roles(ctx).Assign(ctx, assignment)
}

// AppendAudit records action in append-only log.
func (s *Service) AppendAudit(ctx context.Context, entry *AuditEntry) error {
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = s.now().UTC()
	}
	return s.store.Audit(ctx).Append(ctx, entry)
}

// SupportsTokens reports whether bearer token verification is enabled.
func (s *Service) SupportsTokens() bool {
	return s.privateKey != nil && s.publicKey != nil || len(s.tokenSecret) > 0
}

func (s *Service) canIssueTokens() bool {
	return s.privateKey != nil && s.publicKey != nil
}

// TokenPair represents access and refresh tokens along with their expirations.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

// IssueTokenPair authenticates user credentials and issues fresh tokens.
func (s *Service) IssueTokenPair(ctx context.Context, email, password string) (TokenPair, Principal, error) {
	if !s.canIssueTokens() {
		return TokenPair{}, Principal{}, ErrNotImplemented
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return TokenPair{}, Principal{}, ErrUnauthorized
	}
	user, err := s.store.Users(ctx).FindByEmail(ctx, email)
	if err != nil {
		return TokenPair{}, Principal{}, ErrUnauthorized
	}
	if user.Status != "active" {
		return TokenPair{}, Principal{}, ErrUnauthorized
	}
	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return TokenPair{}, Principal{}, ErrUnauthorized
	}
	principal, err := s.Principal(ctx, user.ID)
	if err != nil {
		return TokenPair{}, Principal{}, err
	}
	pair, err := s.mintTokens(ctx, principal)
	if err != nil {
		return TokenPair{}, Principal{}, err
	}
	return pair, principal, nil
}

// RefreshTokenPair rotates refresh token and issues new access credentials.
func (s *Service) RefreshTokenPair(ctx context.Context, refreshToken string) (TokenPair, Principal, error) {
	if !s.canIssueTokens() {
		return TokenPair{}, Principal{}, ErrNotImplemented
	}
	tokenID, secret, err := splitRefreshToken(refreshToken)
	if err != nil {
		return TokenPair{}, Principal{}, ErrInvalidToken
	}

	store := s.store.RefreshTokens(ctx)
	record, err := store.Find(ctx, tokenID)
	if err != nil {
		return TokenPair{}, Principal{}, ErrInvalidToken
	}
	if record.Revoked || s.now().After(record.ExpiresAt) {
		return TokenPair{}, Principal{}, ErrInvalidToken
	}
	if !secureCompareHash(record.TokenHash, secret) {
		_ = store.MarkRevoked(ctx, record.ID)
		return TokenPair{}, Principal{}, ErrInvalidToken
	}

	principal, err := s.Principal(ctx, record.UserID)
	if err != nil {
		return TokenPair{}, Principal{}, err
	}

	// Rotate refresh token: revoke old, issue new pair
	if err := store.MarkRevoked(ctx, record.ID); err != nil {
		return TokenPair{}, Principal{}, err
	}

	pair, err := s.mintTokens(ctx, principal)
	if err != nil {
		return TokenPair{}, Principal{}, err
	}
	return pair, principal, nil
}

func (s *Service) mintTokens(ctx context.Context, principal Principal) (TokenPair, error) {
	now := s.now()
	accessToken, accessExp, err := s.signAccessToken(principal, now)
	if err != nil {
		return TokenPair{}, err
	}
	refreshTokenString, refreshRec, err := s.generateRefreshToken(principal.User.ID, now)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.store.RefreshTokens(ctx).Create(ctx, refreshRec); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshTokenString,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshRec.ExpiresAt,
	}, nil
}

func (s *Service) generateRefreshToken(userID string, now time.Time) (string, *RefreshToken, error) {
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	tokenID := ids.New()
	sum := sha256.Sum256([]byte(secret))
	rec := &RefreshToken{
		ID:        tokenID,
		UserID:    userID,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: now.Add(s.refreshTTL),
	}
	tokenString := tokenID + "." + secret
	return tokenString, rec, nil
}

func splitRefreshToken(raw string) (id, secret string, err error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return "", "", errors.New("invalid refresh token format")
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("invalid refresh token format")
	}
	return parts[0], parts[1], nil
}

func secureCompareHash(expectedHash string, secret string) bool {
	sum := sha256.Sum256([]byte(secret))
	actual := hex.EncodeToString(sum[:])
	return subtleCompare(expectedHash, actual)
}

// AuthenticateToken validates access token and returns principal.
func (s *Service) AuthenticateToken(ctx context.Context, token string) (Principal, error) {
	if !s.SupportsTokens() {
		return Principal{}, ErrNotImplemented
	}
	claims, err := s.verifyAccessToken(token)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	principal, err := s.Principal(ctx, claims.Subject)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Principal{}, ErrInvalidToken
		}
		return Principal{}, err
	}
	return principal, nil
}

func (s *Service) signAccessToken(principal Principal, now time.Time) (string, time.Time, error) {
	if s.privateKey == nil {
		return "", time.Time{}, ErrNotImplemented
	}
	exp := now.Add(s.accessTTL)
	jti := ids.New()
	claims := map[string]any{
		"sub":         principal.User.ID,
		"iat":         now.Unix(),
		"exp":         exp.Unix(),
		"token_type":  "access",
		"jti":         jti,
		"org":         principal.User.OrganizationID,
		"permissions": sortedPermissions(principal.Permissions),
	}
	if s.issuer != "" {
		claims["iss"] = s.issuer
	}
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	if s.keyID != "" {
		header["kid"] = s.keyID
	}
	token, err := signJWT(header, claims, s.privateKey)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func (s *Service) verifyAccessToken(token string) (TokenClaims, error) {
	parts, header, payload, err := parseJWT(token)
	if err != nil {
		return TokenClaims{}, err
	}
	alg, _ := header["alg"].(string)
	switch alg {
	case "RS256":
		if s.publicKey == nil {
			return TokenClaims{}, ErrInvalidToken
		}
		if err := verifyRS256(parts.signingString, parts.signature, s.publicKey); err != nil {
			return TokenClaims{}, ErrInvalidToken
		}
	case "HS256":
		if len(s.tokenSecret) == 0 {
			return TokenClaims{}, ErrInvalidToken
		}
		if err := verifyHMAC(parts.signingString, parts.signature, s.tokenSecret); err != nil {
			return TokenClaims{}, ErrInvalidToken
		}
	default:
		return TokenClaims{}, ErrInvalidToken
	}

	claims, err := extractClaims(payload)
	if err != nil {
		return TokenClaims{}, ErrInvalidToken
	}
	if s.issuer != "" && claims.Issuer != "" && !strings.EqualFold(claims.Issuer, s.issuer) {
		return TokenClaims{}, ErrInvalidToken
	}
	if claims.TokenType != "access" {
		return TokenClaims{}, ErrInvalidToken
	}
	if claims.ExpiresAt > 0 && s.now().Unix() > claims.ExpiresAt {
		return TokenClaims{}, ErrInvalidToken
	}
	return claims, nil
}

type jwtParts struct {
	header        map[string]any
	payload       map[string]any
	signingString string
	signature     []byte
}

func parseJWT(token string) (*jwtParts, map[string]any, map[string]any, error) {
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return nil, nil, nil, ErrInvalidToken
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(segments[0])
	if err != nil {
		return nil, nil, nil, ErrInvalidToken
	}
	payloadJSON, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return nil, nil, nil, ErrInvalidToken
	}
	signature, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		return nil, nil, nil, ErrInvalidToken
	}
	var header map[string]any
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, nil, nil, ErrInvalidToken
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, nil, nil, ErrInvalidToken
	}
	return &jwtParts{
		header:        header,
		payload:       payload,
		signingString: segments[0] + "." + segments[1],
		signature:     signature,
	}, header, payload, nil
}

func signJWT(header, payload map[string]any, key *rsa.PrivateKey) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	headerSegment := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadSegment := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingString := headerSegment + "." + payloadSegment
	hash := sha256.Sum256([]byte(signingString))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	signatureSegment := base64.RawURLEncoding.EncodeToString(sig)
	return signingString + "." + signatureSegment, nil
}

func verifyRS256(signingString string, signature []byte, key *rsa.PublicKey) error {
	hash := sha256.Sum256([]byte(signingString))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature)
}

func verifyHMAC(signingString string, signature []byte, secret []byte) error {
	expected := hmacSign([]byte(signingString), secret)
	if subtle.ConstantTimeCompare(signature, expected) != 1 {
		return ErrInvalidToken
	}
	return nil
}

func extractClaims(payload map[string]any) (TokenClaims, error) {
	sub, _ := payload["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return TokenClaims{}, ErrInvalidToken
	}
	claim := TokenClaims{Subject: sub}
	if iss, ok := payload["iss"].(string); ok {
		claim.Issuer = iss
	}
	if typ, ok := payload["token_type"].(string); ok {
		claim.TokenType = typ
	}
	if exp, ok := toInt64(payload["exp"]); ok {
		claim.ExpiresAt = exp
	}
	if iat, ok := toInt64(payload["iat"]); ok {
		claim.IssuedAt = iat
	}
	return claim, nil
}

func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
}

func sortedPermissions(perms map[string]struct{}) []string {
	out := make([]string, 0, len(perms))
	for k := range perms {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid PEM private key")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("unsupported private key type")
	default:
		return nil, fmt.Errorf("unsupported private key type %s", block.Type)
	}
}

func parseRSAPublicKey(pemData string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid PEM public key")
	}
	switch block.Type {
	case "PUBLIC KEY":
		key, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("not an RSA public key")
		}
		return rsaKey, nil
	case "RSA PUBLIC KEY":
		return x509.ParsePKCS1PublicKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported public key type %s", block.Type)
	}
}

func hmacSign(data []byte, secret []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(data)
	return mac.Sum(nil)
}
