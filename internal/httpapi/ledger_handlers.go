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
		methodNotAllowed(w, r, http.MethodPost)
	}
}

func (a *API) handleAccountResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/accounts/")
	if path == "" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}

	if strings.HasSuffix(path, "/balance") {
		id := strings.TrimSuffix(path, "/balance")
		id = strings.TrimSuffix(id, "/")
		if id == "" {
			writeError(w, r, http.StatusNotFound, "account not found")
			return
		}
		a.getBalance(w, r, id)
		return
	}

	if strings.Contains(path, "/") {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.getAccount(w, r, path)
	default:
		methodNotAllowed(w, r, http.MethodGet)
	}
}

func (a *API) handleTransfers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.transfer(w, r)
	default:
		methodNotAllowed(w, r, http.MethodPost)
	}
}

func (a *API) handleTransactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.listTransactions(w, r)
	default:
		methodNotAllowed(w, r, http.MethodGet)
	}
}

func (a *API) createAccount(w http.ResponseWriter, r *http.Request) {
	var req createAccountRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if strings.TrimSpace(req.Currency) == "" {
		writeError(w, r, http.StatusBadRequest, "currency is required")
		return
	}
	if len(req.Currency) > 8 {
		writeError(w, r, http.StatusBadRequest, "currency code too long")
		return
	}
	if req.InitialAmount < 0 {
		writeError(w, r, http.StatusBadRequest, "initial_amount must be >= 0")
		return
	}

	acc, err := a.ledger.CreateAccount(r.Context(), ledger.Money{
		Currency: strings.ToUpper(req.Currency),
		Amount:   req.InitialAmount,
	})
	if err != nil {
		handleLedgerError(w, r, err)
		return
	}

	a.audit(r.Context(), "ledger.account.create", "account", acc.ID, map[string]string{
		"currency":       strings.ToUpper(req.Currency),
		"initial_amount": strconv.FormatInt(req.InitialAmount, 10),
	})

	w.Header().Set("Location", "/v1/accounts/"+acc.ID)
	writeJSON(w, http.StatusCreated, acc)
}

func (a *API) getAccount(w http.ResponseWriter, r *http.Request, id string) {
	acc, err := a.ledger.GetAccount(r.Context(), id)
	if err != nil {
		handleLedgerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (a *API) getBalance(w http.ResponseWriter, r *http.Request, id string) {
	currency := r.URL.Query().Get("currency")
	if strings.TrimSpace(currency) == "" {
		writeError(w, r, http.StatusBadRequest, "currency query parameter is required")
		return
	}
	mon, err := a.ledger.GetBalance(r.Context(), id, strings.ToUpper(currency))
	if err != nil {
		handleLedgerError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, mon)
}

func (a *API) transfer(w http.ResponseWriter, r *http.Request) {
	var req transferRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if req.IdempotencyKey != "" {
		bodyKey := strings.TrimSpace(req.IdempotencyKey)
		if idem == "" {
			idem = bodyKey
		} else if idem != bodyKey {
			writeError(w, r, http.StatusBadRequest, "Idempotency-Key header and body value must match")
			return
		}
	}
	if len(idem) > 128 {
		writeError(w, r, http.StatusBadRequest, "Idempotency-Key too long")
		return
	}

	fromID := strings.TrimSpace(req.FromID)
	toID := strings.TrimSpace(req.ToID)
	if fromID == "" || toID == "" {
		writeError(w, r, http.StatusBadRequest, "from_id and to_id are required")
		return
	}
	if len(fromID) > 64 || len(toID) > 64 {
		writeError(w, r, http.StatusBadRequest, "account identifiers must be <=64 characters")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		writeError(w, r, http.StatusBadRequest, "currency is required")
		return
	}
	if len(currency) > 8 {
		writeError(w, r, http.StatusBadRequest, "currency code too long")
		return
	}
	if req.Amount <= 0 {
		writeError(w, r, http.StatusBadRequest, "amount must be > 0")
		return
	}

	start := time.Now().UTC()
	tx, err := a.ledger.Transfer(
		r.Context(),
		fromID,
		toID,
		ledger.Money{
			Currency: currency,
			Amount:   req.Amount,
		},
		idem,
	)
	if err != nil {
		handleLedgerError(w, r, err)
		return
	}
	replayed := false
	if idem != "" && !tx.CreatedAt.After(start) {
		replayed = true
	}

	if idem != "" {
		w.Header().Set("Idempotency-Key", idem)
	}

	if a.stream != nil {
		event := stream.TransferEvent{
			From:      a.resolveLocation(fromID),
			To:        a.resolveLocation(toID),
			Amount:    tx.Amount,
			Currency:  tx.Currency,
			Timestamp: time.Now().UTC(),
		}
		a.stream.Publish(event)
	}

	meta := map[string]string{
		"from_account": fromID,
		"to_account":   toID,
		"currency":     currency,
		"amount":       strconv.FormatInt(req.Amount, 10),
	}
	if idem != "" {
		meta["idempotency_key"] = idem
	}
	event := "ledger.transfer.execute"
	if replayed {
		event = "ledger.transfer.idempotent_replay"
	}
	a.audit(r.Context(), event, "transaction", tx.ID, meta)

	writeJSON(w, http.StatusCreated, tx)
}

func (a *API) listTransactions(w http.ResponseWriter, r *http.Request) {
	limit, err := parsePositiveInt(r.URL.Query().Get("limit"), 100, 1, 1000)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	afterParam := strings.TrimSpace(r.URL.Query().Get("after"))
	var after uint64
	if afterParam != "" {
		v, err := strconv.ParseUint(afterParam, 10, 64)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "after must be a non-negative integer")
			return
		}
		after = v
	}

	items, next, err := a.ledger.ListTransactions(r.Context(), limit, after)
	if err != nil {
		handleLedgerError(w, r, err)
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

func handleLedgerError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ledger.ErrInvalidAmount), errors.Is(err, ledger.ErrInvalidCurrency):
		writeError(w, r, http.StatusBadRequest, err.Error())
	case errors.Is(err, ledger.ErrInsufficientFunds):
		writeError(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, ledger.ErrNotFound):
		writeError(w, r, http.StatusNotFound, err.Error())
	default:
		writeError(w, r, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, r *http.Request, code int, msg string) {
	payload := map[string]any{
		"error": msg,
	}
	if rid := RequestIDFromContext(r.Context()); rid != "" {
		payload["request_id"] = rid
	}
	writeJSON(w, code, payload)
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	writeError(w, r, http.StatusMethodNotAllowed, "method not allowed")
}
