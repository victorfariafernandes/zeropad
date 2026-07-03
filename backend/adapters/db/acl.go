package db

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrACLNotFound = errors.New("acl grant not found")

type ACL struct {
	ID          string
	OwnerID     string
	SlugPattern string
	RoleID      string
	CreatedAt   time.Time
}

func (d *DB) CreateACL(ctx context.Context, ownerID, slugPattern, roleID string) (ACL, error) {
	var a ACL
	err := d.pool.QueryRow(ctx,
		`INSERT INTO acl (owner_id, slug_pattern, role_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, owner_id, slug_pattern, role_id, created_at`,
		ownerID, slugPattern, roleID,
	).Scan(&a.ID, &a.OwnerID, &a.SlugPattern, &a.RoleID, &a.CreatedAt)
	if err != nil {
		return ACL{}, fmt.Errorf("insert acl: %w", err)
	}
	return a, nil
}

func (d *DB) ListACL(ctx context.Context, ownerID string) ([]ACL, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, owner_id, slug_pattern, role_id, created_at
		 FROM acl WHERE owner_id = $1 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list acl: %w", err)
	}
	defer rows.Close()

	var out []ACL
	for rows.Next() {
		var a ACL
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.SlugPattern, &a.RoleID, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan acl: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) DeleteACL(ctx context.Context, id, ownerID string) error {
	res, err := d.pool.Exec(ctx, `DELETE FROM acl WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return fmt.Errorf("delete acl: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrACLNotFound
	}
	return nil
}

// ACLGrant is a denormalized (slug_pattern, permissions) pair used by
// services/acl.Check to evaluate whether a role grants access to a slug.
type ACLGrant struct {
	SlugPattern string
	CanRead     bool
	CanWrite    bool
	CanDelete   bool
}

// ACLGrantsForRoles returns every grant made by ownerID on any of roleIDs,
// joined with the role's permissions. The caller matches slug_pattern
// against the requested slug in Go (services/acl.MatchesSlugPattern).
func (d *DB) ACLGrantsForRoles(ctx context.Context, ownerID string, roleIDs []string) ([]ACLGrant, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	rows, err := d.pool.Query(ctx,
		`SELECT a.slug_pattern, r.can_read, r.can_write, r.can_delete
		 FROM acl a
		 JOIN roles r ON r.id = a.role_id
		 WHERE a.owner_id = $1 AND a.role_id = ANY($2)`,
		ownerID, roleIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("query acl grants: %w", err)
	}
	defer rows.Close()

	var out []ACLGrant
	for rows.Next() {
		var g ACLGrant
		if err := rows.Scan(&g.SlugPattern, &g.CanRead, &g.CanWrite, &g.CanDelete); err != nil {
			return nil, fmt.Errorf("scan acl grant: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
