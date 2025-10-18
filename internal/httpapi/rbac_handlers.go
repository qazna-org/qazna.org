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

type updateOrganizationRequest struct {
	Name     *string         `json:"name"`
	Metadata *map[string]any `json:"metadata"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Status   string `json:"status"`
}

type updateUserRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
	Status   *string `json:"status"`
}

type createRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateRoleRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type updateRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

type assignRoleRequest struct {
	RoleID string `json:"role_id"`
}

func (a *API) handleOrganizations(w http.ResponseWriter, r *http.Request) {
	if a.rbac == nil {
		writeError(w, r, http.StatusServiceUnavailable, "rbac service unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !a.ensurePermissions(w, r, auth.PermissionManageOrganizations) {
			return
		}
		orgs, err := a.rbac.ListOrganizations(r.Context())
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, orgs)
	case http.MethodPost:
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
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
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
	orgID := parts[0]
	switch {
	case len(parts) == 1:
		a.handleOrganizationResource(w, r, orgID)
	case len(parts) >= 2 && parts[1] == "users":
		if len(parts) == 2 {
			a.handleOrganizationUsersCollection(w, r, orgID)
			return
		}
		if len(parts) == 3 {
			a.handleOrganizationUserResource(w, r, orgID, parts[2])
			return
		}
		writeError(w, r, http.StatusNotFound, "resource not found")
	case len(parts) >= 2 && parts[1] == "roles":
		if len(parts) == 2 {
			a.handleOrganizationRolesCollection(w, r, orgID)
			return
		}
		if len(parts) == 3 {
			a.handleOrganizationRoleResource(w, r, orgID, parts[2])
			return
		}
		writeError(w, r, http.StatusNotFound, "resource not found")
	default:
		writeError(w, r, http.StatusNotFound, "resource not found")
	}
}

func (a *API) handleOrganizationResource(w http.ResponseWriter, r *http.Request, orgID string) {
	if !a.ensurePermissions(w, r, auth.PermissionManageOrganizations) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		org, err := a.rbac.GetOrganization(r.Context(), orgID)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, org)
	case http.MethodPatch:
		var req updateOrganizationRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		var upd auth.OrganizationUpdate
		if req.Name != nil {
			upd.Name = req.Name
		}
		if req.Metadata != nil {
			upd.Metadata = *req.Metadata
		}
		org, err := a.rbac.UpdateOrganization(r.Context(), orgID, upd)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.organization.update", "organization", orgID, map[string]string{
			"name": org.Name,
		})
		writeJSON(w, http.StatusOK, org)
	case http.MethodDelete:
		if err := a.rbac.DeleteOrganization(r.Context(), orgID); err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.organization.delete", "organization", orgID, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (a *API) handleOrganizationUsersCollection(w http.ResponseWriter, r *http.Request, orgID string) {
	switch r.Method {
	case http.MethodGet:
		if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
			return
		}
		users, err := a.rbac.ListUsers(r.Context(), orgID)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, users)
	case http.MethodPost:
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
		w.Header().Set("Location", fmt.Sprintf("/v1/organizations/%s/users/%s", orgID, user.ID))
		writeJSON(w, http.StatusCreated, user)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (a *API) handleOrganizationUserResource(w http.ResponseWriter, r *http.Request, orgID, userID string) {
	if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, err := a.rbac.GetUser(r.Context(), orgID, userID)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, user)
	case http.MethodPatch:
		var req updateUserRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		upd := auth.UserUpdate{
			Email:    req.Email,
			Status:   req.Status,
			Password: req.Password,
		}
		user, err := a.rbac.UpdateUser(r.Context(), userID, upd)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.user.update", "user", userID, map[string]string{
			"organization_id": user.OrganizationID,
			"email":           user.Email,
		})
		writeJSON(w, http.StatusOK, user)
	case http.MethodDelete:
		if err := a.rbac.DeleteUser(r.Context(), userID); err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.user.delete", "user", userID, map[string]string{
			"organization_id": orgID,
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
}

func (a *API) handleOrganizationRolesCollection(w http.ResponseWriter, r *http.Request, orgID string) {
	switch r.Method {
	case http.MethodGet:
		if !a.ensurePermissions(w, r, auth.PermissionManageRoles) {
			return
		}
		roles, err := a.rbac.ListRoles(r.Context(), orgID)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, roles)
	case http.MethodPost:
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
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
	}
}

func (a *API) handleOrganizationRoleResource(w http.ResponseWriter, r *http.Request, orgID, roleID string) {
	if !a.ensurePermissions(w, r, auth.PermissionManageRoles) {
		return
	}
	role, err := a.rbac.GetRole(r.Context(), roleID)
	if err != nil {
		handleRBACError(w, r, err)
		return
	}
	if role.OrganizationID != orgID {
		writeError(w, r, http.StatusNotFound, "role not found in organization")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, role)
	case http.MethodPatch:
		var req updateRoleRequest
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, http.StatusBadRequest, err.Error())
			return
		}
		upd := auth.RoleUpdate{
			Name:        req.Name,
			Description: req.Description,
		}
		updated, err := a.rbac.UpdateRole(r.Context(), roleID, upd)
		if err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.role.update", "role", roleID, map[string]string{
			"name": updated.Name,
		})
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.rbac.DeleteRole(r.Context(), roleID); err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.role.delete", "role", roleID, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
	}
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
	switch {
	case len(parts) == 1:
		roleID := parts[0]
		if !a.ensurePermissions(w, r, auth.PermissionManageRoles) {
			return
		}
		switch r.Method {
		case http.MethodGet:
			role, err := a.rbac.GetRole(r.Context(), roleID)
			if err != nil {
				handleRBACError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, role)
		case http.MethodPatch:
			var req updateRoleRequest
			if err := decodeJSON(w, r, &req); err != nil {
				writeError(w, r, http.StatusBadRequest, err.Error())
				return
			}
			upd := auth.RoleUpdate{
				Name:        req.Name,
				Description: req.Description,
			}
			role, err := a.rbac.UpdateRole(r.Context(), roleID, upd)
			if err != nil {
				handleRBACError(w, r, err)
				return
			}
			a.audit(r.Context(), "rbac.role.update", "role", roleID, map[string]string{
				"name": role.Name,
			})
			writeJSON(w, http.StatusOK, role)
		case http.MethodDelete:
			if err := a.rbac.DeleteRole(r.Context(), roleID); err != nil {
				handleRBACError(w, r, err)
				return
			}
			a.audit(r.Context(), "rbac.role.delete", "role", roleID, nil)
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w, r, http.MethodGet, http.MethodPatch, http.MethodDelete)
		}
	case len(parts) == 2 && parts[1] == "permissions":
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
	default:
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
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
	if len(parts) < 2 || parts[1] != "assignments" {
		writeError(w, r, http.StatusNotFound, "resource not found")
		return
	}
	userID := parts[0]
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
				return
			}
			assignments, err := a.rbac.ListRoleAssignments(r.Context(), userID)
			if err != nil {
				handleRBACError(w, r, err)
				return
			}
			writeJSON(w, http.StatusOK, assignments)
		case http.MethodPost:
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
		default:
			methodNotAllowed(w, r, http.MethodGet, http.MethodPost)
		}
		return
	}
	if len(parts) == 3 {
		if r.Method != http.MethodDelete {
			methodNotAllowed(w, r, http.MethodDelete)
			return
		}
		if !a.ensurePermissions(w, r, auth.PermissionManageUsers) {
			return
		}
		roleID := strings.TrimSpace(parts[2])
		if roleID == "" {
			writeError(w, r, http.StatusBadRequest, "role_id is required")
			return
		}
		if err := a.rbac.RemoveRoleAssignment(r.Context(), userID, roleID); err != nil {
			handleRBACError(w, r, err)
			return
		}
		a.audit(r.Context(), "rbac.user.unassign_role", "user", userID, map[string]string{
			"role_id": roleID,
		})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeError(w, r, http.StatusNotFound, "resource not found")
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
