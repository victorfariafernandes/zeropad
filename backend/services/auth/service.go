package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	siwe "github.com/spruceid/siwe-go"
	"golang.org/x/crypto/argon2"

	"zeropad-backend/adapters/db"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")

type Service struct {
	db     *db.DB
	secret []byte
}

func NewService(database *db.DB, jwtSecret []byte) *Service {
	return &Service{db: database, secret: jwtSecret}
}

func (s *Service) Secret() []byte { return s.secret }

type SignupRequest struct {
	Username      string
	Email         string
	Password      string
	WalletAddress string
	SIWESignature string
	SIWEMessage   string
}

func (s *Service) Signup(ctx context.Context, req SignupRequest) (string, error) {
	if req.Username == "" {
		return "", fmt.Errorf("username is required")
	}

	var passwordHash, walletAddress string

	switch {
	case req.Password != "":
		h, err := hashPassword(req.Password)
		if err != nil {
			return "", fmt.Errorf("hash password: %w", err)
		}
		passwordHash = h

	case req.WalletAddress != "" && req.SIWESignature != "" && req.SIWEMessage != "":
		addr, err := verifySIWE(req.SIWEMessage, req.SIWESignature)
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
		}
		if !strings.EqualFold(addr, req.WalletAddress) {
			return "", fmt.Errorf("%w: address mismatch", ErrInvalidCredentials)
		}
		walletAddress = strings.ToLower(req.WalletAddress)

	default:
		return "", fmt.Errorf("password or wallet credentials required")
	}

	user, err := s.db.CreateUser(ctx, req.Username, req.Email, passwordHash, walletAddress)
	if err != nil {
		return "", err // db layer returns typed errors (ErrDuplicateUsername etc.)
	}
	return IssueToken(s.secret, user)
}

func (s *Service) LoginPassword(ctx context.Context, username, password string) (string, error) {
	user, ok, err := s.db.GetUserByUsername(ctx, username)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUserNotFound
	}
	if user.PasswordHash == "" {
		return "", ErrInvalidCredentials
	}
	match, err := verifyPassword(password, user.PasswordHash)
	if err != nil || !match {
		return "", ErrInvalidCredentials
	}
	return IssueToken(s.secret, user)
}

func (s *Service) LoginWallet(ctx context.Context, username, walletAddress, signature, message string) (string, error) {
	addr, err := verifySIWE(message, signature)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	if !strings.EqualFold(addr, walletAddress) {
		return "", fmt.Errorf("%w: address mismatch", ErrInvalidCredentials)
	}

	user, ok, err := s.db.GetUserByWallet(ctx, strings.ToLower(walletAddress))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrUserNotFound
	}
	if username != "" && !strings.EqualFold(user.Username, username) {
		return "", ErrInvalidCredentials
	}
	return IssueToken(s.secret, user)
}

// ─── Argon2id ────────────────────────────────────────────────────────────────

const argonTime    = 3
const argonMemory  = 64 * 1024
const argonThreads = 4
const argonKeyLen  = 32

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// expected: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash format")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	storedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}
	computed := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, uint32(len(storedHash)))
	return string(computed) == string(storedHash), nil
}

// ─── SIWE ────────────────────────────────────────────────────────────────────

func verifySIWE(message, signature string) (string, error) {
	msg, err := siwe.ParseMessage(message)
	if err != nil {
		return "", fmt.Errorf("parse siwe message: %w", err)
	}
	// Verify returns the recovered ECDSA public key; convert it to an address.
	pubKey, err := msg.Verify(signature, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("verify signature: %w", err)
	}
	addr := crypto.PubkeyToAddress(*pubKey)
	return strings.ToLower(addr.Hex()), nil
}
