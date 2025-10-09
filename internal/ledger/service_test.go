package ledger

import (
	"context"
	"sync"
	"testing"
)

func TestTransferSuccessAndBalance(t *testing.T) {
	s := NewInMemory()
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 1000})
	b, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 0})

	_, err := s.Transfer(ctx, a.ID, b.ID, Money{Currency: "QZN", Amount: 600}, "k1")
	if err != nil {
		t.Fatal(err)
	}
	ba, _ := s.GetBalance(ctx, a.ID, "QZN")
	bb, _ := s.GetBalance(ctx, b.ID, "QZN")

	if ba.Amount != 400 || bb.Amount != 600 {
		t.Fatalf("unexpected balances: a=%d b=%d", ba.Amount, bb.Amount)
	}
}

func TestInsufficientFunds(t *testing.T) {
	s := NewInMemory()
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 100})
	b, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 0})

	if _, err := s.Transfer(ctx, a.ID, b.ID, Money{Currency: "QZN", Amount: 200}, "k2"); err != ErrInsufficientFunds {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestIdempotency(t *testing.T) {
	s := NewInMemory()
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 1000})
	b, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 0})

	tx1, err := s.Transfer(ctx, a.ID, b.ID, Money{Currency: "QZN", Amount: 100}, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	tx2, err := s.Transfer(ctx, a.ID, b.ID, Money{Currency: "QZN", Amount: 100}, "same-key")
	if err != nil {
		t.Fatal(err)
	}
	if tx1.ID != tx2.ID || tx1.Sequence != tx2.Sequence {
		t.Fatalf("idempotency violated: %#v != %#v", tx1, tx2)
	}
}

func TestConcurrentTransfers(t *testing.T) {
	s := NewInMemory()
	ctx := context.Background()
	a, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 10000})
	b, _ := s.CreateAccount(ctx, Money{Currency: "QZN", Amount: 0})

	var wg sync.WaitGroup
	N := 50
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = s.Transfer(ctx, a.ID, b.ID, Money{Currency: "QZN", Amount: 100}, "")
		}(i)
	}
	wg.Wait()

	ba, _ := s.GetBalance(ctx, a.ID, "QZN")
	bb, _ := s.GetBalance(ctx, b.ID, "QZN")
	if ba.Amount+bb.Amount != 10000 {
		t.Fatalf("conservation violated: a+b=%d", ba.Amount+bb.Amount)
	}
}
