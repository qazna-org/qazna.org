package ledger

import (
	"errors"
	"time"

	"qazna.org/internal/ids"
)

// Money is represented in minor units (e.g., cents). No floats.
type Money struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}

func (m Money) IsPositive() bool { return m.Amount > 0 }
func (m Money) IsZero() bool     { return m.Amount == 0 }

// Account is a simple account with per-currency balances.
// For MVP we typically use a single currency (e.g., "QZN").
type Account struct {
	ID        string           `json:"id"`
	CreatedAt time.Time        `json:"created_at"`
	Balances  map[string]int64 `json:"balances"` // currency -> minor units
}

// Transaction is a double-entry transfer result.
type Transaction struct {
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	FromAccountID  string    `json:"from_account_id"`
	ToAccountID    string    `json:"to_account_id"`
	Currency       string    `json:"currency"`
	Amount         int64     `json:"amount"` // minor units
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	Sequence       uint64    `json:"sequence"` // monotonic sequence number
}

var (
	ErrNotFound          = errors.New("not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrInvalidAmount     = errors.New("invalid amount (must be > 0)")
	ErrInvalidCurrency   = errors.New("invalid currency")
)

func newID() string {
	return ids.New()
}
