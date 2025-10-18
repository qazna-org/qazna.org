package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("resource conflict")
)

const (
	userStatusActive   = "active"
	userStatusDisabled = "disabled"
)

const (
	UserStatusActive   = userStatusActive
	UserStatusDisabled = userStatusDisabled
)

type Organization struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type User struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Email          string    `json:"email"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Role struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type UserRoleAssignment struct {
	UserID         string    `json:"user_id"`
	RoleID         string    `json:"role_id"`
	OrganizationID string    `json:"organization_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type RBACStore interface {
	CreateOrganization(ctx context.Context, name string, metadata map[string]any) (Organization, error)
	ListOrganizations(ctx context.Context) ([]Organization, error)
	GetOrganization(ctx context.Context, id string) (Organization, error)
	UpdateOrganization(ctx context.Context, id string, upd OrganizationUpdate) (Organization, error)
	DeleteOrganization(ctx context.Context, id string) error

	CreateUser(ctx context.Context, organizationID, email, passwordHash, status string) (User, error)
	ListUsers(ctx context.Context, organizationID string) ([]User, error)
	GetUser(ctx context.Context, organizationID, userID string) (User, error)
	UpdateUser(ctx context.Context, userID string, upd UserUpdate) (User, error)
	DeleteUser(ctx context.Context, userID string) error

	CreateRole(ctx context.Context, organizationID, name, description string) (Role, error)
	ListRoles(ctx context.Context, organizationID string) ([]Role, error)
	GetRole(ctx context.Context, roleID string) (Role, error)
	UpdateRole(ctx context.Context, roleID string, upd RoleUpdate) (Role, error)
	DeleteRole(ctx context.Context, roleID string) error

	SetRolePermissions(ctx context.Context, roleID string, permissionKeys []string) error
	AssignRoleToUser(ctx context.Context, userID, roleID string) (UserRoleAssignment, error)
	RemoveRoleAssignment(ctx context.Context, userID, roleID string) error
	ListRoleAssignments(ctx context.Context, userID string) ([]UserRoleAssignment, error)
	UserPermissions(ctx context.Context, userID string) ([]string, error)
}

type OrganizationUpdate struct {
	Name     *string
	Metadata map[string]any
}

type UserUpdate struct {
	Email    *string
	Password *string
	Status   *string
}

type RoleUpdate struct {
	Name        *string
	Description *string
}

type RBACService struct {
	store RBACStore
}

func NewRBACService(store RBACStore) (*RBACService, error) {
	if store == nil {
		return nil, errors.New("rbac store is required")
	}
	return &RBACService{store: store}, nil
}

func (s *RBACService) CreateOrganization(ctx context.Context, name string, metadata map[string]any) (Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Organization{}, fmt.Errorf("%w: organization name is required", ErrInvalidInput)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	return s.store.CreateOrganization(ctx, name, metadata)
}

func (s *RBACService) ListOrganizations(ctx context.Context) ([]Organization, error) {
	return s.store.ListOrganizations(ctx)
}

func (s *RBACService) GetOrganization(ctx context.Context, id string) (Organization, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Organization{}, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	return s.store.GetOrganization(ctx, id)
}

func (s *RBACService) UpdateOrganization(ctx context.Context, id string, upd OrganizationUpdate) (Organization, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Organization{}, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	if upd.Name != nil {
		trimmed := strings.TrimSpace(*upd.Name)
		if trimmed == "" {
			return Organization{}, fmt.Errorf("%w: organization name is required", ErrInvalidInput)
		}
		upd.Name = &trimmed
	}
	return s.store.UpdateOrganization(ctx, id, upd)
}

func (s *RBACService) DeleteOrganization(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	return s.store.DeleteOrganization(ctx, id)
}

func (s *RBACService) CreateUser(ctx context.Context, organizationID, email, password, status string) (User, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return User{}, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || !strings.Contains(email, "@") {
		return User{}, fmt.Errorf("%w: valid email is required", ErrInvalidInput)
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return User{}, fmt.Errorf("%w: password is required", ErrInvalidInput)
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		status = userStatusActive
	}
	if status != userStatusActive && status != userStatusDisabled {
		return User{}, fmt.Errorf("%w: unsupported status %s", ErrInvalidInput, status)
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	return s.store.CreateUser(ctx, organizationID, email, hash, status)
}

func (s *RBACService) ListUsers(ctx context.Context, organizationID string) ([]User, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return nil, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	return s.store.ListUsers(ctx, organizationID)
}

func (s *RBACService) GetUser(ctx context.Context, organizationID, userID string) (User, error) {
	organizationID = strings.TrimSpace(organizationID)
	userID = strings.TrimSpace(userID)
	if organizationID == "" || userID == "" {
		return User{}, fmt.Errorf("%w: organization_id and user_id are required", ErrInvalidInput)
	}
	return s.store.GetUser(ctx, organizationID, userID)
}

func (s *RBACService) UpdateUser(ctx context.Context, userID string, upd UserUpdate) (User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return User{}, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if upd.Email != nil {
		trimmed := strings.TrimSpace(strings.ToLower(*upd.Email))
		if trimmed == "" || !strings.Contains(trimmed, "@") {
			return User{}, fmt.Errorf("%w: valid email is required", ErrInvalidInput)
		}
		upd.Email = &trimmed
	}
	if upd.Status != nil {
		status := strings.TrimSpace(strings.ToLower(*upd.Status))
		if status == "" {
			status = userStatusActive
		}
		if status != userStatusActive && status != userStatusDisabled {
			return User{}, fmt.Errorf("%w: unsupported status %s", ErrInvalidInput, status)
		}
		upd.Status = &status
	}
	if upd.Password != nil {
		pw := strings.TrimSpace(*upd.Password)
		if pw == "" {
			return User{}, fmt.Errorf("%w: password is required", ErrInvalidInput)
		}
		hash, err := hashPassword(pw)
		if err != nil {
			return User{}, err
		}
		upd.Password = &hash
	}
	return s.store.UpdateUser(ctx, userID, upd)
}

func (s *RBACService) DeleteUser(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	return s.store.DeleteUser(ctx, userID)
}

func (s *RBACService) CreateRole(ctx context.Context, organizationID, name, description string) (Role, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return Role{}, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, fmt.Errorf("%w: role name is required", ErrInvalidInput)
	}
	description = strings.TrimSpace(description)
	return s.store.CreateRole(ctx, organizationID, name, description)
}

func (s *RBACService) ListRoles(ctx context.Context, organizationID string) ([]Role, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return nil, fmt.Errorf("%w: organization_id is required", ErrInvalidInput)
	}
	return s.store.ListRoles(ctx, organizationID)
}

func (s *RBACService) GetRole(ctx context.Context, roleID string) (Role, error) {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return Role{}, fmt.Errorf("%w: role_id is required", ErrInvalidInput)
	}
	return s.store.GetRole(ctx, roleID)
}

func (s *RBACService) UpdateRole(ctx context.Context, roleID string, upd RoleUpdate) (Role, error) {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return Role{}, fmt.Errorf("%w: role_id is required", ErrInvalidInput)
	}
	if upd.Name != nil {
		name := strings.TrimSpace(*upd.Name)
		if name == "" {
			return Role{}, fmt.Errorf("%w: role name is required", ErrInvalidInput)
		}
		upd.Name = &name
	}
	if upd.Description != nil {
		desc := strings.TrimSpace(*upd.Description)
		upd.Description = &desc
	}
	return s.store.UpdateRole(ctx, roleID, upd)
}

func (s *RBACService) DeleteRole(ctx context.Context, roleID string) error {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return fmt.Errorf("%w: role_id is required", ErrInvalidInput)
	}
	return s.store.DeleteRole(ctx, roleID)
}

func (s *RBACService) SetRolePermissions(ctx context.Context, roleID string, permissions []string) error {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return fmt.Errorf("%w: role_id is required", ErrInvalidInput)
	}
	keys := dedupeStrings(permissions)
	return s.store.SetRolePermissions(ctx, roleID, keys)
}

func (s *RBACService) AssignRoleToUser(ctx context.Context, userID, roleID string) (UserRoleAssignment, error) {
	userID = strings.TrimSpace(userID)
	roleID = strings.TrimSpace(roleID)
	if userID == "" || roleID == "" {
		return UserRoleAssignment{}, fmt.Errorf("%w: user_id and role_id are required", ErrInvalidInput)
	}
	return s.store.AssignRoleToUser(ctx, userID, roleID)
}

func (s *RBACService) RemoveRoleAssignment(ctx context.Context, userID, roleID string) error {
	userID = strings.TrimSpace(userID)
	roleID = strings.TrimSpace(roleID)
	if userID == "" || roleID == "" {
		return fmt.Errorf("%w: user_id and role_id are required", ErrInvalidInput)
	}
	return s.store.RemoveRoleAssignment(ctx, userID, roleID)
}

func (s *RBACService) ListRoleAssignments(ctx context.Context, userID string) ([]UserRoleAssignment, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	return s.store.ListRoleAssignments(ctx, userID)
}

func (s *RBACService) UserPermissions(ctx context.Context, userID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	return s.store.UserPermissions(ctx, userID)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func hashPassword(password string) (string, error) {
	const (
		memory      = 64 * 1024
		iterations  = 2
		parallelism = 1
		keyLength   = 32
		saltLength  = 16
	)

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}
