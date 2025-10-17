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
	CreateUser(ctx context.Context, organizationID, email, passwordHash, status string) (User, error)
	CreateRole(ctx context.Context, organizationID, name, description string) (Role, error)
	SetRolePermissions(ctx context.Context, roleID string, permissionKeys []string) error
	AssignRoleToUser(ctx context.Context, userID, roleID string) (UserRoleAssignment, error)
	UserPermissions(ctx context.Context, userID string) ([]string, error)
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
