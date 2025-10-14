package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"qazna.org/internal/auth"
	"qazna.org/internal/ledger"
	"qazna.org/internal/stream"
)

var fixedNow = time.Unix(1700000000, 0)

type apiClient struct {
	baseURL string
	client  *http.Client
	t       *testing.T
}

func newTestAPI(t *testing.T) *apiClient {
	return newTestAPIWithAuth(t, nil)
}

func newTestAPIWithAuth(t *testing.T, authSvc *auth.Service) *apiClient {
	t.Helper()
	api := New(ReadyProbe{}, "test", ledger.NewInMemory(), stream.New(), nil, authSvc)
	api.rateBurst = 100
	api.ratePerSec = 100

	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)

	return &apiClient{
		baseURL: srv.URL,
		client:  srv.Client(),
		t:       t,
	}
}

func (c *apiClient) post(path string, body any, headers map[string]string) *http.Response {
	c.t.Helper()
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal body: %v", err)
		}
		payload = bytes.NewReader(b)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, payload)
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

func (c *apiClient) get(path string, params url.Values) *http.Response {
	c.t.Helper()
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		c.t.Fatalf("parse url: %v", err)
	}
	if params != nil {
		u.RawQuery = params.Encode()
	}
	resp, err := c.client.Get(u.String())
	if err != nil {
		c.t.Fatalf("get: %v", err)
	}
	return resp
}

func decode[T any](t *testing.T, r io.Reader) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return v
}

func obtainToken(t *testing.T, api *apiClient, email, password string) tokenResponse {
	t.Helper()
	resp := api.post("/v1/auth/token", map[string]string{
		"grant_type": "password",
		"email":      email,
		"password":   password,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from token endpoint, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()
	return decode[tokenResponse](t, resp.Body)
}

func TestAPIAccountsTransferFlow(t *testing.T) {
	api := newTestAPI(t)

	// Create account A with initial QZN balance.
	resp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 100000,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	accA := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	idA := accA["id"].(string)

	// Create empty account B.
	resp = api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	accB := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	idB := accB["id"].(string)

	// Transfer 25000 with idempotency key.
	headers := map[string]string{"Idempotency-Key": "test-key-1"}
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
	tx := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
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
	tx2 := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if tx2["id"] != tx["id"] {
		t.Fatalf("idempotent call returned different transaction id")
	}

	// Query balances.
	resp = api.get("/v1/accounts/"+idA+"/balance", url.Values{"currency": []string{"QZN"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	balA := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if balA["amount"].(float64) != 75000 {
		t.Fatalf("unexpected balance for account A: %v", balA["amount"])
	}

	resp = api.get("/v1/accounts/"+idB+"/balance", url.Values{"currency": []string{"QZN"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	balB := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if balB["amount"].(float64) != 25000 {
		t.Fatalf("unexpected balance for account B: %v", balB["amount"])
	}

	// List transactions.
	resp = api.get("/v1/ledger/transactions", url.Values{"limit": []string{"10"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	payload := decode[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	items := payload["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected single transaction, got %d", len(items))
	}
	if payload["next_after"].(float64) != items[0].(map[string]any)["sequence"].(float64) {
		t.Fatalf("unexpected next_after value")
	}
	if _, err := time.Parse(time.RFC3339, payload["as_of"].(string)); err != nil {
		t.Fatalf("invalid as_of timestamp: %v", err)
	}
}

func TestAPIValidationErrors(t *testing.T) {
	api := newTestAPI(t)

	resp := api.post("/v1/accounts", map[string]any{
		"initial_amount": 10,
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	resp = api.post("/v1/transfers", map[string]any{
		"from_id":  "a",
		"to_id":    "",
		"currency": "QZN",
		"amount":   0,
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAuthRequiresToken(t *testing.T) {
	user := &auth.User{ID: "user-1", OrganizationID: "org-1", Email: "user@example.com", Status: "active"}
	authSvc, _ := newStaticAuthService(t, user, "P@ssw0rd!", nil, nil)

	api := newTestAPIWithAuth(t, authSvc)

	resp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAuthPermissionDenied(t *testing.T) {
	user := &auth.User{ID: "user-2", OrganizationID: "org-2", Email: "user2@example.com", Status: "active"}
	assignments := []auth.Assignment{{UserID: user.ID, RoleID: "role-2", OrganizationID: user.OrganizationID}}
	authSvc, _ := newStaticAuthService(t, user, "StrongPass!", assignments, map[string][]string{
		"role-2": {},
	})

	api := newTestAPIWithAuth(t, authSvc)
	tokens := obtainToken(t, api, user.Email, "StrongPass!")

	resp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, map[string]string{
		"Authorization": "Bearer " + tokens.AccessToken,
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestAuthPermissionGranted(t *testing.T) {
	user := &auth.User{ID: "user-3", OrganizationID: "org-3", Email: "user3@example.com", Status: "active"}
	assignments := []auth.Assignment{{UserID: user.ID, RoleID: "role-3", OrganizationID: user.OrganizationID}}
	authSvc, store := newStaticAuthService(t, user, "SafePass#1", assignments, map[string][]string{
		"role-3": {auth.PermLedgerCreateAccount, auth.PermLedgerTransfer},
	})

	api := newTestAPIWithAuth(t, authSvc)
	tokens := obtainToken(t, api, user.Email, "SafePass#1")

	headers := map[string]string{"Authorization": "Bearer " + tokens.AccessToken}

	accountResp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 100,
	}, headers)
	if accountResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", accountResp.StatusCode)
	}
	acc := decode[map[string]any](t, accountResp.Body)
	_ = accountResp.Body.Close()
	accID := acc["id"].(string)

	targetResp := api.post("/v1/accounts", map[string]any{
		"currency":       "QZN",
		"initial_amount": 0,
	}, headers)
	if targetResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", targetResp.StatusCode)
	}
	target := decode[map[string]any](t, targetResp.Body)
	_ = targetResp.Body.Close()
	targetID := target["id"].(string)

	txResp := api.post("/v1/transfers", map[string]any{
		"from_id":  accID,
		"to_id":    targetID,
		"currency": "QZN",
		"amount":   50,
	}, map[string]string{
		"Authorization":   "Bearer " + tokens.AccessToken,
		"Idempotency-Key": "perm-granted",
	})
	if txResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", txResp.StatusCode)
	}
	_ = txResp.Body.Close()

	entries := store.audit.entries
	if len(entries) != 4 {
		t.Fatalf("expected 4 audit entries, got %d", len(entries))
	}
	if entries[0].Action != "auth.token.issue" {
		t.Fatalf("unexpected first audit action: %s", entries[0].Action)
	}
	if entries[1].Action != "ledger.account.create" {
		t.Fatalf("unexpected second audit action: %s", entries[1].Action)
	}
	if entries[3].ResourceType != "transaction" {
		t.Fatalf("unexpected resource type: %s", entries[3].ResourceType)
	}
}

func TestAuthTokenRefresh(t *testing.T) {
	user := &auth.User{ID: "user-4", OrganizationID: "org-4", Email: "user4@example.com", Status: "active"}
	authSvc, store := newStaticAuthService(t, user, "Refresh123!", nil, nil)

	api := newTestAPIWithAuth(t, authSvc)
	original := obtainToken(t, api, user.Email, "Refresh123!")

	resp := api.post("/v1/auth/token", map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": original.RefreshToken,
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	newTokens := decode[tokenResponse](t, resp.Body)
	_ = resp.Body.Close()

	if newTokens.AccessToken == "" {
		t.Fatalf("missing access token")
	}
	if newTokens.AccessToken == original.AccessToken {
		t.Fatalf("access token was not rotated")
	}
	if len(store.audit.entries) < 2 || store.audit.entries[1].Action != "auth.token.refresh" {
		t.Fatalf("expected refresh audit entry")
	}
}

type staticStore struct {
	users         map[string]*auth.User
	assignments   map[string][]auth.Assignment
	rolePerms     map[string][]auth.Permission
	refreshTokens map[string]*auth.RefreshToken
	audit         *staticAuditStore
}

func newStaticAuthService(t *testing.T, user *auth.User, password string, assignments []auth.Assignment, rolePermKeys map[string][]string) (*auth.Service, *staticStore) {
	t.Helper()
	store := &staticStore{
		users:         map[string]*auth.User{},
		assignments:   map[string][]auth.Assignment{},
		rolePerms:     map[string][]auth.Permission{},
		refreshTokens: map[string]*auth.RefreshToken{},
		audit:         &staticAuditStore{},
	}
	if user != nil {
		u := *user
		hash, err := auth.HashPassword(password)
		if err != nil {
			t.Fatalf("hash password: %v", err)
		}
		u.PasswordHash = hash
		if u.Status == "" {
			u.Status = "active"
		}
		store.users[u.ID] = &u
	}
	if len(assignments) > 0 {
		store.assignments[assignments[0].UserID] = append([]auth.Assignment(nil), assignments...)
	}
	for roleID, keys := range rolePermKeys {
		var perms []auth.Permission
		for _, key := range keys {
			perms = append(perms, auth.Permission{ID: key, Key: key})
		}
		store.rolePerms[roleID] = perms
	}
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	svc, err := auth.NewService(store,
		auth.WithRS256Keys(string(privPEM), string(pubPEM)),
		auth.WithIssuer("test-suite"),
		auth.WithClock(func() time.Time { return fixedNow }),
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, store
}

func (s *staticStore) Organizations(context.Context) auth.OrganizationStore {
	return staticOrgStore{}
}

func (s *staticStore) Users(context.Context) auth.UserStore {
	return &staticUserStore{store: s}
}

func (s *staticStore) Roles(context.Context) auth.RoleStore {
	return &staticRoleStore{store: s}
}

func (s *staticStore) Permissions(context.Context) auth.PermissionStore {
	return &staticPermissionStore{store: s}
}

func (s *staticStore) Audit(context.Context) auth.AuditStore {
	return s.audit
}

func (s *staticStore) RefreshTokens(context.Context) auth.RefreshTokenStore {
	return &staticRefreshTokenStore{store: s}
}

type staticOrgStore struct{}

func (staticOrgStore) Create(context.Context, *auth.Organization) error { return nil }
func (staticOrgStore) Find(context.Context, string) (*auth.Organization, error) {
	return nil, auth.ErrNotFound
}
func (staticOrgStore) List(context.Context) ([]*auth.Organization, error) { return nil, nil }

type staticUserStore struct {
	store *staticStore
}

func (s *staticUserStore) Create(ctx context.Context, user *auth.User) error {
	clone := *user
	s.store.users[clone.ID] = &clone
	return nil
}

func (s *staticUserStore) Find(ctx context.Context, id string) (*auth.User, error) {
	if user, ok := s.store.users[id]; ok {
		clone := *user
		return &clone, nil
	}
	return nil, auth.ErrNotFound
}

func (s *staticUserStore) FindByEmail(ctx context.Context, email string) (*auth.User, error) {
	for _, user := range s.store.users {
		if user.Email == email {
			clone := *user
			return &clone, nil
		}
	}
	return nil, auth.ErrNotFound
}

func (s *staticUserStore) ListByOrg(ctx context.Context, orgID string) ([]*auth.User, error) {
	var res []*auth.User
	for _, user := range s.store.users {
		if user.OrganizationID == orgID {
			clone := *user
			res = append(res, &clone)
		}
	}
	return res, nil
}

func (s *staticUserStore) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	if user, ok := s.store.users[userID]; ok {
		user.PasswordHash = passwordHash
		return nil
	}
	return auth.ErrNotFound
}

type staticRoleStore struct {
	store *staticStore
}

func (s *staticRoleStore) Create(context.Context, *auth.Role) error { return nil }

func (s *staticRoleStore) Find(context.Context, string) (*auth.Role, error) {
	return nil, auth.ErrNotFound
}

func (s *staticRoleStore) ListByOrg(context.Context, string) ([]*auth.Role, error) { return nil, nil }

func (s *staticRoleStore) Assign(ctx context.Context, assignment auth.Assignment) error {
	s.store.assignments[assignment.UserID] = append(s.store.assignments[assignment.UserID], assignment)
	return nil
}

func (s *staticRoleStore) Assignments(ctx context.Context, userID string) ([]auth.Assignment, error) {
	list := s.store.assignments[userID]
	out := make([]auth.Assignment, len(list))
	copy(out, list)
	return out, nil
}

type staticPermissionStore struct {
	store *staticStore
}

func (s *staticPermissionStore) Ensure(context.Context, []auth.Permission) error { return nil }

func (s *staticPermissionStore) List(context.Context) ([]auth.Permission, error) {
	var res []auth.Permission
	seen := make(map[string]struct{})
	for _, perms := range s.store.rolePerms {
		for _, p := range perms {
			if _, ok := seen[p.Key]; ok {
				continue
			}
			seen[p.Key] = struct{}{}
			res = append(res, p)
		}
	}
	return res, nil
}

func (s *staticPermissionStore) SetForRole(ctx context.Context, roleID string, permKeys []string) error {
	var perms []auth.Permission
	for _, key := range permKeys {
		perms = append(perms, auth.Permission{ID: key, Key: key})
	}
	s.store.rolePerms[roleID] = perms
	return nil
}

func (s *staticPermissionStore) PermissionsForRole(ctx context.Context, roleID string) ([]auth.Permission, error) {
	perms := s.store.rolePerms[roleID]
	out := make([]auth.Permission, len(perms))
	copy(out, perms)
	return out, nil
}

type staticRefreshTokenStore struct {
	store *staticStore
}

func (s *staticRefreshTokenStore) Create(ctx context.Context, tok *auth.RefreshToken) error {
	clone := *tok
	s.store.refreshTokens[clone.ID] = &clone
	return nil
}

func (s *staticRefreshTokenStore) Find(ctx context.Context, id string) (*auth.RefreshToken, error) {
	if tok, ok := s.store.refreshTokens[id]; ok {
		clone := *tok
		return &clone, nil
	}
	return nil, auth.ErrNotFound
}

func (s *staticRefreshTokenStore) MarkRevoked(ctx context.Context, id string) error {
	if tok, ok := s.store.refreshTokens[id]; ok {
		tok.Revoked = true
		return nil
	}
	return auth.ErrNotFound
}

func (s *staticRefreshTokenStore) MarkRevokedByUser(ctx context.Context, userID string) error {
	for _, tok := range s.store.refreshTokens {
		if tok.UserID == userID {
			tok.Revoked = true
		}
	}
	return nil
}

type staticAuditStore struct {
	entries []*auth.AuditEntry
}

func (s *staticAuditStore) Append(_ context.Context, entry *auth.AuditEntry) error {
	if entry == nil {
		return nil
	}
	clone := *entry
	s.entries = append(s.entries, &clone)
	return nil
}
