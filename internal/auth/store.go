package auth

import "context"

// Store describes persistence operations required by the auth subsystem.
type Store interface {
	Organizations(ctx context.Context) OrganizationStore
	Users(ctx context.Context) UserStore
	Roles(ctx context.Context) RoleStore
	Permissions(ctx context.Context) PermissionStore
	Audit(ctx context.Context) AuditStore
	RefreshTokens(ctx context.Context) RefreshTokenStore
}

// OrganizationStore manages organizations.
type OrganizationStore interface {
	Create(ctx context.Context, org *Organization) error
	Find(ctx context.Context, id string) (*Organization, error)
	List(ctx context.Context) ([]*Organization, error)
}

// UserStore manages users.
type UserStore interface {
	Create(ctx context.Context, u *User) error
	Find(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	ListByOrg(ctx context.Context, orgID string) ([]*User, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
}

// RoleStore manages roles and assignments.
type RoleStore interface {
	Create(ctx context.Context, role *Role) error
	Find(ctx context.Context, id string) (*Role, error)
	ListByOrg(ctx context.Context, orgID string) ([]*Role, error)
	Assign(ctx context.Context, assignment Assignment) error
	Assignments(ctx context.Context, userID string) ([]Assignment, error)
}

// PermissionStore manages permission catalog.
type PermissionStore interface {
	Ensure(ctx context.Context, perms []Permission) error
	List(ctx context.Context) ([]Permission, error)
	SetForRole(ctx context.Context, roleID string, perms []string) error
	PermissionsForRole(ctx context.Context, roleID string) ([]Permission, error)
}

// AuditStore appends immutable entries.
type AuditStore interface {
	Append(ctx context.Context, entry *AuditEntry) error
}

// RefreshTokenStore manages refresh token lifecycle.
type RefreshTokenStore interface {
	Create(ctx context.Context, tok *RefreshToken) error
	Find(ctx context.Context, id string) (*RefreshToken, error)
	MarkRevoked(ctx context.Context, id string) error
	MarkRevokedByUser(ctx context.Context, userID string) error
}
