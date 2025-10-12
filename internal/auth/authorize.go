package auth

// Principal represents a user with resolved roles and permissions.
type Principal struct {
	User        *User
	Assignments []Assignment
	Permissions map[string]struct{}
}

// NewPrincipal constructs a principal with preloaded permissions.
func NewPrincipal(user *User, assignments []Assignment, perms []Permission) Principal {
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		set[p.Key] = struct{}{}
	}
	return Principal{User: user, Assignments: assignments, Permissions: set}
}

// HasPermission reports whether the principal can execute action identified by key.
func (p Principal) HasPermission(key string) bool {
	_, ok := p.Permissions[key]
	return ok
}
