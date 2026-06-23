package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
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
		addr, err := verifyPersonalSign(req.SIWEMessage, req.SIWESignature)
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
	addr, err := verifyPersonalSign(message, signature)
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

// ─── Wallet signature ─────────────────────────────────────────────────────────

// verifyPersonalSign recovers the Ethereum address from an eth_personal_sign
// signature (produced by ethers.js signer.signMessage). Returns the lowercase address.
func verifyPersonalSign(message, signature string) (string, error) {
	// eth_personal_sign hash: keccak256("\x19Ethereum Signed Message:\n" + len + message)
	prefix := fmt.Sprintf("\x19Ethereum Signed Message:\n%d", len(message))
	hash := crypto.Keccak256([]byte(prefix + message))

	sigHex := strings.TrimPrefix(signature, "0x")
	sigBytes, err := hex.DecodeString(sigHex)
	if err != nil {
		return "", fmt.Errorf("decode signature: %w", err)
	}
	if len(sigBytes) != 65 {
		return "", fmt.Errorf("invalid signature length: %d", len(sigBytes))
	}
	// ethers uses recovery id 27/28; go-ethereum expects 0/1
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	pubKey, err := crypto.SigToPub(hash, sigBytes)
	if err != nil {
		return "", fmt.Errorf("recover public key: %w", err)
	}
	return strings.ToLower(crypto.PubkeyToAddress(*pubKey).Hex()), nil
}
