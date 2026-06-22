package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrDuplicateUsername = errors.New("username already taken")
var ErrDuplicateWallet = errors.New("wallet address already registered")
var ErrDuplicateEmail = errors.New("email already registered")

type User struct {
	ID            string
	Username      string
	Email         string // empty if not set
	PasswordHash  string // empty if not set
	WalletAddress string // empty if not set
}

type Passkey struct {
	CredentialID []byte
	PublicKey    []byte
	SignCount    uint32
}

func (d *DB) CreateUser(ctx context.Context, username, email, passwordHash, walletAddress string) (User, error) {
	var emailVal, passwordHashVal, walletVal *string
	if email != "" {
		emailVal = &email
	}
	if passwordHash != "" {
		passwordHashVal = &passwordHash
	}
	if walletAddress != "" {
		walletVal = &walletAddress
	}

	var u User
	err := d.pool.QueryRow(ctx,
		`INSERT INTO users (username, email, password_hash, wallet_address)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,'')`,
		username, emailVal, passwordHashVal, walletVal,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress)
	if err != nil {
		if isDuplicateError(err, "users_username_key") {
			return User{}, ErrDuplicateUsername
		}
		if isDuplicateError(err, "users_email_key") {
			return User{}, ErrDuplicateEmail
		}
		if isDuplicateError(err, "users_wallet_address_key") {
			return User{}, ErrDuplicateWallet
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

func (d *DB) GetUserByUsername(ctx context.Context, username string) (User, bool, error) {
	var u User
	err := d.pool.QueryRow(ctx,
		`SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,'')
		 FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by username: %w", err)
	}
	return u, true, nil
}

func (d *DB) GetUserByWallet(ctx context.Context, walletAddress string) (User, bool, error) {
	var u User
	err := d.pool.QueryRow(ctx,
		`SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,'')
		 FROM users WHERE wallet_address = $1`,
		walletAddress,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by wallet: %w", err)
	}
	return u, true, nil
}

func (d *DB) GetPasskeysByUserID(ctx context.Context, userID string) ([]Passkey, error) {
	rows, err := d.pool.Query(ctx,
		`SELECT credential_id, public_key, sign_count FROM passkeys WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query passkeys: %w", err)
	}
	defer rows.Close()

	var out []Passkey
	for rows.Next() {
		var p Passkey
		if err := rows.Scan(&p.CredentialID, &p.PublicKey, &p.SignCount); err != nil {
			return nil, fmt.Errorf("scan passkey: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) CreatePasskey(ctx context.Context, userID string, credentialID, publicKey []byte) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO passkeys (user_id, credential_id, public_key) VALUES ($1, $2, $3)`,
		userID, credentialID, publicKey,
	)
	if err != nil {
		return fmt.Errorf("insert passkey: %w", err)
	}
	return nil
}

func (d *DB) UpdatePasskeySignCount(ctx context.Context, credentialID []byte, signCount uint32) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE passkeys SET sign_count = $1 WHERE credential_id = $2`,
		signCount, credentialID,
	)
	if err != nil {
		return fmt.Errorf("update passkey sign count: %w", err)
	}
	return nil
}

func (d *DB) HasPasskey(ctx context.Context, userID string) (bool, error) {
	var count int
	err := d.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM passkeys WHERE user_id = $1`, userID,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("count passkeys: %w", err)
	}
	return count > 0, nil
}

func isDuplicateError(err error, constraintName string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") && strings.Contains(msg, constraintName)
}
