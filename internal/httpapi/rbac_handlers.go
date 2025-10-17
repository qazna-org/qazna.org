package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"qazna.org/internal/auth"
)

type createOrganizationRequest struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type createRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

type assignRoleRequest struct {
	RoleID string `json:"role_id"`
}

func (a *API) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !a.ensurePermissions(w, r, auth.PermissionManageOrganizations) {
		return
	}
	var req createOrganizationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	org, err := a.rbac.CreateOrganization(r.Context(), req.Name, req.Metadata)
	if err != nil {
		handleRBACError(w, r, err)
		return
	}
	a.audit(r.Context(), "rbac.organization.create", "organization", org.ID, map[string]string{
		"name": org.Name,
	})
	w.Header().Set("Location", fmt.Sprintf("/v1/organizations/%s", org.ID))
	writeJSON(w, http.StatusCreated, org)
}

func (a *API) handleOrganizationScoped(w http.ResponseWriter, r *http.Request) {
	if a.rbac == nil {
		writeError(w, r, http.StatusServiceUnavailable, "rbac service unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/organizations/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	orgID := parts[0]
	switch parts[1] {
	case "users":
		if len(parts) != 2 {
			writeError(w, r, http.StatusNotFound, "resource not found")
			return
		}
		a.handleOrganizationUsers(w, r, orgID)
	case "roles":
		if len(parts) != 2 {
			writeError(w, r, http.StatusNotFound, "resource not found")
			return
		}
		a.handleOrganizationRoles(w, r, orgID)
	default:
		writeError(w, r, http.StatusNotFound, "resource not found")
	}
}

func (a *API) handleOrganizationUsers(w http.ResponseWriter, r *http.Request, orgID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
		return
	}
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	user, err := a.rbac.CreateUser(r.Context(), orgID, req.Email, req.Password, req.Status)
	if err != nil {
		handleRBACError(w, r, err)
		return
	}
	a.audit(r.Context(), "rbac.user.create", "user", user.ID, map[string]string{
		"organization_id": orgID,
		"email":           user.Email,
	})
	w.Header().Set("Location", fmt.Sprintf("/v1/users/%s", user.ID))
	writeJSON(w, http.StatusCreated, user)
}

func (a *API) handleOrganizationRoles(w http.ResponseWriter, r *http.Request, orgID string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !a.ensurePermissions(w, r, auth.PermissionManageRoles) {
		return
	}
	var req createRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	role, err := a.rbac.CreateRole(r.Context(), orgID, req.Name, req.Description)
	if err != nil {
		handleRBACError(w, r, err)
		return
	}
	a.audit(r.Context(), "rbac.role.create", "role", role.ID, map[string]string{
		"organization_id": orgID,
		"name":            role.Name,
	})
	w.Header().Set("Location", fmt.Sprintf("/v1/roles/%s", role.ID))
	writeJSON(w, http.StatusCreated, role)
}

func (a *API) handleRoleResource(w http.ResponseWriter, r *http.Request) {
	if a.rbac == nil {
		writeError(w, r, http.StatusServiceUnavailable, "rbac service unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/roles/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "permissions" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	roleID := parts[0]
	if r.Method != http.MethodPut {
		methodNotAllowed(w, r, http.MethodPut)
		return
	}
	if !a.ensurePermissions(w, r, auth.PermissionManagePermissions) {
		return
	}
	var req updateRolePermissionsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.rbac.SetRolePermissions(r.Context(), roleID, req.Permissions); err != nil {
		handleRBACError(w, r, err)
		return
	}
	a.audit(r.Context(), "rbac.role.permissions.update", "role", roleID, map[string]string{
		"count": fmt.Sprintf("%d", len(req.Permissions)),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) handleUserResource(w http.ResponseWriter, r *http.Request) {
	if a.rbac == nil {
		writeError(w, r, http.StatusServiceUnavailable, "rbac service unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/users/")
	path = strings.Trim(path, "/")
	if path == "" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "assignments" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	userID := parts[0]
	if r.Method != http.MethodPost {
		methodNotAllowed(w, r, http.MethodPost)
		return
	}
	if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
		return
	}
	var req assignRoleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	req.RoleID = strings.TrimSpace(req.RoleID)
	if req.RoleID == "" {
		writeError(w, r, http.StatusBadRequest, "role_id is required")
		return
	}
	assignment, err := a.rbac.AssignRoleToUser(r.Context(), userID, req.RoleID)
	if err != nil {
		handleRBACError(w, r, err)
		return
	}
	a.audit(r.Context(), "rbac.user.assign_role", "user", userID, map[string]string{
		"role_id": assignment.RoleID,
	})
	writeJSON(w, http.StatusCreated, assignment)
}

func handleRBACError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeError(w, r, http.StatusBadRequest, err.Error())
	case errors.Is(err, auth.ErrConflict):
		writeError(w, r, http.StatusConflict, err.Error())
	case errors.Is(err, auth.ErrNotFound):
		writeError(w, r, http.StatusNotFound, err.Error())
	default:
		writeError(w, r, http.StatusInternalServerError, "rbac operation failed")
	}
}
