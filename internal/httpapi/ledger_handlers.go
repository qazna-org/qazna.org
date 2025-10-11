package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"qazna.org/internal/ledger"
	"qazna.org/internal/stream"
)

type createAccountRequest struct {
	Currency      string `json:"currency"`
	InitialAmount int64  `json:"initial_amount"`
}

type transferRequest struct {
	FromID         string `json:"from_id"`
	ToID           string `json:"to_id"`
	Currency       string `json:"currency"`
	Amount         int64  `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type listTransactionsResponse struct {
	Items     []ledger.Transaction `json:"items"`
	NextAfter uint64               `json:"next_after"`
	AsOf      time.Time            `json:"as_of"`
}

func (a *API) handleAccountsCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.createAccount(w, r)
	default:
		methodNotAllowed(w, http.MethodPost)
	}
}

func (a *API) handleAccountResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/accounts/")
	if path == "" {
		respondError(w, http.StatusNotFound, "resource not found")
		return
	}

	if strings.HasSuffix(path, "/balance") {
		id := strings.TrimSuffix(path, "/balance")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			respondError(w, http.StatusNotFound, "account not found")
			return
		}
		a.getBalance(w, r, id)
		return
	}

	if strings.Contains(path, "/") {
		respondError(w, http.StatusNotFound, "resource not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.getAccount(w, r, path)
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (a *API) handleTransfers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.transfer(w, r)
	default:
		methodNotAllowed(w, http.MethodPost)
	}
}

func (a *API) handleTransactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listTransactions(w, r)
	default:
		methodNotAllowed(w, http.MethodGet)
	}
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Currency) == "" {
		respondError(w, http.StatusBadRequest, "currency is required")
		return
	}
	if req.InitialAmount < 0 {
		respondError(w, http.StatusBadRequest, "initial_amount must be >= 0")
		return
	}

	acc, err := a.ledger.CreateAccount(r.Context(), ledger.Money{
		Currency: strings.ToUpper(req.Currency),
		Amount:   req.InitialAmount,
	})
	if err != nil {
		handleLedgerError(w, err)
		return
	}

	w.Header().Set("Location", "/v1/accounts/"+acc.ID)
	writeJSON(w, http.StatusCreated, acc)
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request, id string) {
	acc, err := a.ledger.GetAccount(r.Context(), id)
	if err != nil {
		handleLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (a *API) getBalance(w http.ResponseWriter, r *http.Request, id string) {
	currency := r.URL.Query().Get("currency")
	if strings.TrimSpace(currency) == "" {
		respondError(w, http.StatusBadRequest, "currency query parameter is required")
		return
	}
	mon, err := a.ledger.GetBalance(r.Context(), id, strings.ToUpper(currency))
	if err != nil {
		handleLedgerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mon)
}

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := decodeJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if req.IdempotencyKey != "" {
		bodyKey := strings.TrimSpace(req.IdempotencyKey)
		if idem == "" {
			idem = bodyKey
		} else if idem != bodyKey {
			respondError(w, http.StatusBadRequest, "Idempotency-Key header and body value must match")
			return
		}
	}

	if strings.TrimSpace(req.FromID) == "" || strings.TrimSpace(req.ToID) == "" {
		respondError(w, http.StatusBadRequest, "from_id and to_id are required")
		return
	}
	if strings.TrimSpace(req.Currency) == "" {
		respondError(w, http.StatusBadRequest, "currency is required")
		return
	}
	if req.Amount <= 0 {
		respondError(w, http.StatusBadRequest, "amount must be > 0")
		return
	}

	tx, err := a.ledger.Transfer(
		r.Context(),
		strings.TrimSpace(req.FromID),
		strings.TrimSpace(req.ToID),
		ledger.Money{
			Currency: strings.ToUpper(req.Currency),
			Amount:   req.Amount,
		},
		idem,
	)
	if err != nil {
		handleLedgerError(w, err)
		return
	}

	if idem != "" {
		w.Header().Set("Idempotency-Key", idem)
	}

	if a.stream != nil {
		event := stream.TransferEvent{
			From:      a.resolveLocation(req.FromID),
			To:        a.resolveLocation(req.ToID),
			Amount:    tx.Amount,
			Currency:  tx.Currency,
			Timestamp: time.Now().UTC(),
		}
		a.stream.Publish(event)
	}

	writeJSON(w, http.StatusCreated, tx)
}

func (a *API) listTransactions(w http.ResponseWriter, r *http.Request) {
	limit, err := parsePositiveInt(r.URL.Query().Get("limit"), 100, 1, 1000)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	afterParam := strings.TrimSpace(r.URL.Query().Get("after"))
	var after uint64
	if afterParam != "" {
		v, err := strconv.ParseUint(afterParam, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "after must be a non-negative integer")
			return
		}
		after = v
	}

	items, next, err := a.ledger.ListTransactions(r.Context(), limit, after)
	if err != nil {
		handleLedgerError(w, err)
		return
	}

	resp := listTransactionsResponse{
		Items:     items,
		NextAfter: next,
		AsOf:      time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, resp)
}

func parsePositiveInt(raw string, def, min, max int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return def, nil
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if val < min || val > max {
		return 0, errors.New("limit must be between 1 and 1000")
	}
	return val, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	reader := http.MaxBytesReader(w, r.Body, 1<<20)
	defer reader.Close()
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected data after JSON body")
		}
		return err
	}
	return nil
}

func handleLedgerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalidAmount), errors.Is(err, ledger.ErrInvalidCurrency):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ledger.ErrInsufficientFunds):
		respondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ledger.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	default:
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func respondError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{
		"error": msg,
	})
}

func methodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	respondError(w, http.StatusMethodNotAllowed, "method not allowed")
}
