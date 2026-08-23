package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hkjang/muni/internal/cryptoutil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/argon2"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Open(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL DSN: %w", err)
	}
	config.MaxConns = 20
	config.MinConns = 2
	config.MaxConnLifetime = 30 * time.Minute
	config.MaxConnIdleTime = 5 * time.Minute
	config.HealthCheckPeriod = time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func Migrate(ctx context.Context, db *pgxpool.Pool) error {
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, entry.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, entry.Name())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func Bootstrap(ctx context.Context, db *pgxpool.Pool, identity, password string, sealer *cryptoutil.Sealer) error {
	var userCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	username := strings.ToLower(identity)
	email := identity
	if !strings.Contains(identity, "@") {
		email = identity + "@localhost"
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}
	userID := uuid.New()
	workspaceID := uuid.New()
	dataKey, err := cryptoutil.RandomBytes(32)
	if err != nil {
		return err
	}
	wrapped, err := sealer.Seal(dataKey, "user-key:"+userID.String()+":1")
	if err != nil {
		return err
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO users(id, username, email, display_name, password_hash, role, status)
		VALUES($1,$2,$3,$4,$5,'ADMIN','ACTIVE')`, userID, username, strings.ToLower(email), identity, passwordHash)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workspaces(id, name, slug, kind, owner_id) VALUES($1,$2,$3,'PERSONAL',$4)`, workspaceID, identity+"의 문서", "personal-"+userID.String(), userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workspace_members(workspace_id,user_id,role) VALUES($1,$2,'OWNER')`, workspaceID, userID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO user_keys(id,user_id,name,fingerprint,wrapped_key,status,version) VALUES($1,$2,'기본 개인 키',$3,$4,'ACTIVE',1)`, uuid.New(), userID, cryptoutil.Fingerprint(dataKey), wrapped)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO activity_logs(actor_id, action, resource_type, resource_id, metadata) VALUES($1,'BOOTSTRAP_ADMIN','USER',$1,'{}')`, userID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func HashPassword(password string) (string, error) {
	salt, err := cryptoutil.RandomBytes(16)
	if err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=4$%x$%x", salt, hash), nil
}

func VerifyPassword(encoded, password string) bool {
	var salt, expected []byte
	_, err := fmt.Sscanf(encoded, "$argon2id$v=19$m=65536,t=3,p=4$%x$%x", &salt, &expected)
	if err != nil || len(salt) != 16 || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	return subtleEqual(actual, expected)
}

func subtleEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var result byte
	for i := range a {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

func WithTx(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

var ErrNotFound = errors.New("not found")
