package httpapi

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"qazna.org/internal/auth"
	"qazna.org/internal/ledger"
	"qazna.org/internal/stream"
)

type apiClient struct {
	baseURL string
	client  *http.Client
	t       *testing.T
	mock    sqlmock.Sqlmock
}

func newTestAPI(t *testing.T, store auth.RBACStore) *apiClient {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	mock.ExpectQuery("select kid, private_pem, public_pem, expires_at.*from auth_keys").WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectExec("update auth_keys set status = 'retired'").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("insert into auth_keys").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	authSvc, err := auth.NewService(db, auth.WithIssuer("test"), auth.WithKeyTTL(time.Hour), auth.WithRotateWindow(20*time.Minute))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var rbacSvc *auth.RBACService
	if store != nil {
		var svcErr error
		rbacSvc, svcErr = auth.NewRBACService(store)
		if svcErr != nil {
			t.Fatalf("NewRBACService: %v", svcErr)
		}
	}

	api := New(ReadyProbe{}, "test", ledger.NewInMemory(), stream.New(), nil, authSvc, rbacSvc)
	api.rateBurst = 100
	api.ratePerSec = 100

	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
		_ = db.Close()
	})

	return &apiClient{
		baseURL: srv.URL,
		client:  srv.Client(),
		t:       t,
		mock:    mock,
	}
}

func (c *apiClient) post(path string, body any, headers map[string]string) *http.Response {
	c.t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("do request: %v", err)
	}
	return resp
}

func (c *apiClient) get(path string, params url.Values, headers map[string]string) *http.Response {
	c.t.Helper()
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		c.t.Fatalf("parse url: %v", err)
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		c.t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.t.Fatalf("get request: %v", err)
	}
	return resp
}

func (c *apiClient) obtainToken(user string, roles []string) string {
	c.t.Helper()
	resp := c.post("/v1/auth/token", map[string]any{
		"user":  user,
		"roles": roles,
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.t.Fatalf("unexpected token status: %d", resp.StatusCode)
	}
	var payload tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		c.t.Fatalf("decode token response: %v", err)
	}
	if payload.Token == "" {
		c.t.Fatalf("empty token issued")
	}
	return payload.Token
}

func decode[T any](t *testing.T, r *http.Response) T {
	t.Helper()
	defer r.Body.Close()
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

func TestAPIAccountsTransferFlow(t *testing.T) {
	api := newTestAPI(t, nil)
	token := api.obtainToken("demo", []string{"admin"})
	authHeader := map[string]string{"Authorization": "Bearer " + token}

	// Create account A with initial QZN balance.
	resp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 100000,
	}, authHeader)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	accA := decode[map[string]any](t, resp)
	idA := accA["id"].(string)

	// Create empty account B.
	resp = api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, authHeader)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	accB := decode[map[string]any](t, resp)
	idB := accB["id"].(string)

	// Transfer 25000 with idempotency key.
	headers := map[string]string{
		"Idempotency-Key": "test-key-1",
		"Authorization":   "Bearer " + token,
	}
	req := map[string]any{
		"from_id":  idA,
		"to_id":    idB,
		"currency": "QZN",
		"amount":   25000,
	}
	resp = api.post("/v1/transfers", req, headers)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	tx := decode[map[string]any](t, resp)
	if tx["amount"].(float64) != 25000 {
		t.Fatalf("unexpected transfer amount: %v", tx["amount"])
	}
	if resp.Header.Get("Idempotency-Key") != "test-key-1" {
		t.Fatalf("missing idempotency header echo")
	}

	// Repeat the same request: expect identical transaction.
	resp = api.post("/v1/transfers", req, headers)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	tx2 := decode[map[string]any](t, resp)
	if tx2["id"] != tx["id"] {
		t.Fatalf("idempotent call returned different transaction id")
	}

	// Query balances.
	resp = api.get("/v1/accounts/"+idA+"/balance", url.Values{"currency": []string{"QZN"}}, authHeader)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	balA := decode[map[string]any](t, resp)
	if balA["amount"].(float64) != 75000 {
		t.Fatalf("unexpected balance for account A: %v", balA["amount"])
	}

	resp = api.get("/v1/accounts/"+idB+"/balance", url.Values{"currency": []string{"QZN"}}, authHeader)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	balB := decode[map[string]any](t, resp)
	if balB["amount"].(float64) != 25000 {
		t.Fatalf("unexpected balance for account B: %v", balB["amount"])
	}

	// List transactions.
	resp = api.get("/v1/ledger/transactions", url.Values{"limit": []string{"10"}}, authHeader)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	payload := decode[map[string]any](t, resp)
	if payload["next_after"] == nil {
		t.Fatalf("expected pagination field present")
	}
}

func TestAPIEnforcesAuth(t *testing.T) {
	api := newTestAPI(t, nil)

	resp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	var errBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if errBody["error"] == "" {
		t.Fatalf("expected error message")
	}
}

func TestTokenEndpointValidation(t *testing.T) {
	api := newTestAPI(t, nil)

	resp := api.post("/v1/auth/token", map[string]any{"user": ""}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestOAuthPKCEFlow(t *testing.T) {
	api := newTestAPI(t, nil)

	verifier := "sample-verifier"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	api.mock.ExpectQuery("select secret, redirect_uri from oauth_clients").WithArgs("demo-client").WillReturnRows(sqlmock.NewRows([]string{"secret", "redirect_uri"}).AddRow("demo-secret", "http://localhost/callback"))
	api.mock.ExpectExec("insert into oauth_auth_codes").WithArgs(sqlmock.AnyArg(), "demo-client", challenge, "S256", "http://localhost/callback", "demo-user", sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

	resp := api.post("/v1/auth/oauth/authorize", map[string]any{
		"client_id":             "demo-client",
		"redirect_uri":          "http://localhost/callback",
		"code_challenge":        challenge,
		"code_challenge_method": "S256",
		"user":                  "demo-user",
		"roles":                 []string{"admin"},
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorize expected 200, got %d", resp.StatusCode)
	}
	var authResp authCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil {
		t.Fatalf("decode authorize: %v", err)
	}
	_ = resp.Body.Close()
	if authResp.Code == "" {
		t.Fatalf("expected authorization code")
	}

	rolesRaw, _ := json.Marshal([]string{"admin"})
	expires := time.Now().Add(3 * time.Minute)
	api.mock.ExpectQuery("select a.code_challenge").WithArgs(authResp.Code, "demo-client").WillReturnRows(sqlmock.NewRows([]string{"code_challenge", "code_challenge_method", "redirect_uri", "user_id", "roles", "expires_at", "consumed_at", "secret"}).AddRow(challenge, "S256", "http://localhost/callback", "demo-user", rolesRaw, expires, nil, "demo-secret"))
	api.mock.ExpectExec("update oauth_auth_codes set consumed_at").WithArgs(sqlmock.AnyArg(), authResp.Code).WillReturnResult(sqlmock.NewResult(1, 1))

	resp = api.post("/v1/auth/oauth/token", map[string]any{
		"client_id":     "demo-client",
		"client_secret": "demo-secret",
		"code":          authResp.Code,
		"code_verifier": verifier,
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token expected 200, got %d", resp.StatusCode)
	}
	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if tok.Token == "" {
		t.Fatalf("expected token in response")
	}
}
