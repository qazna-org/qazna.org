package auth

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestGenerateTokenRequiresSecret(t *testing.T) {
	ResetSecretForTests()
	t.Setenv(secretEnvVariable, "")
	if _, err := GenerateToken("user-1", []string{"admin"}, time.Minute); err == nil {
		t.Fatalf("expected error when secret missing")
	}
}

func TestGenerateAndParseToken(t *testing.T) {
	t.Setenv(secretEnvVariable, "unit-test-secret")
	ResetSecretForTests()

	token, err := GenerateToken("user-42", []string{"Admin", "viewer", "admin"}, 2*time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	claims, err := ParseAndValidate(token)
	if err != nil {
		t.Fatalf("ParseAndValidate: %v", err)
	}
	if claims.Subject != "user-42" {
		t.Fatalf("unexpected subject: %s", claims.Subject)
	}
	if !slices.Contains(claims.Roles, "admin") || !slices.Contains(claims.Roles, "viewer") {
		t.Fatalf("roles were not preserved: %v", claims.Roles)
	}
	if claims.Issuer != issuer {
		t.Fatalf("unexpected issuer: %s", claims.Issuer)
	}
	if claims.ExpiresAt.Time.Before(time.Now().UTC()) {
		t.Fatalf("token already expired")
	}
}

func TestParseInvalidToken(t *testing.T) {
	t.Setenv(secretEnvVariable, "unit-test-secret")
	ResetSecretForTests()

	if _, err := ParseAndValidate("not-a-token"); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUser(ctx, "user-7", []string{"Admin", "Admin", "viewer"})
	id, ok := UserIDFromContext(ctx)
	if !ok || id != "user-7" {
		t.Fatalf("unexpected user id: %s, ok=%v", id, ok)
	}
	roles := RolesFromContext(ctx)
	if len(roles) != 2 {
		t.Fatalf("expected deduplicated roles, got %v", roles)
	}
	if !HasRole(ctx, "viewer") || !HasRole(ctx, "admin") {
		t.Fatalf("HasRole missing expected roles: %v", roles)
	}
	if HasRole(ctx, "operator") {
		t.Fatalf("unexpected role found")
	}
}
