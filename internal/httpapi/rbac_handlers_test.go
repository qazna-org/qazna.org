package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"qazna.org/internal/auth"
)

type stubRBACStore struct {
	createOrgFn       func(context.Context, string, map[string]any) (auth.Organization, error)
	createUserFn      func(context.Context, string, string, string, string) (auth.User, error)
	createRoleFn      func(context.Context, string, string, string) (auth.Role, error)
	setRolePermsFn    func(context.Context, string, []string) error
	assignRoleFn      func(context.Context, string, string) (auth.UserRoleAssignment, error)
	userPermissionsFn func(context.Context, string) ([]string, error)
}

func (s *stubRBACStore) CreateOrganization(ctx context.Context, name string, metadata map[string]any) (auth.Organization, error) {
	if s.createOrgFn != nil {
		return s.createOrgFn(ctx, name, metadata)
	}
	return auth.Organization{}, nil
}

func (s *stubRBACStore) CreateUser(ctx context.Context, organizationID, email, passwordHash, status string) (auth.User, error) {
	if s.createUserFn != nil {
		return s.createUserFn(ctx, organizationID, email, passwordHash, status)
	}
	return auth.User{}, nil
}

func (s *stubRBACStore) CreateRole(ctx context.Context, organizationID, name, description string) (auth.Role, error) {
	if s.createRoleFn != nil {
		return s.createRoleFn(ctx, organizationID, name, description)
	}
	return auth.Role{}, nil
}

func (s *stubRBACStore) SetRolePermissions(ctx context.Context, roleID string, permissionKeys []string) error {
	if s.setRolePermsFn != nil {
		return s.setRolePermsFn(ctx, roleID, permissionKeys)
	}
	return nil
}

func (s *stubRBACStore) AssignRoleToUser(ctx context.Context, userID, roleID string) (auth.UserRoleAssignment, error) {
	if s.assignRoleFn != nil {
		return s.assignRoleFn(ctx, userID, roleID)
	}
	return auth.UserRoleAssignment{}, nil
}

func (s *stubRBACStore) UserPermissions(ctx context.Context, userID string) ([]string, error) {
	if s.userPermissionsFn != nil {
		return s.userPermissionsFn(ctx, userID)
	}
	return nil, nil
}

func TestRBACCreateOrganizationRequiresPermission(t *testing.T) {
	store := &stubRBACStore{
		userPermissionsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
	}
	api := newTestAPI(t, store)
	token := api.obtainToken("rbac-user", []string{"admin"})

	resp := api.post("/v1/organizations", map[string]any{"name": "Demo Org"}, map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestRBACCreateOrganizationSuccess(t *testing.T) {
	var capturedName string
	store := &stubRBACStore{
		userPermissionsFn: func(_ context.Context, userID string) ([]string, error) {
			if userID != "rbac-admin" {
				t.Fatalf("unexpected user id %s", userID)
			}
			return []string{auth.PermissionManageOrganizations}, nil
		},
		createOrgFn: func(_ context.Context, name string, metadata map[string]any) (auth.Organization, error) {
			capturedName = name
			return auth.Organization{
				ID:        "org-123",
				Name:      name,
				Metadata:  metadata,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}, nil
		},
	}

	api := newTestAPI(t, store)
	token := api.obtainToken("rbac-admin", []string{"admin"})

	body := map[string]any{
		"name":     "  Strategic Ops  ",
		"metadata": map[string]any{"region": "EU"},
	}
	resp := api.post("/v1/organizations", body, map[string]string{"Authorization": "Bearer " + token})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var payload auth.Organization
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	_ = resp.Body.Close()

	if capturedName != "Strategic Ops" {
		t.Fatalf("expected trimmed name, got %q", capturedName)
	}
	if payload.ID != "org-123" {
		t.Fatalf("unexpected organization id: %s", payload.ID)
	}
	if payload.Metadata["region"] != "EU" {
		t.Fatalf("metadata not forwarded: %v", payload.Metadata)
	}
}

func TestRBACAssignRoleRequiresPayload(t *testing.T) {
	store := &stubRBACStore{
		userPermissionsFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{auth.PermissionManageUsers}, nil
		},
	}
	api := newTestAPI(t, store)
	token := api.obtainToken("rbac-admin", []string{"admin"})

	resp := api.post("/v1/users/user-42/assignments", map[string]any{"role_id": ""}, map[string]string{"Authorization": "Bearer " + token})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
