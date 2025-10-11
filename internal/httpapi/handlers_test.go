package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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
	api := New(ReadyProbe{}, "test", ledger.NewInMemory(), stream.New())
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
