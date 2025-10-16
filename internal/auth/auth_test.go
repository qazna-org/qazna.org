package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"slices"
)

func TestServiceGenerateAndValidate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("select kid, private_pem, public_pem, expires_at.*from auth_keys").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("update auth_keys set status = 'retired'").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("insert into auth_keys").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc, err := NewService(db, WithIssuer("test-issuer"), WithKeyTTL(time.Hour), WithRotateWindow(15*time.Minute))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	token, expiresAt, err := svc.GenerateToken(context.Background(), "user-42", []string{"Admin", "viewer", "admin"}, 30*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatalf("expected future expiration, got %v", expiresAt)
	}

	claims, err := svc.ParseAndValidate(context.Background(), token)
	if err != nil {
		t.Fatalf("ParseAndValidate: %v", err)
	}
	if claims.Subject != "user-42" {
		t.Fatalf("unexpected subject: %s", claims.Subject)
	}
	if claims.Issuer != "test-issuer" {
		t.Fatalf("unexpected issuer: %s", claims.Issuer)
	}
	if !slices.Contains(claims.Roles, "admin") || !slices.Contains(claims.Roles, "viewer") {
		t.Fatalf("roles were not preserved: %v", claims.Roles)
	}

	pubPEM, err := encodePublicKey(svc.active.PublicKey)
	if err != nil {
		t.Fatalf("encodePublicKey: %v", err)
	}
	mock.ExpectQuery("select kid, public_pem from auth_keys").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"kid", "public_pem"}).AddRow(svc.active.Kid, pubPEM))

	jwksBytes, err := svc.JWKS(context.Background())
	if err != nil {
		t.Fatalf("JWKS: %v", err)
	}
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(jwksBytes, &jwks); err != nil {
		t.Fatalf("decode jwks: %v", err)
	}
	if len(jwks.Keys) == 0 || jwks.Keys[0].Kid != svc.active.Kid {
		t.Fatalf("expected jwks to include key %s", svc.active.Kid)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUser(ctx, "user-7", []string{"Admin", "Admin", "viewer"})
	id, ok := UserIDFromContext(ctx)
	if !ok || id != "user-7" {
		t.Fatalf("unexpected user id: %s, ok=%v", id, ok)
	}
	roles := RolesFromContext(ctx)
	if len(roles) != 2 {
		t.Fatalf("expected deduplicated roles, got %v", roles)
	}
	if !HasRole(ctx, "viewer") || !HasRole(ctx, "admin") {
		t.Fatalf("HasRole missing expected roles: %v", roles)
	}
	if HasRole(ctx, "operator") {
		t.Fatalf("unexpected role found")
	}
}

func TestIssueAndExchangeAuthCode(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("select kid, private_pem, public_pem, expires_at.*from auth_keys").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("update auth_keys set status = 'retired'").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("insert into auth_keys").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	svc, err := NewService(db)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.mu.Lock()
	if svc.active != nil {
		svc.active.ExpiresAt = time.Now().Add(48 * time.Hour)
	}
	svc.mu.Unlock()

	verifier := "demo-verifier"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	mock.ExpectQuery("select secret, redirect_uri from oauth_clients").WithArgs("demo-client").WillReturnRows(sqlmock.NewRows([]string{"secret", "redirect_uri"}).AddRow("demo-secret", "http://localhost/callback"))
	mock.ExpectExec("insert into oauth_auth_codes").WithArgs(sqlmock.AnyArg(), "demo-client", challenge, "S256", "http://localhost/callback", "demo-user", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

	code, err := svc.IssueAuthCode(context.Background(), AuthCodeRequest{
		ClientID:            "demo-client",
		RedirectURI:         "http://localhost/callback",
		CodeChallenge:       challenge,
		CodeChallengeMethod: "S256",
		UserID:              "demo-user",
		Roles:               []string{"admin"},
	})
	if err != nil {
		t.Fatalf("IssueAuthCode: %v", err)
	}
	if code.Code == "" {
		t.Fatalf("expected code")
	}

	rolesRaw, _ := json.Marshal([]string{"admin"})
	expires := time.Now().Add(2 * time.Minute)
	mock.ExpectQuery("select a.code_challenge").WithArgs(code.Code, "demo-client").WillReturnRows(sqlmock.NewRows([]string{"code_challenge", "code_challenge_method", "redirect_uri", "user_id", "roles", "expires_at", "consumed_at", "secret"}).AddRow(challenge, "S256", "http://localhost/callback", "demo-user", rolesRaw, expires, nil, "demo-secret"))
	mock.ExpectExec("update oauth_auth_codes set consumed_at").WithArgs(sqlmock.AnyArg(), code.Code).WillReturnResult(sqlmock.NewResult(1, 1))

	token, exp, err := svc.ExchangeAuthCode(context.Background(), AuthCodeExchangeRequest{
		ClientID:     "demo-client",
		ClientSecret: "demo-secret",
		Code:         code.Code,
		CodeVerifier: verifier,
	})
	if err != nil {
		t.Fatalf("ExchangeAuthCode: %v", err)
	}
	if token == "" {
		t.Fatalf("expected token")
	}
	if !exp.After(time.Now()) {
		t.Fatalf("unexpected expiry: %v", exp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
