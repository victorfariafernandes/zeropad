package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Usage struct {
	OwnerID      string
	PeriodStart  time.Time
	RequestCount int64
	BytesIn      int64
	BytesOut     int64
}

// RecordUsage adds one request and bytesIn/bytesOut to today's (UTC) bucket
// for ownerID, creating the row if it doesn't exist yet.
func (d *DB) RecordUsage(ctx context.Context, ownerID string, bytesIn, bytesOut int64) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	_, err := d.pool.Exec(ctx,
		`INSERT INTO api_usage (owner_id, period_start, request_count, bytes_in, bytes_out)
		 VALUES ($1, $2, 1, $3, $4)
		 ON CONFLICT (owner_id, period_start) DO UPDATE
		 SET request_count = api_usage.request_count + 1,
		     bytes_in      = api_usage.bytes_in + EXCLUDED.bytes_in,
		     bytes_out     = api_usage.bytes_out + EXCLUDED.bytes_out`,
		ownerID, today, bytesIn, bytesOut,
	)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

// GetUsageToday returns today's (UTC) usage bucket for ownerID, zero-valued
// if no requests have been made yet today.
func (d *DB) GetUsageToday(ctx context.Context, ownerID string) (Usage, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	u := Usage{OwnerID: ownerID, PeriodStart: today}
	err := d.pool.QueryRow(ctx,
		`SELECT request_count, bytes_in, bytes_out FROM api_usage WHERE owner_id = $1 AND period_start = $2`,
		ownerID, today,
	).Scan(&u.RequestCount, &u.BytesIn, &u.BytesOut)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil
	}
	if err != nil {
		return Usage{}, fmt.Errorf("get usage today: %w", err)
	}
	return u, nil
}
