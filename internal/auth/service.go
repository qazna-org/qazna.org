package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	issuerDefault         = "qazna"
	defaultKeyTTL         = 48 * time.Hour
	defaultRotationWindow = 12 * time.Hour
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrKeyNotFound  = errors.New("signing key not found")
)

type Claims struct {
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

type keyRecord struct {
	Kid        string
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	ExpiresAt  time.Time
}

type Service struct {
	db       *sql.DB
	issuer   string
	keyTTL   time.Duration
	rotateIn time.Duration
	codeTTL  time.Duration

	mu         sync.RWMutex
	active     *keyRecord
	verifyMu   sync.RWMutex
	verifyKeys map[string]*rsa.PublicKey
}

type Option func(*Service)

func WithIssuer(issuer string) Option {
	return func(s *Service) {
		if strings.TrimSpace(issuer) != "" {
			s.issuer = issuer
		}
	}
}

func WithKeyTTL(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.keyTTL = d
		}
	}
}

func WithRotateWindow(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.rotateIn = d
		}
	}
}

func WithAuthCodeTTL(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.codeTTL = d
		}
	}
}

func NewService(db *sql.DB, opts ...Option) (*Service, error) {
	if db == nil {
		return nil, errors.New("auth service requires database connection")
	}
	svc := &Service{
		db:         db,
		issuer:     issuerDefault,
		keyTTL:     defaultKeyTTL,
		rotateIn:   defaultRotationWindow,
		codeTTL:    5 * time.Minute,
		verifyKeys: make(map[string]*rsa.PublicKey),
	}
	for _, opt := range opts {
		opt(svc)
	}
	if err := svc.ensureActiveKey(context.Background()); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) GenerateToken(ctx context.Context, userID string, roles []string, ttl time.Duration) (string, time.Time, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", time.Time{}, errors.New("userID is required")
	}
	if ttl <= 0 {
		return "", time.Time{}, errors.New("ttl must be greater than zero")
	}

	if err := s.ensureActiveKey(ctx); err != nil {
		return "", time.Time{}, err
	}

	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if active == nil {
		return "", time.Time{}, ErrKeyNotFound
	}
	if ttl > time.Until(active.ExpiresAt) {
		ttl = time.Until(active.ExpiresAt)
	}

	now := time.Now().UTC()
	claims := Claims{
		Roles: dedupeRoles(roles),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = active.Kid
	signed, err := token.SignedString(active.PrivateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, now.Add(ttl), nil
}

type AuthCodeRequest struct {
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	UserID              string
	Roles               []string
}

type AuthCode struct {
	Code        string
	RedirectURI string
	ExpiresAt   time.Time
}

type AuthCodeExchangeRequest struct {
	ClientID     string
	ClientSecret string
	Code         string
	CodeVerifier string
}

func (s *Service) IssueAuthCode(ctx context.Context, req AuthCodeRequest) (*AuthCode, error) {
	if s.db == nil {
		return nil, errors.New("auth service missing database connection")
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	if req.ClientID == "" {
		return nil, errors.New("client_id is required")
	}
	req.RedirectURI = strings.TrimSpace(req.RedirectURI)
	if req.RedirectURI == "" {
		return nil, errors.New("redirect_uri is required")
	}
	challenge := strings.TrimSpace(req.CodeChallenge)
	if challenge == "" {
		return nil, errors.New("code_challenge is required")
	}
	method := strings.ToUpper(strings.TrimSpace(req.CodeChallengeMethod))
	if method == "" {
		method = "S256"
	}
	if method != "S256" && method != "PLAIN" {
		return nil, fmt.Errorf("unsupported code_challenge_method %s", method)
	}
	user := strings.TrimSpace(req.UserID)
	if user == "" {
		return nil, errors.New("user is required")
	}

	var storedSecret, storedRedirect string
	if err := s.db.QueryRowContext(ctx, `select secret, redirect_uri from oauth_clients where id=$1`, req.ClientID).Scan(&storedSecret, &storedRedirect); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("oauth client %s not found", req.ClientID)
		}
		return nil, err
	}
	if storedRedirect != req.RedirectURI {
		return nil, errors.New("redirect_uri mismatch")
	}

	roles := dedupeRoles(req.Roles)
	rolesJSON, err := json.Marshal(roles)
	if err != nil {
		return nil, err
	}
	code := uuid.NewString()
	expires := time.Now().UTC().Add(s.codeTTL)

	if _, err := s.db.ExecContext(ctx, `
		insert into oauth_auth_codes(code, client_id, code_challenge, code_challenge_method, redirect_uri, user_id, roles, expires_at)
		values ($1,$2,$3,$4,$5,$6,$7,$8)
	`, code, req.ClientID, challenge, method, req.RedirectURI, user, rolesJSON, expires); err != nil {
		return nil, err
	}

	return &AuthCode{Code: code, RedirectURI: req.RedirectURI, ExpiresAt: expires}, nil
}

func (s *Service) ExchangeAuthCode(ctx context.Context, req AuthCodeExchangeRequest) (string, time.Time, error) {
	if s.db == nil {
		return "", time.Time{}, errors.New("auth service missing database connection")
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	if req.ClientID == "" {
		return "", time.Time{}, errors.New("client_id is required")
	}
	req.Code = strings.TrimSpace(req.Code)
	if req.Code == "" {
		return "", time.Time{}, errors.New("code is required")
	}
	req.CodeVerifier = strings.TrimSpace(req.CodeVerifier)
	if req.CodeVerifier == "" {
		return "", time.Time{}, errors.New("code_verifier is required")
	}

	var (
		challenge    string
		method       string
		redirectURI  string
		userID       string
		rolesRaw     []byte
		expires      time.Time
		consumed     sql.NullTime
		clientSecret string
	)
	row := s.db.QueryRowContext(ctx, `
		select a.code_challenge, a.code_challenge_method, a.redirect_uri, a.user_id, a.roles, a.expires_at, a.consumed_at, c.secret
		from oauth_auth_codes a
		join oauth_clients c on c.id = a.client_id
		where a.code = $1 and a.client_id = $2
	`, req.Code, req.ClientID)
	if err := row.Scan(&challenge, &method, &redirectURI, &userID, &rolesRaw, &expires, &consumed, &clientSecret); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", time.Time{}, errors.New("authorization code not found")
		}
		return "", time.Time{}, err
	}
	if consumed.Valid {
		return "", time.Time{}, errors.New("authorization code already used")
	}
	if time.Now().UTC().After(expires) {
		return "", time.Time{}, errors.New("authorization code expired")
	}
	if clientSecret != "" && req.ClientSecret != clientSecret {
		return "", time.Time{}, errors.New("invalid client secret")
	}

	switch strings.ToUpper(method) {
	case "S256":
		sum := sha256.Sum256([]byte(req.CodeVerifier))
		expected := base64.RawURLEncoding.EncodeToString(sum[:])
		if expected != challenge {
			return "", time.Time{}, errors.New("invalid code verifier")
		}
	case "PLAIN":
		if challenge != req.CodeVerifier {
			return "", time.Time{}, errors.New("invalid code verifier")
		}
	default:
		return "", time.Time{}, fmt.Errorf("unsupported code challenge method %s", method)
	}

	var roles []string
	if len(rolesRaw) > 0 {
		if err := json.Unmarshal(rolesRaw, &roles); err != nil {
			return "", time.Time{}, err
		}
	}

	token, expiresAt, err := s.GenerateToken(ctx, userID, roles, 15*time.Minute)
	if err != nil {
		return "", time.Time{}, err
	}

	if _, err := s.db.ExecContext(ctx, `update oauth_auth_codes set consumed_at = $1 where code = $2`, time.Now().UTC(), req.Code); err != nil {
		return "", time.Time{}, err
	}

	return token, expiresAt, nil
}

func (s *Service) ParseAndValidate(ctx context.Context, tokenStr string) (*Claims, error) {
	tokenStr = strings.TrimSpace(tokenStr)
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}
	parsed, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, ErrInvalidToken
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, ErrInvalidToken
		}
		pub, err := s.lookupKey(ctx, kid)
		if err != nil {
			return nil, err
		}
		return pub, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	if err := s.validateClaims(claims); err != nil {
		return nil, ErrInvalidToken
	}
	claims.Roles = dedupeRoles(claims.Roles)
	return claims, nil
}

func (s *Service) validateClaims(claims *Claims) error {
	if claims.Issuer != s.issuer {
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
	if claims.IssuedAt.Time.After(now.Add(5 * time.Second)) {
		return errors.New("token issued in the future")
	}
	if claims.ExpiresAt.Time.Before(claims.IssuedAt.Time) {
		return errors.New("token expiry precedes issued-at")
	}
	return nil
}

func (s *Service) ensureActiveKey(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil && time.Until(s.active.ExpiresAt) > s.rotateIn {
		return nil
	}
	rec, err := s.fetchActiveKey(ctx)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else if rec != nil {
		if s.active == nil || rec.Kid != s.active.Kid {
			s.active = rec
		}
		if time.Until(rec.ExpiresAt) > s.rotateIn {
			return nil
		}
	}
	newRec, err := s.generateAndStoreKey(ctx)
	if err != nil {
		return err
	}
	s.active = newRec
	s.verifyMu.Lock()
	s.verifyKeys[newRec.Kid] = newRec.PublicKey
	s.verifyMu.Unlock()
	return nil
}

func (s *Service) fetchActiveKey(ctx context.Context) (*keyRecord, error) {
	row := s.db.QueryRowContext(ctx, `
        select kid, private_pem, public_pem, expires_at
        from auth_keys
        where status = 'active'
        order by created_at desc
        limit 1`)
	var kid, privPEM, pubPEM string
	var expires time.Time
	if err := row.Scan(&kid, &privPEM, &pubPEM, &expires); err != nil {
		return nil, err
	}
	privKey, err := parsePrivateKey(privPEM)
	if err != nil {
		return nil, err
	}
	pubKey, err := parsePublicKey(pubPEM)
	if err != nil {
		return nil, err
	}
	return &keyRecord{Kid: kid, PrivateKey: privKey, PublicKey: pubKey, ExpiresAt: expires}, nil
}

func (s *Service) generateAndStoreKey(ctx context.Context) (*keyRecord, error) {
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, err
	}
	kid := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(s.keyTTL)

	privPEM := encodePrivateKey(key)
	pubPEM, err := encodePublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `update auth_keys set status = 'retired', rotated_at = $1 where status = 'active'`, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
        insert into auth_keys (kid, public_pem, private_pem, created_at, expires_at, status)
        values ($1,$2,$3,$4,$5,'active')
    `, kid, pubPEM, privPEM, now, expires); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &keyRecord{Kid: kid, PrivateKey: key, PublicKey: &key.PublicKey, ExpiresAt: expires}, nil
}

func (s *Service) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	s.verifyMu.RLock()
	if key, ok := s.verifyKeys[kid]; ok {
		s.verifyMu.RUnlock()
		return key, nil
	}
	s.verifyMu.RUnlock()

	row := s.db.QueryRowContext(ctx, `select public_pem from auth_keys where kid = $1 limit 1`, kid)
	var pubPEM string
	if err := row.Scan(&pubPEM); err != nil {
		return nil, ErrInvalidToken
	}
	pubKey, err := parsePublicKey(pubPEM)
	if err != nil {
		return nil, err
	}
	s.verifyMu.Lock()
	s.verifyKeys[kid] = pubKey
	s.verifyMu.Unlock()
	return pubKey, nil
}

func (s *Service) JWKS(ctx context.Context) ([]byte, error) {
	rows, err := s.db.QueryContext(ctx, `
        select kid, public_pem from auth_keys
        where status = 'active' or (status = 'retired' and expires_at >= $1)
    `, time.Now().UTC().Add(-s.rotateIn))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type jwk struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	var jwks struct {
		Keys []jwk `json:"keys"`
	}
	for rows.Next() {
		var kid, pubPEM string
		if err := rows.Scan(&kid, &pubPEM); err != nil {
			return nil, err
		}
		pubKey, err := parsePublicKey(pubPEM)
		if err != nil {
			return nil, err
		}
		jwks.Keys = append(jwks.Keys, jwk{
			Kty: "RSA",
			Kid: kid,
			Use: "sig",
			Alg: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(jwks)
}

// Context helpers

type ctxKey string

const (
	userIDKey ctxKey = "auth_user_id"
	rolesKey  ctxKey = "auth_roles"
)

func ContextWithUser(ctx context.Context, userID string, roles []string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, strings.TrimSpace(userID))
	if len(roles) > 0 {
		ctx = context.WithValue(ctx, rolesKey, dedupeRoles(roles))
	}
	return ctx
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	if !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

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

func encodePrivateKey(key *rsa.PrivateKey) string {
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	return string(pem.EncodeToMemory(block))
}

func encodePublicKey(pub *rsa.PublicKey) (string, error) {
	bytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", err
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: bytes}
	return string(pem.EncodeToMemory(block)), nil
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, errors.New("invalid private key PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pub, ok := pubAny.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("unexpected public key type")
	}
	return pub, nil
}
