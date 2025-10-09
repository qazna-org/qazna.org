package ledger

import (
	"context"
	"sync"
	"time"
)

// Service defines ledger operations.
type Service interface {
	CreateAccount(ctx context.Context, initial Money) (Account, error)
	GetAccount(ctx context.Context, id string) (Account, error)
	GetBalance(ctx context.Context, id, currency string) (Money, error)
	Transfer(ctx context.Context, fromID, toID string, amt Money, idemKey string) (Transaction, error)
	ListTransactions(ctx context.Context, limit int, afterSeq uint64) ([]Transaction, uint64, error)
}

// InMemory implements Service with in-process concurrency safety.
// NOTE: Replace with durable storage later (FoundationDB/Postgres).
type InMemory struct {
	mu    sync.RWMutex
	accts map[string]*Account
	seq   uint64
	txs   []Transaction
	idem  map[string]Transaction // idemKey -> tx
}

// NewInMemory creates a fresh ledger.
func NewInMemory() *InMemory {
	return &InMemory{
		accts: make(map[string]*Account),
		idem:  make(map[string]Transaction),
	}
}

func (s *InMemory) CreateAccount(ctx context.Context, initial Money) (Account, error) {
	if initial.Currency == "" {
		return Account{}, ErrInvalidCurrency
	}
	if initial.Amount < 0 {
		return Account{}, ErrInvalidAmount
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid16()
	acc := &Account{
		ID:        id,
		CreatedAt: time.Now().UTC(),
		Balances:  map[string]int64{initial.Currency: initial.Amount},
	}
	s.accts[id] = acc
	return *acc, nil
}

func (s *InMemory) GetAccount(ctx context.Context, id string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.accts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	// return copy
	out := *acc
	out.Balances = map[string]int64{}
	for k, v := range acc.Balances {
		out.Balances[k] = v
	}
	return out, nil
}

func (s *InMemory) GetBalance(ctx context.Context, id, currency string) (Money, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acc, ok := s.accts[id]
	if !ok {
		return Money{}, ErrNotFound
	}
	return Money{Currency: currency, Amount: acc.Balances[currency]}, nil
}

func (s *InMemory) Transfer(ctx context.Context, fromID, toID string, amt Money, idemKey string) (Transaction, error) {
	if !amt.IsPositive() {
		return Transaction{}, ErrInvalidAmount
	}
	if amt.Currency == "" {
		return Transaction{}, ErrInvalidCurrency
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency
	if idemKey != "" {
		if tx, ok := s.idem[idemKey]; ok {
			return tx, nil
		}
	}

	from, ok := s.accts[fromID]
	if !ok {
		return Transaction{}, ErrNotFound
	}
	to, ok := s.accts[toID]
	if !ok {
		return Transaction{}, ErrNotFound
	}

	// Double-entry invariant: total debits == total credits (same currency).
	// Enforce sufficient funds.
	if from.Balances[amt.Currency] < amt.Amount {
		return Transaction{}, ErrInsufficientFunds
	}

	// Apply mutation
	from.Balances[amt.Currency] -= amt.Amount
	to.Balances[amt.Currency] += amt.Amount

	s.seq++
	tx := Transaction{
		ID:             uuid16(),
		CreatedAt:      time.Now().UTC(),
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Currency:       amt.Currency,
		Amount:         amt.Amount,
		IdempotencyKey: idemKey,
		Sequence:       s.seq,
	}
	s.txs = append(s.txs, tx)
	if idemKey != "" {
		s.idem[idemKey] = tx
	}
	return tx, nil
}

func (s *InMemory) ListTransactions(ctx context.Context, limit int, afterSeq uint64) ([]Transaction, uint64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res []Transaction
	var last uint64
	for _, tx := range s.txs {
		if tx.Sequence <= afterSeq {
			continue
		}
		res = append(res, tx)
		last = tx.Sequence
		if len(res) >= limit {
			break
		}
	}
	return res, last, nil
}
