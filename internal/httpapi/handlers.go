package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"qazna.org/internal/ledger"
)

type API struct {
	Ledger ledger.Service
}

func New(svc ledger.Service) *API { return &API{Ledger: svc} }

// type API struct {
// 	Ledger *ledger.InMemory
// }

// func New(l *ledger.InMemory) *API { return &API{Ledger: l} }

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// POST /v1/accounts
func (a *API) CreateAccount(w http.ResponseWriter, r *http.Request) {
	type req struct {
		Currency      string `json:"currency"`
		InitialAmount int64  `json:"initial_amount"`
	}
	var in req
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	acc, err := a.Ledger.CreateAccount(r.Context(), ledger.Money{Currency: in.Currency, Amount: in.InitialAmount})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, acc)
}

// GET /v1/accounts/{id}
func (a *API) GetAccount(w http.ResponseWriter, r *http.Request) {
	id := lastSeg(r.URL.Path)
	acc, err := a.Ledger.GetAccount(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

// GET /v1/accounts/{id}/balance?currency=QZN
func (a *API) GetBalance(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		writeErr(w, http.StatusBadRequest, "bad path")
		return
	}
	id := parts[2]
	curr := r.URL.Query().Get("currency")
	m, err := a.Ledger.GetBalance(r.Context(), id, curr)
	if err != nil {
		writeErr(w, http.StatusNotFound, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// POST /v1/transfers  {from_id,to_id,currency,amount,idempotency_key}
// Supports Idempotency-Key header (takes precedence over body).
func (a *API) Transfer(w http.ResponseWriter, r *http.Request) {
	type req struct {
		FromID         string `json:"from_id"`
		ToID           string `json:"to_id"`
		Currency       string `json:"currency"`
		Amount         int64  `json:"amount"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	var in req
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if hk := r.Header.Get("Idempotency-Key"); hk != "" {
		in.IdempotencyKey = hk
	}

	tx, err := a.Ledger.Transfer(
		r.Context(),
		in.FromID, in.ToID,
		ledger.Money{Currency: in.Currency, Amount: in.Amount},
		in.IdempotencyKey,
	)
	if err != nil {
		switch err {
		case ledger.ErrInsufficientFunds:
			writeErr(w, http.StatusConflict, err.Error())
		case ledger.ErrInvalidAmount, ledger.ErrInvalidCurrency:
			writeErr(w, http.StatusBadRequest, err.Error())
		default:
			writeErr(w, http.StatusNotFound, err.Error())
		}
		return
	}

	if in.IdempotencyKey != "" {
		w.Header().Set("Idempotency-Key", in.IdempotencyKey)
	}
	writeJSON(w, http.StatusCreated, tx)
}

// GET /v1/ledger/transactions?limit=&after=
func (a *API) ListTransactions(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	after, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	items, last, _ := a.Ledger.ListTransactions(r.Context(), limit, after)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"next_after": last,
		"as_of":      time.Now().UTC().Format(time.RFC3339),
	})
}

func lastSeg(p string) string {
	s := strings.Trim(p, "/")
	parts := strings.Split(s, "/")
	return parts[len(parts)-1]
}
