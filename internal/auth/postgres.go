package auth

import (
	"context"
	"database/sql"
	"encoding/json"

	"qazna.org/internal/ids"
)

var _ Store = (*PGStore)(nil)

// PGStore implements Store using PostgreSQL.
type PGStore struct {
	db *sql.DB
}

func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) Organizations(context context.Context) OrganizationStore {
	return &orgStore{db: s.db}
}
func (s *PGStore) Users(context context.Context) UserStore { return &userStore{db: s.db} }
func (s *PGStore) Roles(context context.Context) RoleStore { return &roleStore{db: s.db} }
func (s *PGStore) Permissions(context context.Context) PermissionStore {
	return &permissionStore{db: s.db}
}
func (s *PGStore) Audit(context context.Context) AuditStore { return &auditStore{db: s.db} }

// Organization store -------------------------------------------------------
type orgStore struct{ db *sql.DB }

func (s *orgStore) Create(ctx context.Context, org *Organization) error {
	if org.ID == "" {
		org.ID = ids.New()
	}
	meta, _ := json.Marshal(org.Metadata)
	_, err := s.db.ExecContext(ctx,
		`insert into organizations(id, name, metadata) values($1,$2,$3)`,
		org.ID, org.Name, meta,
	)
	return err
}

func (s *orgStore) Find(ctx context.Context, id string) (*Organization, error) {
	row := s.db.QueryRowContext(ctx,
		`select id, name, created_at, updated_at, metadata from organizations where id=$1`, id,
	)
	var (
		org      Organization
		metadata []byte
	)
	if err := row.Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt, &metadata); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(metadata, &org.Metadata)
	return &org, nil
}

func (s *orgStore) List(ctx context.Context) ([]*Organization, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, name, created_at, updated_at, metadata from organizations order by created_at asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []*Organization
	for rows.Next() {
		var (
			org      Organization
			metadata []byte
		)
		if err := rows.Scan(&org.ID, &org.Name, &org.CreatedAt, &org.UpdatedAt, &metadata); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &org.Metadata)
		res = append(res, &org)
	}
	return res, rows.Err()
}

// User store ---------------------------------------------------------------
type userStore struct{ db *sql.DB }

func (s *userStore) Create(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = ids.New()
	}
	_, err := s.db.ExecContext(ctx,
		`insert into users(id, organization_id, email, status) values($1,$2,$3,$4)`,
		u.ID, u.OrganizationID, u.Email, u.Status,
	)
	return err
}

func (s *userStore) Find(ctx context.Context, id string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`select id, organization_id, email, status, created_at, updated_at from users where id=$1`, id)
	var u User
	if err := row.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *userStore) FindByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`select id, organization_id, email, status, created_at, updated_at from users where email=$1`, email)
	var u User
	if err := row.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *userStore) ListByOrg(ctx context.Context, orgID string) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, organization_id, email, status, created_at, updated_at from users where organization_id=$1 order by created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.Status, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

// Role store ---------------------------------------------------------------
type roleStore struct{ db *sql.DB }

func (s *roleStore) Create(ctx context.Context, role *Role) error {
	if role.ID == "" {
		role.ID = ids.New()
	}
	_, err := s.db.ExecContext(ctx,
		`insert into roles(id, organization_id, name, description) values($1,$2,$3,$4)`,
		role.ID, role.OrganizationID, role.Name, role.Description,
	)
	return err
}

func (s *roleStore) Find(ctx context.Context, id string) (*Role, error) {
	row := s.db.QueryRowContext(ctx,
		`select id, organization_id, name, description, created_at, updated_at from roles where id=$1`, id)
	var role Role
	if err := row.Scan(&role.ID, &role.OrganizationID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &role, nil
}

func (s *roleStore) ListByOrg(ctx context.Context, orgID string) ([]*Role, error) {
	rows, err := s.db.QueryContext(ctx,
		`select id, organization_id, name, description, created_at, updated_at from roles where organization_id=$1 order by created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Name, &role.Description, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, &role)
	}
	return roles, rows.Err()
}

func (s *roleStore) Assign(ctx context.Context, assignment Assignment) error {
	_, err := s.db.ExecContext(ctx,
		`insert into user_roles(user_id, role_id, organization_id) values($1,$2,$3) on conflict do nothing`,
		assignment.UserID, assignment.RoleID, assignment.OrganizationID,
	)
	return err
}

func (s *roleStore) Assignments(ctx context.Context, userID string) ([]Assignment, error) {
	rows, err := s.db.QueryContext(ctx,
		`select user_id, role_id, organization_id, created_at from user_roles where user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Assignment
	for rows.Next() {
		var a Assignment
		if err := rows.Scan(&a.UserID, &a.RoleID, &a.OrganizationID, &a.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// Permission store ---------------------------------------------------------
type permissionStore struct{ db *sql.DB }

func (s *permissionStore) Ensure(ctx context.Context, perms []Permission) error {
	for _, p := range perms {
		if p.ID == "" {
			p.ID = ids.New()
		}
		_, err := s.db.ExecContext(ctx,
			`insert into permissions(id, key, description) values($1,$2,$3) on conflict (key) do nothing`,
			p.ID, p.Key, p.Description,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *permissionStore) List(ctx context.Context) ([]Permission, error) {
	rows, err := s.db.QueryContext(ctx, `select id, key, description, created_at from permissions order by key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Key, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (s *permissionStore) SetForRole(ctx context.Context, roleID string, permKeys []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `delete from role_permissions where role_id=$1`, roleID); err != nil {
		return err
	}
	for _, key := range permKeys {
		_, err := tx.ExecContext(ctx,
			`insert into role_permissions(role_id, permission_id)
			 select $1, id from permissions where key=$2`, roleID, key,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *permissionStore) PermissionsForRole(ctx context.Context, roleID string) ([]Permission, error) {
	rows, err := s.db.QueryContext(ctx,
		`select p.id, p.key, p.description, p.created_at from permissions p
		 join role_permissions rp on rp.permission_id=p.id where rp.role_id=$1`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Key, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// Audit store --------------------------------------------------------------
type auditStore struct{ db *sql.DB }

func (s *auditStore) Append(ctx context.Context, entry *AuditEntry) error {
	if entry.ID == "" {
		entry.ID = ids.New()
	}
	meta, _ := json.Marshal(entry.Metadata)
	_, err := s.db.ExecContext(ctx,
		`insert into audit_log(id, occurred_at, actor_user_id, actor_org_id, action, resource_type, resource_id, metadata, trace_id)
		 values($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		entry.ID, entry.OccurredAt, entry.ActorUserID, entry.ActorOrgID, entry.Action,
		entry.ResourceType, entry.ResourceID, meta, entry.TraceID,
	)
	return err
}
