package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultMigrationsTable = "schema_migrations"
	defaultSeedsTable      = "schema_seeds"
)

// Manager executes SQL migrations and seed files stored on disk.
type Manager struct {
	db              *sql.DB
	migrationsDir   string
	seedsDir        string
	migrationsTable string
	seedsTable      string
}

// Option configures Manager.
type Option func(*Manager)

// WithMigrationsTable overrides the default migrations bookkeeping table.
func WithMigrationsTable(name string) Option {
	return func(m *Manager) {
		if name != "" {
			m.migrationsTable = name
		}
	}
}

// WithSeedsTable overrides the default seeds bookkeeping table.
func WithSeedsTable(name string) Option {
	return func(m *Manager) {
		if name != "" {
			m.seedsTable = name
		}
	}
}

// NewManager constructs a Manager.
func NewManager(db *sql.DB, migrationsDir, seedsDir string, opts ...Option) *Manager {
	m := &Manager{
		db:              db,
		migrationsDir:   migrationsDir,
		seedsDir:        seedsDir,
		migrationsTable: defaultMigrationsTable,
		seedsTable:      defaultSeedsTable,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Up applies all pending migrations.
func (m *Manager) Up(ctx context.Context) error {
	if err := m.ensureTables(ctx); err != nil {
		return err
	}
	executed, err := m.listExecuted(ctx, m.migrationsTable)
	if err != nil {
		return err
	}
	files, err := collectSQL(m.migrationsDir, ".up.sql")
	if err != nil {
		return err
	}
	for _, mig := range files {
		if executed[mig.Base] {
			continue
		}
		if err := m.exec(ctx, mig.Path); err != nil {
			return fmt.Errorf("apply migration %s: %w", mig.Base, err)
		}
		if err := m.insertRecord(ctx, m.migrationsTable, mig.Base); err != nil {
			return err
		}
	}
	return nil
}

// Down rolls back the most recent applied migration.
func (m *Manager) Down(ctx context.Context) error {
	if err := m.ensureTables(ctx); err != nil {
		return err
	}
	executed, err := m.history(ctx, m.migrationsTable)
	if err != nil {
		return err
	}
	if len(executed) == 0 {
		return errors.New("no migrations applied")
	}
	last := executed[len(executed)-1]
	downPath := strings.TrimSuffix(filepath.Join(m.migrationsDir, last), ".up.sql") + ".down.sql"
	if _, err := os.Stat(downPath); err != nil {
		return fmt.Errorf("missing down migration for %s", last)
	}
	if err := m.exec(ctx, downPath); err != nil {
		return fmt.Errorf("rollback migration %s: %w", last, err)
	}
	if _, err := m.db.ExecContext(ctx, fmt.Sprintf(`delete from %s where name = $1`, m.migrationsTable), last); err != nil {
		return err
	}
	return nil
}

// Status returns ordered applied migrations.
func (m *Manager) Status(ctx context.Context) ([]string, error) {
	if err := m.ensureTables(ctx); err != nil {
		return nil, err
	}
	return m.history(ctx, m.migrationsTable)
}

// Seed applies seed files idempotently.
func (m *Manager) Seed(ctx context.Context) error {
	if err := m.ensureTables(ctx); err != nil {
		return err
	}
	executed, err := m.listExecuted(ctx, m.seedsTable)
	if err != nil {
		return err
	}
	files, err := collectSQL(m.seedsDir, ".sql")
	if err != nil {
		return err
	}
	for _, seed := range files {
		if executed[seed.Base] {
			continue
		}
		if err := m.exec(ctx, seed.Path); err != nil {
			return fmt.Errorf("apply seed %s: %w", seed.Base, err)
		}
		if err := m.insertRecord(ctx, m.seedsTable, seed.Base); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ensureTables(ctx context.Context) error {
	ddl := fmt.Sprintf(`
		create table if not exists %s (
			name text primary key,
			applied_at timestamptz not null default now()
		);`, m.migrationsTable)
	if _, err := m.db.ExecContext(ctx, ddl); err != nil {
		return err
	}
	seedDDL := fmt.Sprintf(`
		create table if not exists %s (
			name text primary key,
			applied_at timestamptz not null default now()
		);`, m.seedsTable)
	_, err := m.db.ExecContext(ctx, seedDDL)
	return err
}

func (m *Manager) exec(ctx context.Context, path string) error {
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	statements := splitStatements(string(sqlBytes))
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range statements {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *Manager) insertRecord(ctx context.Context, table, name string) error {
	_, err := m.db.ExecContext(ctx, fmt.Sprintf(`insert into %s(name, applied_at) values ($1, $2)`, table),
		name, time.Now().UTC())
	return err
}

func (m *Manager) listExecuted(ctx context.Context, table string) (map[string]bool, error) {
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf(`select name from %s`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result[name] = true
	}
	return result, nil
}

func (m *Manager) history(ctx context.Context, table string) ([]string, error) {
	rows, err := m.db.QueryContext(ctx, fmt.Sprintf(`select name from %s order by applied_at asc`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		res = append(res, name)
	}
	return res, nil
}

type sqlFile struct {
	Base string
	Path string
}

func collectSQL(dir, suffix string) ([]sqlFile, error) {
	if dir == "" {
		return nil, nil
	}
	var files []sqlFile
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), suffix) {
			files = append(files, sqlFile{
				Base: d.Name(),
				Path: path,
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].Base < files[j].Base
	})
	return files, nil
}

// splitStatements naively splits SQL by semicolon while preserving simple cases.
func splitStatements(sql string) []string {
	var stmts []string
	var current strings.Builder
	var inString bool
	for _, r := range sql {
		switch r {
		case '\'':
			current.WriteRune(r)
			if !inString {
				inString = true
			} else {
				inString = false
			}
		case ';':
			current.WriteRune(r)
			if !inString {
				stmts = append(stmts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if strings.TrimSpace(current.String()) != "" {
		stmts = append(stmts, current.String())
	}
	return stmts
}
