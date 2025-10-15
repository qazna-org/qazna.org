package remote

import (
	"context"
	"strings"
	"time"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"
	"qazna.org/internal/auth"
	"qazna.org/internal/ledger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client wraps the gRPC ledger service.
type Client struct {
	conn *grpc.ClientConn
	svc  v1.LedgerServiceClient
}

// Dial creates a new client with sensible defaults (insecure transport).
func Dial(ctx context.Context, target string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, svc: v1.NewLedgerServiceClient(conn)}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Service adapts the gRPC client to the ledger.Service interface.
type Service struct {
	client *Client
}

func NewService(client *Client) *Service { return &Service{client: client} }

func (s *Service) CreateAccount(ctx context.Context, initial ledger.Money) (ledger.Account, error) {
	ctx = outgoingWithIdentity(ctx)
	resp, err := s.client.svc.CreateAccount(ctx, &v1.CreateAccountRequest{
		Currency:      initial.Currency,
		InitialAmount: initial.Amount,
	})
	if err != nil {
		return ledger.Account{}, err
	}
	return fromProtoAccount(resp), nil
}

func (s *Service) GetAccount(ctx context.Context, id string) (ledger.Account, error) {
	ctx = outgoingWithIdentity(ctx)
	resp, err := s.client.svc.GetAccount(ctx, &v1.GetAccountRequest{Id: id})
	if err != nil {
		return ledger.Account{}, err
	}
	return fromProtoAccount(resp), nil
}

func (s *Service) GetBalance(ctx context.Context, id, currency string) (ledger.Money, error) {
	ctx = outgoingWithIdentity(ctx)
	resp, err := s.client.svc.GetBalance(ctx, &v1.GetBalanceRequest{Id: id, Currency: currency})
	if err != nil {
		return ledger.Money{}, err
	}
	return ledger.Money{Currency: resp.Currency, Amount: resp.Amount}, nil
}

func (s *Service) Transfer(ctx context.Context, fromID, toID string, amt ledger.Money, idemKey string) (ledger.Transaction, error) {
	ctx = outgoingWithIdentity(ctx)
	resp, err := s.client.svc.Transfer(ctx, &v1.TransferRequest{
		FromId:         fromID,
		ToId:           toID,
		Currency:       amt.Currency,
		Amount:         amt.Amount,
		IdempotencyKey: idemKey,
	})
	if err != nil {
		return ledger.Transaction{}, err
	}
	return fromProtoTransaction(resp.Transaction), nil
}

func (s *Service) ListTransactions(ctx context.Context, limit int, afterSeq uint64) ([]ledger.Transaction, uint64, error) {
	if limit <= 0 {
		limit = 100
	}
	ctx = outgoingWithIdentity(ctx)
	resp, err := s.client.svc.ListTransactions(ctx, &v1.ListTransactionsRequest{
		AfterSequence: afterSeq,
		Limit:         uint32(limit),
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]ledger.Transaction, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, fromProtoTransaction(item))
	}
	return items, resp.NextAfter, nil
}

// Helpers -----------------------------------------------------------------

func outgoingWithIdentity(ctx context.Context) context.Context {
	if ctx == nil {
		return ctx
	}
	var pairs []string
	if userID, ok := auth.UserIDFromContext(ctx); ok {
		pairs = append(pairs, "x-qazna-user-id", userID)
	}
	if roles := auth.RolesFromContext(ctx); len(roles) > 0 {
		pairs = append(pairs, "x-qazna-roles", strings.Join(roles, ","))
	}
	if len(pairs) == 0 {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}

func fromProtoAccount(a *v1.Account) ledger.Account {
	balances := make(map[string]int64, len(a.Balances))
	for k, v := range a.Balances {
		balances[k] = v
	}
	var created time.Time
	if ts := a.GetCreatedAt(); ts != nil {
		created = ts.AsTime()
	}
	return ledger.Account{
		ID:        a.Id,
		CreatedAt: created,
		Balances:  balances,
	}
}

func fromProtoTransaction(tx *v1.Transaction) ledger.Transaction {
	var created time.Time
	if ts := tx.GetCreatedAt(); ts != nil {
		created = ts.AsTime()
	}
	return ledger.Transaction{
		ID:             tx.Id,
		CreatedAt:      created,
		FromAccountID:  tx.FromAccountId,
		ToAccountID:    tx.ToAccountId,
		Currency:       tx.Currency,
		Amount:         tx.Amount,
		IdempotencyKey: tx.IdempotencyKey,
		Sequence:       tx.Sequence,
	}
}

// ToProtoTimestamp converts time to protobuf timestamp.
// WithTimeout returns a context with default timeout useful for CLI tools.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithTimeout(parent, d)
}
