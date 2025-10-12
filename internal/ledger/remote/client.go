package remote

import (
	context "context"
	"time"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func (c *Client) CreateAccount(ctx context.Context, currency string, initial int64) (*v1.Account, error) {
	return c.svc.CreateAccount(ctx, &v1.CreateAccountRequest{Currency: currency, InitialAmount: initial})
}

func (c *Client) GetAccount(ctx context.Context, id string) (*v1.Account, error) {
	return c.svc.GetAccount(ctx, &v1.GetAccountRequest{Id: id})
}

func (c *Client) GetBalance(ctx context.Context, id, currency string) (*v1.Balance, error) {
	return c.svc.GetBalance(ctx, &v1.GetBalanceRequest{Id: id, Currency: currency})
}

func (c *Client) Transfer(ctx context.Context, req *v1.TransferRequest) (*v1.TransferResponse, error) {
	return c.svc.Transfer(ctx, req)
}

func (c *Client) ListTransactions(ctx context.Context, after uint64, limit uint32) (*v1.ListTransactionsResponse, error) {
	return c.svc.ListTransactions(ctx, &v1.ListTransactionsRequest{
		AfterSequence: after,
		Limit:         limit,
	})
}

// WithTimeout returns a context with default timeout useful for CLI tools.
func WithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 10 * time.Second
	}
	return context.WithTimeout(parent, d)
}
