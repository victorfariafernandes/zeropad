package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrRoleNotFound = errors.New("role not found")
var ErrDuplicateRoleName = errors.New("role name already exists")

type Role struct {
	ID        string
	OwnerID   string
	Name      string
	CanRead   bool
	CanWrite  bool
	CanDelete bool
	CreatedAt time.Time
}

func (d *DB) CreateRole(ctx context.Context, ownerID, name string, canRead, canWrite, canDelete bool) (Role, error) {
	var role Role
	err := d.pool.QueryRow(ctx,
		`INSERT INTO roles (owner_id, name, can_read, can_write, can_delete)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, owner_id, name, can_read, can_write, can_delete, created_at`,
		ownerID, name, canRead, canWrite, canDelete,
	).Scan(&role.ID, &role.OwnerID, &role.Name, &role.CanRead, &role.CanWrite, &role.CanDelete, &role.CreatedAt)
	if err != nil {
		if isDuplicateError(err, "roles_owner_id_name_key") {
			return Role{}, ErrDuplicateRoleName
		}
		return Role{}, fmt.Errorf("insert role: %w", err)
	}
	return role, nil
}

func (d *DB) ListRoles(ctx context.Context, ownerID string) ([]Role, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT id, owner_id, name, can_read, can_write, can_delete, created_at
		 FROM roles WHERE owner_id = $1 ORDER BY created_at DESC`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.OwnerID, &role.Name, &role.CanRead, &role.CanWrite, &role.CanDelete, &role.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		out = append(out, role)
	}
	return out, rows.Err()
}

func (d *DB) GetRole(ctx context.Context, id string) (Role, error) {
	var role Role
	err := d.pool.QueryRow(ctx,
		`SELECT id, owner_id, name, can_read, can_write, can_delete, created_at FROM roles WHERE id = $1`,
		id,
	).Scan(&role.ID, &role.OwnerID, &role.Name, &role.CanRead, &role.CanWrite, &role.CanDelete, &role.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Role{}, ErrRoleNotFound
	}
	if err != nil {
		return Role{}, fmt.Errorf("get role: %w", err)
	}
	return role, nil
}

func (d *DB) UpdateRole(ctx context.Context, id, ownerID, name string, canRead, canWrite, canDelete bool) error {
	res, err := d.pool.Exec(ctx,
		`UPDATE roles SET name = $1, can_read = $2, can_write = $3, can_delete = $4
		 WHERE id = $5 AND owner_id = $6`,
		name, canRead, canWrite, canDelete, id, ownerID,
	)
	if err != nil {
		if isDuplicateError(err, "roles_owner_id_name_key") {
			return ErrDuplicateRoleName
		}
		return fmt.Errorf("update role: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrRoleNotFound
	}
	return nil
}

func (d *DB) DeleteRole(ctx context.Context, id, ownerID string) error {
	res, err := d.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if res.RowsAffected() == 0 {
		return ErrRoleNotFound
	}
	return nil
}
