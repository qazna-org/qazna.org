package auth

import "time"

// Organization represents a sovereign participant or institutional partner.
type Organization struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Metadata  map[string]string
}

// User represents a human or service account operating on behalf of an organization.
type User struct {
	ID             string
	OrganizationID string
	Email          string
	PasswordHash   string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Role groups permissions.
type Role struct {
	ID             string
	OrganizationID string
	Name           string
	Description    string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Permission is a fine-grained capability.
type Permission struct {
	ID          string
	Key         string
	Description string
	CreatedAt   time.Time
}

// AuditEntry is an append-only log of critical actions.
type AuditEntry struct {
	ID           string
	OccurredAt   time.Time
	ActorUserID  string
	ActorOrgID   string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
	TraceID      string
}

// Assignment gives a user a role in optional scope.
type Assignment struct {
	UserID         string
	RoleID         string
	OrganizationID string
	CreatedAt      time.Time
}

// RolePermission links roles to permissions.
type RolePermission struct {
	RoleID       string
	PermissionID string
}

// RefreshToken represents a persisted refresh token.
type RefreshToken struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	Revoked    bool
}
