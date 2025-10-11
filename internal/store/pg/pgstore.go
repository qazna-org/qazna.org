package pg

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"qazna.org/internal/ledger"
)

type Store struct {
	db *sql.DB
}

var _ ledger.Service = (*Store)(nil)

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	// Tuned pool defaults; adjust under load tests
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(15 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) CreateAccount(ctx context.Context, initial ledger.Money) (ledger.Account, error) {
	if initial.Currency == "" {
		return ledger.Account{}, ledger.ErrInvalidCurrency
	}
	if initial.Amount < 0 {
		return ledger.Account{}, ledger.ErrInvalidAmount
	}
	id := uuid16()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ledger.Account{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `insert into accounts(id, created_at) values($1, now())`, id); err != nil {
		return ledger.Account{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into balances(account_id, currency, amount)
		values ($1,$2,$3)
		on conflict (account_id, currency) do update
		set amount = balances.amount + excluded.amount
	`, id, initial.Currency, initial.Amount); err != nil {
		return ledger.Account{}, err
	}
	if err := tx.Commit(); err != nil {
		return ledger.Account{}, err
	}

	return ledger.Account{
		ID:        id,
		CreatedAt: time.Now().UTC(),
		Balances:  map[string]int64{initial.Currency: initial.Amount},
	}, nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (ledger.Account, error) {
	var created time.Time
	err := s.db.QueryRowContext(ctx, `select created_at from accounts where id=$1`, id).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.Account{}, ledger.ErrNotFound
	}
	if err != nil {
		return ledger.Account{}, err
	}

	rows, err := s.db.QueryContext(ctx, `select currency, amount from balances where account_id=$1`, id)
	if err != nil {
		return ledger.Account{}, err
	}
	defer rows.Close()

	bals := map[string]int64{}
	for rows.Next() {
		var c string
		var a int64
		if err := rows.Scan(&c, &a); err != nil {
			return ledger.Account{}, err
		}
		bals[c] = a
	}
	return ledger.Account{ID: id, CreatedAt: created, Balances: bals}, nil
}

func (s *Store) GetBalance(ctx context.Context, id, currency string) (ledger.Money, error) {
	var amt int64
	err := s.db.QueryRowContext(ctx, `
		select coalesce(b.amount,0)
		from accounts a
		left join balances b on b.account_id=a.id and b.currency=$2
		where a.id=$1
	`, id, currency).Scan(&amt)
	if errors.Is(err, sql.ErrNoRows) {
		return ledger.Money{}, ledger.ErrNotFound
	}
	if err != nil {
		return ledger.Money{}, err
	}
	return ledger.Money{Currency: currency, Amount: amt}, nil
}

func (s *Store) Transfer(ctx context.Context, fromID, toID string, amt ledger.Money, idemKey string) (ledger.Transaction, error) {
	if !amt.IsPositive() {
		return ledger.Transaction{}, ledger.ErrInvalidAmount
	}
	if amt.Currency == "" {
		return ledger.Transaction{}, ledger.ErrInvalidCurrency
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return ledger.Transaction{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Idempotency: return existing tx if idemKey already recorded
	if idemKey != "" {
		var t ledger.Transaction
		var created time.Time
		var idem sql.NullString
		err := tx.QueryRowContext(ctx, `
			select id, created_at, from_account_id, to_account_id, currency, amount, sequence, idempotency_key
			from transactions where idempotency_key=$1
		`, idemKey).Scan(&t.ID, &created, &t.FromAccountID, &t.ToAccountID, &t.Currency, &t.Amount, &t.Sequence, &idem)
		if err == nil {
			t.CreatedAt = created
			if idem.Valid {
				t.IdempotencyKey = idem.String
			}
			return t, nil
		} else if err != sql.ErrNoRows {
			return ledger.Transaction{}, err
		}
	}

	// Lock accounts to ensure existence and stable ordering to avoid deadlocks
	for _, acc := range sorted(fromID, toID) {
		var dummy int
		if err := tx.QueryRowContext(ctx, `select 1 from accounts where id=$1 for update`, acc).Scan(&dummy); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ledger.Transaction{}, ledger.ErrNotFound
			}
			return ledger.Transaction{}, err
		}
	}

	// Ensure balance rows exist
	if _, err := tx.ExecContext(ctx, `
		insert into balances(account_id, currency, amount)
		values ($1,$2,0) on conflict do nothing
	`, fromID, amt.Currency); err != nil {
		return ledger.Transaction{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		insert into balances(account_id, currency, amount)
		values ($1,$2,0) on conflict do nothing
	`, toID, amt.Currency); err != nil {
		return ledger.Transaction{}, err
	}

	// Check sufficient funds (lock row)
	var fromBal int64
	if err := tx.QueryRowContext(ctx, `
		select amount from balances where account_id=$1 and currency=$2 for update
	`, fromID, amt.Currency).Scan(&fromBal); err != nil {
		return ledger.Transaction{}, ledger.ErrNotFound
	}
	if fromBal < amt.Amount {
		return ledger.Transaction{}, ledger.ErrInsufficientFunds
	}

	// Apply delta
	if _, err := tx.ExecContext(ctx, `
		update balances set amount = amount - $3
		where account_id=$1 and currency=$2
	`, fromID, amt.Currency, amt.Amount); err != nil {
		return ledger.Transaction{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		update balances set amount = amount + $3
		where account_id=$1 and currency=$2
	`, toID, amt.Currency, amt.Amount); err != nil {
		return ledger.Transaction{}, err
	}

	// Record transaction
	tid := uuid16()
	var seq uint64
	if err := tx.QueryRowContext(ctx, `
		insert into transactions(id, from_account_id, to_account_id, currency, amount, idempotency_key)
		values ($1,$2,$3,$4,$5,nullif($6,'')) returning sequence
	`, tid, fromID, toID, amt.Currency, amt.Amount, idemKey).Scan(&seq); err != nil {
		return ledger.Transaction{}, err
	}

	if err := tx.Commit(); err != nil {
		return ledger.Transaction{}, err
	}

	return ledger.Transaction{
		ID:             tid,
		CreatedAt:      time.Now().UTC(),
		FromAccountID:  fromID,
		ToAccountID:    toID,
		Currency:       amt.Currency,
		Amount:         amt.Amount,
		IdempotencyKey: idemKey,
		Sequence:       seq,
	}, nil
}

func (s *Store) ListTransactions(ctx context.Context, limit int, afterSeq uint64) ([]ledger.Transaction, uint64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, created_at, from_account_id, to_account_id, currency, amount, sequence, coalesce(idempotency_key,'')
		from transactions
		where sequence > $1
		order by sequence asc
		limit $2
	`, afterSeq, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var res []ledger.Transaction
	var last uint64
	for rows.Next() {
		var tx ledger.Transaction
		var idem string
		if err := rows.Scan(&tx.ID, &tx.CreatedAt, &tx.FromAccountID, &tx.ToAccountID, &tx.Currency, &tx.Amount, &tx.Sequence, &idem); err != nil {
			return nil, 0, err
		}
		if idem != "" {
			tx.IdempotencyKey = idem
		}
		res = append(res, tx)
		last = tx.Sequence
	}
	return res, last, nil
}

// --- helpers ---
func uuid16() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func sorted(a, b string) []string {
	if a <= b {
		return []string{a, b}
	}
	return []string{b, a}
}
