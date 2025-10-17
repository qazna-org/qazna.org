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
