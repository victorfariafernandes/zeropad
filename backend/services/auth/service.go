package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/argon2"

	"zeropad-backend/adapters/db"
	"zeropad-backend/services/email"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUserNotFound = errors.New("user not found")

const emailVerificationTTL = 24 * time.Hour

type Service struct {
	db              *db.DB
	secret          []byte
	mailer          email.Sender
	frontendBaseURL string
}

func NewService(database *db.DB, jwtSecret []byte, mailer email.Sender, frontendBaseURL string) *Service {
	return &Service{db: database, secret: jwtSecret, mailer: mailer, frontendBaseURL: frontendBaseURL}
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
	if user.Email != "" {
		s.sendVerificationEmail(ctx, user.ID, user.Username, user.Email)
	}
	return IssueToken(s.secret, user)
}

// UpdateUsername renames the user and returns a freshly minted token
// reflecting the new username, since old tokens' claims.Username is now
// stale (JWT claims and login lookups are keyed by username).
func (s *Service) UpdateUsername(ctx context.Context, userID, newUsername string) (db.User, string, error) {
	if newUsername == "" {
		return db.User{}, "", fmt.Errorf("username is required")
	}
	if err := s.db.UpdateUsername(ctx, userID, newUsername); err != nil {
		return db.User{}, "", err
	}
	user, ok, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return db.User{}, "", err
	}
	if !ok {
		return db.User{}, "", ErrUserNotFound
	}
	token, err := IssueToken(s.secret, user)
	if err != nil {
		return db.User{}, "", err
	}
	return user, token, nil
}

// UpdateEmail sets a new email, resets verification, and fires a
// verification email if the new email is non-empty.
func (s *Service) UpdateEmail(ctx context.Context, userID, newEmail string) (db.User, error) {
	if err := s.db.UpdateEmail(ctx, userID, newEmail); err != nil {
		return db.User{}, err
	}
	user, ok, err := s.db.GetUserByID(ctx, userID)
	if err != nil {
		return db.User{}, err
	}
	if !ok {
		return db.User{}, ErrUserNotFound
	}
	if newEmail != "" {
		s.sendVerificationEmail(ctx, user.ID, user.Username, newEmail)
	}
	return user, nil
}

// VerifyEmail consumes a verification token.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	_, err := s.db.VerifyEmailToken(ctx, token)
	return err
}

// sendVerificationEmail issues a token and sends the verification email in
// a detached goroutine (not tied to the caller's request context, which
// will be canceled by the time the HTTP handler returns).
func (s *Service) sendVerificationEmail(ctx context.Context, userID, username, emailAddr string) {
	token, err := s.db.CreateEmailVerificationToken(ctx, userID, emailAddr, emailVerificationTTL)
	if err != nil {
		log.Printf("create verification token: %v", err)
		return
	}
	verifyURL := fmt.Sprintf("%s/_/verify-email?token=%s", s.frontendBaseURL, token)
	go func() {
		sendCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.mailer.SendVerificationEmail(sendCtx, emailAddr, username, verifyURL); err != nil {
			log.Printf("send verification email: %v", err)
		}
	}()
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

const argonTime = 3
const argonMemory = 64 * 1024
const argonThreads = 4
const argonKeyLen = 32

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
