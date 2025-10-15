package obs

import "testing"

func TestCanonicalPath(t *testing.T) {
	cases := map[string]string{
		"":                                 "/",
		"/metrics":                         "/metrics",
		"/v1/accounts/abc":                 "/v1/accounts/:id",
		"/v1/accounts/abc/balance":         "/v1/accounts/:id/balance",
		"/v1/accounts/abc/extra":           "/v1/accounts/abc/extra",
		"/v1/ledger/transactions":          "/v1/ledger/transactions",
		"/v1/ledger/transactions?limit=10": "/v1/ledger/transactions",
		"/v1/transfers":                    "/v1/transfers",
	}
	for input, expected := range cases {
		if got := CanonicalPath(input); got != expected {
			t.Fatalf("CanonicalPath(%q)=%q, want %q", input, got, expected)
		}
	}
}
