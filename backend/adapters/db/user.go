package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrDuplicateUsername = errors.New("username already taken")
var ErrDuplicateWallet = errors.New("wallet address already registered")
var ErrDuplicateEmail = errors.New("email already registered")
var ErrInvalidToken = errors.New("invalid or expired token")

type User struct {
	ID            string
	Username      string
	Email         string // empty if not set
	PasswordHash  string // empty if not set
	WalletAddress string // empty if not set
	EmailVerified bool
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
		 RETURNING id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,''), email_verified`,
		username, emailVal, passwordHashVal, walletVal,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress, &u.EmailVerified)
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
		`SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,''), email_verified
		 FROM users WHERE username = $1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress, &u.EmailVerified)
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
		`SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,''), email_verified
		 FROM users WHERE wallet_address = $1`,
		walletAddress,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by wallet: %w", err)
	}
	return u, true, nil
}

func (d *DB) GetUserByID(ctx context.Context, id string) (User, bool, error) {
	var u User
	err := d.pool.QueryRow(ctx,
		`SELECT id, username, COALESCE(email,''), COALESCE(password_hash,''), COALESCE(wallet_address,''), email_verified
		 FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.WalletAddress, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user by id: %w", err)
	}
	return u, true, nil
}

// UpdateUsername renames a user. Returns ErrDuplicateUsername if taken.
func (d *DB) UpdateUsername(ctx context.Context, userID, newUsername string) error {
	_, err := d.pool.Exec(ctx,
		`UPDATE users SET username = $1 WHERE id = $2`,
		newUsername, userID,
	)
	if err != nil {
		if isDuplicateError(err, "users_username_key") {
			return ErrDuplicateUsername
		}
		return fmt.Errorf("update username: %w", err)
	}
	return nil
}

// UpdateEmail sets a user's email and resets email_verified to false.
// Passing "" clears the email (NULL); an empty email is a valid state.
func (d *DB) UpdateEmail(ctx context.Context, userID, newEmail string) error {
	var emailVal *string
	if newEmail != "" {
		emailVal = &newEmail
	}
	_, err := d.pool.Exec(ctx,
		`UPDATE users SET email = $1, email_verified = false WHERE id = $2`,
		emailVal, userID,
	)
	if err != nil {
		if isDuplicateError(err, "users_email_key") {
			return ErrDuplicateEmail
		}
		return fmt.Errorf("update email: %w", err)
	}
	return nil
}

// CreateEmailVerificationToken issues a fresh single-use token for the given
// user/email pair, invalidating any previously pending tokens for that user.
func (d *DB) CreateEmailVerificationToken(ctx context.Context, userID, email string, ttl time.Duration) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM email_verification_tokens WHERE user_id = $1`, userID); err != nil {
		return "", fmt.Errorf("invalidate old tokens: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO email_verification_tokens (token, user_id, email, expires_at) VALUES ($1, $2, $3, $4)`,
		token, userID, email, time.Now().Add(ttl),
	); err != nil {
		return "", fmt.Errorf("insert token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return token, nil
}

// VerifyEmailToken marks the associated user's email as verified if the
// token exists, is unexpired, and its stored email still matches the user's
// current email (guards against a stale link after another email change).
// The token is deleted whether or not verification succeeds (single use).
func (d *DB) VerifyEmailToken(ctx context.Context, token string) (userID string, err error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var uid, tokenEmail string
	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT user_id, email, expires_at FROM email_verification_tokens WHERE token = $1`,
		token,
	).Scan(&uid, &tokenEmail, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("lookup token: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM email_verification_tokens WHERE token = $1`, token); err != nil {
		return "", fmt.Errorf("delete token: %w", err)
	}

	if time.Now().After(expiresAt) {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit tx: %w", err)
		}
		return "", ErrInvalidToken
	}

	res, err := tx.Exec(ctx,
		`UPDATE users SET email_verified = true WHERE id = $1 AND email = $2`,
		uid, tokenEmail,
	)
	if err != nil {
		return "", fmt.Errorf("mark verified: %w", err)
	}
	if res.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit tx: %w", err)
		}
		return "", ErrInvalidToken
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return uid, nil
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
