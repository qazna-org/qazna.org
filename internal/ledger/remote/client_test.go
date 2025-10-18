package remote

import (
	"errors"
	"testing"

	"qazna.org/internal/ledger"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapLedgerError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "not found",
			err:  status.Error(codes.NotFound, "not found"),
			want: ledger.ErrNotFound,
		},
		{
			name: "invalid currency",
			err:  status.Error(codes.InvalidArgument, "invalid currency"),
			want: ledger.ErrInvalidCurrency,
		},
		{
			name: "invalid amount canonical",
			err:  status.Error(codes.InvalidArgument, "invalid amount (must be > 0)"),
			want: ledger.ErrInvalidAmount,
		},
		{
			name: "insufficient funds",
			err:  status.Error(codes.FailedPrecondition, "insufficient funds"),
			want: ledger.ErrInsufficientFunds,
		},
		{
			name: "pass through",
			err:  status.Error(codes.Internal, "internal"),
			want: status.Error(codes.Internal, "internal"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := mapLedgerError(tc.err)
			if !errors.Is(got, tc.want) {
				t.Fatalf("mapLedgerError() = %v, want %v", got, tc.want)
			}
		})
	}
}
