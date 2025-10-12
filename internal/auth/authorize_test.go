package auth

import "testing"

func TestPrincipalPermissions(t *testing.T) {
	user := &User{ID: "u1", OrganizationID: "org", Email: "user@example.com"}
	assignments := []Assignment{{UserID: "u1", RoleID: "role", OrganizationID: "org"}}
	perms := []Permission{{ID: "p1", Key: "ledger.transfer"}}

	principal := NewPrincipal(user, assignments, perms)

	if !principal.HasPermission("ledger.transfer") {
		t.Fatalf("expected permission")
	}
	if principal.HasPermission("ledger.admin") {
		t.Fatalf("unexpected permission")
	}
}
