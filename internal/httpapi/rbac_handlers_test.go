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
	listOrgFn         func(context.Context) ([]auth.Organization, error)
	getOrgFn          func(context.Context, string) (auth.Organization, error)
	updateOrgFn       func(context.Context, string, auth.OrganizationUpdate) (auth.Organization, error)
	deleteOrgFn       func(context.Context, string) error
	createUserFn      func(context.Context, string, string, string, string) (auth.User, error)
	listUsersFn       func(context.Context, string) ([]auth.User, error)
	getUserFn         func(context.Context, string, string) (auth.User, error)
	updateUserFn      func(context.Context, string, auth.UserUpdate) (auth.User, error)
	deleteUserFn      func(context.Context, string) error
	createRoleFn      func(context.Context, string, string, string) (auth.Role, error)
	listRolesFn       func(context.Context, string) ([]auth.Role, error)
	getRoleFn         func(context.Context, string) (auth.Role, error)
	updateRoleFn      func(context.Context, string, auth.RoleUpdate) (auth.Role, error)
	deleteRoleFn      func(context.Context, string) error
	setRolePermsFn    func(context.Context, string, []string) error
	assignRoleFn      func(context.Context, string, string) (auth.UserRoleAssignment, error)
	removeAssignFn    func(context.Context, string, string) error
	listAssignmentsFn func(context.Context, string) ([]auth.UserRoleAssignment, error)
	userPermissionsFn func(context.Context, string) ([]string, error)
}

func (s *stubRBACStore) CreateOrganization(ctx context.Context, name string, metadata map[string]any) (auth.Organization, error) {
	if s.createOrgFn != nil {
		return s.createOrgFn(ctx, name, metadata)
	}
	return auth.Organization{}, nil
}

func (s *stubRBACStore) ListOrganizations(ctx context.Context) ([]auth.Organization, error) {
	if s.listOrgFn != nil {
		return s.listOrgFn(ctx)
	}
	return nil, nil
}

func (s *stubRBACStore) GetOrganization(ctx context.Context, id string) (auth.Organization, error) {
	if s.getOrgFn != nil {
		return s.getOrgFn(ctx, id)
	}
	return auth.Organization{}, auth.ErrNotFound
}

func (s *stubRBACStore) UpdateOrganization(ctx context.Context, id string, upd auth.OrganizationUpdate) (auth.Organization, error) {
	if s.updateOrgFn != nil {
		return s.updateOrgFn(ctx, id, upd)
	}
	return auth.Organization{}, nil
}

func (s *stubRBACStore) DeleteOrganization(ctx context.Context, id string) error {
	if s.deleteOrgFn != nil {
		return s.deleteOrgFn(ctx, id)
	}
	return nil
}

func (s *stubRBACStore) CreateUser(ctx context.Context, organizationID, email, passwordHash, status string) (auth.User, error) {
	if s.createUserFn != nil {
		return s.createUserFn(ctx, organizationID, email, passwordHash, status)
	}
	return auth.User{}, nil
}

func (s *stubRBACStore) ListUsers(ctx context.Context, organizationID string) ([]auth.User, error) {
	if s.listUsersFn != nil {
		return s.listUsersFn(ctx, organizationID)
	}
	return nil, nil
}

func (s *stubRBACStore) GetUser(ctx context.Context, organizationID, userID string) (auth.User, error) {
	if s.getUserFn != nil {
		return s.getUserFn(ctx, organizationID, userID)
	}
	return auth.User{}, auth.ErrNotFound
}

func (s *stubRBACStore) UpdateUser(ctx context.Context, userID string, upd auth.UserUpdate) (auth.User, error) {
	if s.updateUserFn != nil {
		return s.updateUserFn(ctx, userID, upd)
	}
	return auth.User{}, nil
}

func (s *stubRBACStore) DeleteUser(ctx context.Context, userID string) error {
	if s.deleteUserFn != nil {
		return s.deleteUserFn(ctx, userID)
	}
	return nil
}

func (s *stubRBACStore) CreateRole(ctx context.Context, organizationID, name, description string) (auth.Role, error) {
	if s.createRoleFn != nil {
		return s.createRoleFn(ctx, organizationID, name, description)
	}
	return auth.Role{}, nil
}

func (s *stubRBACStore) ListRoles(ctx context.Context, organizationID string) ([]auth.Role, error) {
	if s.listRolesFn != nil {
		return s.listRolesFn(ctx, organizationID)
	}
	return nil, nil
}

func (s *stubRBACStore) GetRole(ctx context.Context, roleID string) (auth.Role, error) {
	if s.getRoleFn != nil {
		return s.getRoleFn(ctx, roleID)
	}
	return auth.Role{}, auth.ErrNotFound
}

func (s *stubRBACStore) UpdateRole(ctx context.Context, roleID string, upd auth.RoleUpdate) (auth.Role, error) {
	if s.updateRoleFn != nil {
		return s.updateRoleFn(ctx, roleID, upd)
	}
	return auth.Role{}, nil
}

func (s *stubRBACStore) DeleteRole(ctx context.Context, roleID string) error {
	if s.deleteRoleFn != nil {
		return s.deleteRoleFn(ctx, roleID)
	}
	return nil
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

func (s *stubRBACStore) RemoveRoleAssignment(ctx context.Context, userID, roleID string) error {
	if s.removeAssignFn != nil {
		return s.removeAssignFn(ctx, userID, roleID)
	}
	return nil
}

func (s *stubRBACStore) ListRoleAssignments(ctx context.Context, userID string) ([]auth.UserRoleAssignment, error) {
	if s.listAssignmentsFn != nil {
		return s.listAssignmentsFn(ctx, userID)
	}
	return nil, nil
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
