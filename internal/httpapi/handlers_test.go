package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"qazna.org/internal/auth"
	"qazna.org/internal/ledger"
	"qazna.org/internal/stream"
)

type apiClient struct {
	baseURL string
	client  *http.Client
	t       *testing.T
}

func newTestAPI(t *testing.T) *apiClient {
	t.Helper()

	t.Setenv("QAZNA_AUTH_SECRET", "test-secret")
	auth.ResetSecretForTests()

	api := New(ReadyProbe{}, "test", ledger.NewInMemory(), stream.New(), nil)
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
	api := newTestAPI(t)
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
	api := newTestAPI(t)

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
	api := newTestAPI(t)

	resp := api.post("/v1/auth/token", map[string]any{"user": ""}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
