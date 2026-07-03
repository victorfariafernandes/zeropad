package db

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/001_users.sql
var migration001 string

//go:embed migrations/002_auth.sql
var migration002 string

//go:embed migrations/003_email_verification.sql
var migration003 string

//go:embed migrations/004_pad_meta.sql
var migration004 string

//go:embed migrations/005_api_keys.sql
var migration005 string

//go:embed migrations/006_roles_acl.sql
var migration006 string

//go:embed migrations/007_api_usage.sql
var migration007 string

//go:embed migrations/008_users_tier.sql
var migration008 string

type DB struct {
	pool *pgxpool.Pool
}

func Init(ctx context.Context) (*DB, error) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		return nil, fmt.Errorf("POSTGRES_URL not set")
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	d := &DB{pool: pool}
	if err := d.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	log.Println("database connected")
	return d, nil
}

func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

func (d *DB) Close() {
	d.pool.Close()
}

func (d *DB) migrate(ctx context.Context) error {
	for _, m := range []string{
		migration001, migration002, migration003,
		migration004, migration005, migration006, migration007, migration008,
	} {
		if _, err := d.pool.Exec(ctx, m); err != nil {
			return err
		}
	}
	return nil
}
