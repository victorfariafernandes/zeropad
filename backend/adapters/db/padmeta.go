package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var ErrPadMetaNotFound = errors.New("pad meta not found")

// GetPadOwner returns the owner_id of slug, or ErrPadMetaNotFound if the pad
// has never been claimed via the API (e.g. it's anonymous or UI-only).
func (d *DB) GetPadOwner(ctx context.Context, slug string) (string, error) {
	var ownerID string
	err := d.pool.QueryRow(ctx, `SELECT owner_id FROM pad_meta WHERE slug = $1`, slug).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrPadMetaNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get pad owner: %w", err)
	}
	return ownerID, nil
}

// ClaimPad records ownerID as the owner of slug if it has no owner yet.
// A no-op if the pad is already owned (by anyone).
func (d *DB) ClaimPad(ctx context.Context, slug, ownerID string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO pad_meta (slug, owner_id) VALUES ($1, $2)
		 ON CONFLICT (slug) DO NOTHING`,
		slug, ownerID,
	)
	if err != nil {
		return fmt.Errorf("claim pad: %w", err)
	}
	return nil
}
