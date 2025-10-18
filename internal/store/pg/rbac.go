package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"qazna.org/internal/auth"
	"qazna.org/internal/ids"
)

const (
	pgErrUniqueViolation     = "23505"
	pgErrForeignKeyViolation = "23503"
)

var _ auth.RBACStore = (*Store)(nil)

func (s *Store) CreateOrganization(ctx context.Context, name string, metadata map[string]any) (auth.Organization, error) {
	if s.db == nil {
		return auth.Organization{}, errors.New("database connection unavailable")
	}

	id := ids.New()
	metaJSON := []byte("{}")
	if len(metadata) > 0 {
		bytes, err := json.Marshal(metadata)
		if err != nil {
			return auth.Organization{}, fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = bytes
	}

	var (
		org    auth.Organization
		rawMet []byte
	)
	row := s.db.QueryRowContext(ctx, `
		insert into organizations (id, name, metadata)
		values ($1, $2, $3)
		returning id, name, metadata, created_at, updated_at
	`, id, name, metaJSON)
	if err := row.Scan(&org.ID, &org.Name, &rawMet, &org.CreatedAt, &org.UpdatedAt); err != nil {
		if pgErr, ok := maybePgError(err); ok && pgErr.Code == pgErrUniqueViolation {
			return auth.Organization{}, auth.ErrConflict
		}
		return auth.Organization{}, err
	}
	org.Metadata = map[string]any{}
	if len(rawMet) > 0 {
		if err := json.Unmarshal(rawMet, &org.Metadata); err != nil {
			return auth.Organization{}, fmt.Errorf("decode metadata: %w", err)
		}
	}
	return org, nil
}

func (s *Store) ListOrganizations(ctx context.Context) ([]auth.Organization, error) {
	if s.db == nil {
		return nil, errors.New("database connection unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, name, metadata, created_at, updated_at
		from organizations
		order by name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []auth.Organization
	for rows.Next() {
		var (
			org    auth.Organization
			rawMet []byte
		)
		if err := rows.Scan(&org.ID, &org.Name, &rawMet, &org.CreatedAt, &org.UpdatedAt); err != nil {
			return nil, err
		}
		org.Metadata = map[string]any{}
		if len(rawMet) > 0 {
			if err := json.Unmarshal(rawMet, &org.Metadata); err != nil {
				return nil, err
			}
		}
		result = append(result, org)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) GetOrganization(ctx context.Context, id string) (auth.Organization, error) {
	if s.db == nil {
		return auth.Organization{}, errors.New("database connection unavailable")
	}
	var (
		org    auth.Organization
		rawMet []byte
	)
	err := s.db.QueryRowContext(ctx, `
		select id, name, metadata, created_at, updated_at
		from organizations
		where id = $1
	`, id).Scan(&org.ID, &org.Name, &rawMet, &org.CreatedAt, &org.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Organization{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Organization{}, err
	}
	org.Metadata = map[string]any{}
	if len(rawMet) > 0 {
		if err := json.Unmarshal(rawMet, &org.Metadata); err != nil {
			return auth.Organization{}, err
		}
	}
	return org, nil
}

func (s *Store) UpdateOrganization(ctx context.Context, id string, upd auth.OrganizationUpdate) (auth.Organization, error) {
	if s.db == nil {
		return auth.Organization{}, errors.New("database connection unavailable")
	}

	var (
		setClauses []string
		args       []any
		idx        = 1
	)
	if upd.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *upd.Name)
		idx++
	}
	if upd.Metadata != nil {
		bytes, err := json.Marshal(upd.Metadata)
		if err != nil {
			return auth.Organization{}, fmt.Errorf("marshal metadata: %w", err)
		}
		setClauses = append(setClauses, fmt.Sprintf("metadata = $%d", idx))
		args = append(args, bytes)
		idx++
	}
	if len(setClauses) > 0 {
		setClauses = append(setClauses, fmt.Sprintf("updated_at = now()"))
		query := fmt.Sprintf(`update organizations set %s where id = $%d`, strings.Join(setClauses, ", "), idx)
		args = append(args, id)
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return auth.Organization{}, auth.ErrNotFound
			}
			return auth.Organization{}, err
		}
	}
	return s.GetOrganization(ctx, id)
}

func (s *Store) DeleteOrganization(ctx context.Context, id string) error {
	if s.db == nil {
		return errors.New("database connection unavailable")
	}
	res, err := s.db.ExecContext(ctx, `delete from organizations where id = $1`, id)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return auth.ErrNotFound
	}
	return nil
}

func (s *Store) CreateUser(ctx context.Context, organizationID, email, passwordHash, status string) (auth.User, error) {
	if s.db == nil {
		return auth.User{}, errors.New("database connection unavailable")
	}
	var user auth.User
	row := s.db.QueryRowContext(ctx, `
		insert into users (id, organization_id, email, password_hash, status)
		values ($1, $2, $3, $4, $5)
		returning id, organization_id, email, status, created_at, updated_at
	`, ids.New(), organizationID, email, passwordHash, status)
	if err := row.Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if pgErr, ok := maybePgError(err); ok {
			switch pgErr.Code {
			case pgErrUniqueViolation:
				return auth.User{}, auth.ErrConflict
			case pgErrForeignKeyViolation:
				return auth.User{}, auth.ErrNotFound
			}
		}
		return auth.User{}, err
	}
	return user, nil
}

func (s *Store) ListUsers(ctx context.Context, organizationID string) ([]auth.User, error) {
	if s.db == nil {
		return nil, errors.New("database connection unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, organization_id, email, status, created_at, updated_at
		from users
		where organization_id = $1
		order by email
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []auth.User
	for rows.Next() {
		var user auth.User
		if err := rows.Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Status, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) GetUser(ctx context.Context, organizationID, userID string) (auth.User, error) {
	if s.db == nil {
		return auth.User{}, errors.New("database connection unavailable")
	}
	var user auth.User
	err := s.db.QueryRowContext(ctx, `
		select id, organization_id, email, status, created_at, updated_at
		from users
		where organization_id = $1 and id = $2
	`, organizationID, userID).Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.User{}, err
	}
	return user, nil
}

func (s *Store) UpdateUser(ctx context.Context, userID string, upd auth.UserUpdate) (auth.User, error) {
	if s.db == nil {
		return auth.User{}, errors.New("database connection unavailable")
	}
	var (
		sets []string
		args []any
		idx  = 1
	)
	if upd.Email != nil {
		sets = append(sets, fmt.Sprintf("email = $%d", idx))
		args = append(args, *upd.Email)
		idx++
	}
	if upd.Password != nil {
		sets = append(sets, fmt.Sprintf("password_hash = $%d", idx))
		args = append(args, *upd.Password)
		idx++
	}
	if upd.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, *upd.Status)
		idx++
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf(`update users set %s where id = $%d`, strings.Join(sets, ", "), idx)
		args = append(args, userID)
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			if pgErr, ok := maybePgError(err); ok && pgErr.Code == pgErrUniqueViolation {
				return auth.User{}, auth.ErrConflict
			}
			return auth.User{}, err
		}
		aff, err := res.RowsAffected()
		if err != nil {
			return auth.User{}, err
		}
		if aff == 0 {
			return auth.User{}, auth.ErrNotFound
		}
	}
	return s.userByID(ctx, userID)
}

func (s *Store) DeleteUser(ctx context.Context, userID string) error {
	if s.db == nil {
		return errors.New("database connection unavailable")
	}
	res, err := s.db.ExecContext(ctx, `delete from users where id = $1`, userID)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return auth.ErrNotFound
	}
	return nil
}

func (s *Store) CreateRole(ctx context.Context, organizationID, name, description string) (auth.Role, error) {
	if s.db == nil {
		return auth.Role{}, errors.New("database connection unavailable")
	}
	var (
		role auth.Role
		desc sql.NullString
	)
	row := s.db.QueryRowContext(ctx, `
		insert into roles (id, organization_id, name, description)
		values ($1, $2, $3, $4)
		returning id, organization_id, name, description, created_at, updated_at
	`, ids.New(), organizationID, name, nullIfEmpty(description))
	if err := row.Scan(&role.ID, &role.OrganizationID, &role.Name, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
		if pgErr, ok := maybePgError(err); ok {
			switch pgErr.Code {
			case pgErrUniqueViolation:
				return auth.Role{}, auth.ErrConflict
			case pgErrForeignKeyViolation:
				return auth.Role{}, auth.ErrNotFound
			}
		}
		return auth.Role{}, err
	}
	if desc.Valid {
		role.Description = desc.String
	}
	return role, nil
}

func (s *Store) ListRoles(ctx context.Context, organizationID string) ([]auth.Role, error) {
	if s.db == nil {
		return nil, errors.New("database connection unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		select id, organization_id, name, description, created_at, updated_at
		from roles
		where organization_id = $1
		order by name
	`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []auth.Role
	for rows.Next() {
		var (
			role auth.Role
			desc sql.NullString
		)
		if err := rows.Scan(&role.ID, &role.OrganizationID, &role.Name, &desc, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			role.Description = desc.String
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Store) GetRole(ctx context.Context, roleID string) (auth.Role, error) {
	if s.db == nil {
		return auth.Role{}, errors.New("database connection unavailable")
	}
	var (
		role auth.Role
		desc sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		select id, organization_id, name, description, created_at, updated_at
		from roles
		where id = $1
	`, roleID).Scan(&role.ID, &role.OrganizationID, &role.Name, &desc, &role.CreatedAt, &role.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Role{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.Role{}, err
	}
	if desc.Valid {
		role.Description = desc.String
	}
	return role, nil
}

func (s *Store) UpdateRole(ctx context.Context, roleID string, upd auth.RoleUpdate) (auth.Role, error) {
	if s.db == nil {
		return auth.Role{}, errors.New("database connection unavailable")
	}
	var (
		sets []string
		args []any
		idx  = 1
	)
	if upd.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *upd.Name)
		idx++
	}
	if upd.Description != nil {
		if *upd.Description == "" {
			sets = append(sets, fmt.Sprintf("description = NULL"))
		} else {
			sets = append(sets, fmt.Sprintf("description = $%d", idx))
			args = append(args, *upd.Description)
			idx++
		}
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_at = now()")
		query := fmt.Sprintf(`update roles set %s where id = $%d`, strings.Join(sets, ", "), idx)
		args = append(args, roleID)
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			if pgErr, ok := maybePgError(err); ok {
				switch pgErr.Code {
				case pgErrUniqueViolation:
					return auth.Role{}, auth.ErrConflict
				case pgErrForeignKeyViolation:
					return auth.Role{}, auth.ErrNotFound
				}
			}
			return auth.Role{}, err
		}
		aff, err := res.RowsAffected()
		if err != nil {
			return auth.Role{}, err
		}
		if aff == 0 {
			return auth.Role{}, auth.ErrNotFound
		}
	}
	return s.GetRole(ctx, roleID)
}

func (s *Store) DeleteRole(ctx context.Context, roleID string) error {
	if s.db == nil {
		return errors.New("database connection unavailable")
	}
	res, err := s.db.ExecContext(ctx, `delete from roles where id = $1`, roleID)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return auth.ErrNotFound
	}
	return nil
}

func (s *Store) SetRolePermissions(ctx context.Context, roleID string, permissionKeys []string) error {
	if s.db == nil {
		return errors.New("database connection unavailable")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, `select 1 from roles where id = $1`, roleID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.ErrNotFound
		}
		return err
	}

	if _, err := tx.ExecContext(ctx, `delete from role_permissions where role_id = $1`, roleID); err != nil {
		return err
	}
	if len(permissionKeys) == 0 {
		return tx.Commit()
	}

	for _, key := range permissionKeys {
		var permID string
		err := tx.QueryRowContext(ctx, `select id from permissions where key = $1`, key).Scan(&permID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: permission %s not found", auth.ErrNotFound, key)
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			insert into role_permissions (role_id, permission_id)
			values ($1, $2)
		`, roleID, permID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AssignRoleToUser(ctx context.Context, userID, roleID string) (auth.UserRoleAssignment, error) {
	if s.db == nil {
		return auth.UserRoleAssignment{}, errors.New("database connection unavailable")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return auth.UserRoleAssignment{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var userOrg string
	if err := tx.QueryRowContext(ctx, `select organization_id from users where id = $1`, userID).Scan(&userOrg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.UserRoleAssignment{}, auth.ErrNotFound
		}
		return auth.UserRoleAssignment{}, err
	}

	var roleOrg string
	if err := tx.QueryRowContext(ctx, `select organization_id from roles where id = $1`, roleID).Scan(&roleOrg); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return auth.UserRoleAssignment{}, auth.ErrNotFound
		}
		return auth.UserRoleAssignment{}, err
	}

	if userOrg != roleOrg {
		return auth.UserRoleAssignment{}, fmt.Errorf("%w: user and role belong to different organizations", auth.ErrInvalidInput)
	}

	var assignment auth.UserRoleAssignment
	err = tx.QueryRowContext(ctx, `
		insert into user_roles (user_id, role_id, organization_id)
		values ($1, $2, $3)
		returning user_id, role_id, organization_id, created_at
	`, userID, roleID, userOrg).Scan(&assignment.UserID, &assignment.RoleID, &assignment.OrganizationID, &assignment.CreatedAt)
	if err != nil {
		if pgErr, ok := maybePgError(err); ok && pgErr.Code == pgErrUniqueViolation {
			return auth.UserRoleAssignment{}, auth.ErrConflict
		}
		return auth.UserRoleAssignment{}, err
	}

	if err := tx.Commit(); err != nil {
		return auth.UserRoleAssignment{}, err
	}
	return assignment, nil
}

func (s *Store) RemoveRoleAssignment(ctx context.Context, userID, roleID string) error {
	if s.db == nil {
		return errors.New("database connection unavailable")
	}
	res, err := s.db.ExecContext(ctx, `
		delete from user_roles
		where user_id = $1 and role_id = $2
	`, userID, roleID)
	if err != nil {
		return err
	}
	aff, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if aff == 0 {
		return auth.ErrNotFound
	}
	return nil
}

func (s *Store) ListRoleAssignments(ctx context.Context, userID string) ([]auth.UserRoleAssignment, error) {
	if s.db == nil {
		return nil, errors.New("database connection unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		select user_id, role_id, organization_id, created_at
		from user_roles
		where user_id = $1
		order by role_id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var assignments []auth.UserRoleAssignment
	for rows.Next() {
		var a auth.UserRoleAssignment
		if err := rows.Scan(&a.UserID, &a.RoleID, &a.OrganizationID, &a.CreatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return assignments, nil
}

func (s *Store) UserPermissions(ctx context.Context, userID string) ([]string, error) {
	if s.db == nil {
		return nil, errors.New("database connection unavailable")
	}
	rows, err := s.db.QueryContext(ctx, `
		select distinct p.key
		from user_roles ur
		join role_permissions rp on rp.role_id = ur.role_id
		join permissions p on p.id = rp.permission_id
		where ur.user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		perms = append(perms, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return perms, nil
}

func maybePgError(err error) (*pgconn.PgError, bool) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr, true
	}
	return nil, false
}

func nullIfEmpty(s string) sql.NullString {
	s = strings.TrimSpace(s)
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func (s *Store) userByID(ctx context.Context, userID string) (auth.User, error) {
	var user auth.User
	err := s.db.QueryRowContext(ctx, `
		select id, organization_id, email, status, created_at, updated_at
		from users
		where id = $1
	`, userID).Scan(&user.ID, &user.OrganizationID, &user.Email, &user.Status, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, auth.ErrNotFound
	}
	if err != nil {
		return auth.User{}, err
	}
	return user, nil
}
