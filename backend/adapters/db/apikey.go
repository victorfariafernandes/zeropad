package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrAPIKeyNotFound = errors.New("api key not found")

type APIKey struct {
	ID         string
	OwnerID    string
	Name       string
	Restricted bool
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

func (d *DB) CreateAPIKey(ctx context.Context, ownerID, name, keyHash string, restricted bool) (APIKey, error) {
	var k APIKey
	err := d.pool.QueryRow(ctx,
		`INSERT INTO api_keys (owner_id, name, key_hash, restricted)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, owner_id, name, restricted, created_at, revoked_at`,
		ownerID, name, keyHash, restricted,
	).Scan(&k.ID, &k.OwnerID, &k.Name, &k.Restricted, &k.CreatedAt, &k.RevokedAt)
	if err != nil {
		return APIKey{}, fmt.Errorf("insert api key: %w", err)
	}
	return k, nil
}

func (d *DB) ListAPIKeys(ctx context.Context, ownerID string) ([]APIKey, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, owner_id, name, restricted, created_at, revoked_at
		 FROM api_keys WHERE owner_id = $1 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.OwnerID, &k.Name, &k.Restricted, &k.CreatedAt, &k.RevokedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// GetAPIKeyByHash returns the active (non-revoked) key matching keyHash.
func (d *DB) GetAPIKeyByHash(ctx context.Context, keyHash string) (APIKey, error) {
	var k APIKey
	err := d.pool.QueryRow(ctx,
		`SELECT id, owner_id, name, restricted, created_at, revoked_at
		 FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
		keyHash,
	).Scan(&k.ID, &k.OwnerID, &k.Name, &k.Restricted, &k.CreatedAt, &k.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	if err != nil {
		return APIKey{}, fmt.Errorf("get api key by hash: %w", err)
	}
	return k, nil
}

// UpdateAPIKey updates name/restricted for a key owned by ownerID.
// Returns ErrAPIKeyNotFound if no matching row was updated.
func (d *DB) UpdateAPIKey(ctx context.Context, id, ownerID, name string, restricted bool) error {
	res, err := d.pool.Exec(ctx,
		`UPDATE api_keys SET name = $1, restricted = $2 WHERE id = $3 AND owner_id = $4`,
		name, restricted, id, ownerID,
	)
	if err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// RevokeAPIKey sets revoked_at for a key owned by ownerID.
func (d *DB) RevokeAPIKey(ctx context.Context, id, ownerID string) error {
	res, err := d.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id = $1 AND owner_id = $2 AND revoked_at IS NULL`,
		id, ownerID,
	)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}

// AttachRole assigns roleID to apiKeyID. Both must be owned by ownerID.
func (d *DB) AttachRole(ctx context.Context, apiKeyID, roleID, ownerID string) error {
	tag, err := d.pool.Exec(ctx,
		`INSERT INTO api_key_roles (api_key_id, role_id)
		 SELECT $1, $2
		 WHERE EXISTS (SELECT 1 FROM api_keys WHERE id = $1 AND owner_id = $3)
		   AND EXISTS (SELECT 1 FROM roles WHERE id = $2 AND owner_id = $3)
		 ON CONFLICT DO NOTHING`,
		apiKeyID, roleID, ownerID,
	)
	if err != nil {
		return fmt.Errorf("attach role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either already attached, or the key/role doesn't belong to ownerID.
		var exists bool
		if err := d.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM api_key_roles WHERE api_key_id = $1 AND role_id = $2)`,
			apiKeyID, roleID,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check existing attachment: %w", err)
		}
		if !exists {
			return ErrAPIKeyNotFound
		}
	}
	return nil
}

func (d *DB) DetachRole(ctx context.Context, apiKeyID, roleID, ownerID string) error {
	_, err := d.pool.Exec(ctx,
		`DELETE FROM api_key_roles
		 WHERE api_key_id = $1 AND role_id = $2
		   AND api_key_id IN (SELECT id FROM api_keys WHERE owner_id = $3)`,
		apiKeyID, roleID, ownerID,
	)
	if err != nil {
		return fmt.Errorf("detach role: %w", err)
	}
	return nil
}

// RoleIDsForAPIKey returns the role IDs attached to apiKeyID.
func (d *DB) RoleIDsForAPIKey(ctx context.Context, apiKeyID string) ([]string, error) {
	rows, err := d.pool.Query(ctx, `SELECT role_id FROM api_key_roles WHERE api_key_id = $1`, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("list role ids for api key: %w", err)
	}
	return scanRoleIDs(rows)
}

// RoleIDsForOwnedAPIKey returns the role IDs attached to apiKeyID, scoped to
// keys owned by ownerID (returns an empty slice if apiKeyID isn't theirs).
func (d *DB) RoleIDsForOwnedAPIKey(ctx context.Context, apiKeyID, ownerID string) ([]string, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT akr.role_id FROM api_key_roles akr
		 JOIN api_keys k ON k.id = akr.api_key_id
		 WHERE akr.api_key_id = $1 AND k.owner_id = $2`,
		apiKeyID, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list role ids for owned api key: %w", err)
	}
	return scanRoleIDs(rows)
}

func scanRoleIDs(rows pgx.Rows) ([]string, error) {
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan role id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
