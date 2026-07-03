package apikey

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"zeropad-backend/adapters/db"
)

var ErrTierRequired = errors.New("paid tier required")

type Service struct {
	db *db.DB
}

func New(database *db.DB) *Service {
	return &Service{db: database}
}

// Created carries the one-time-visible raw key alongside the stored record.
type Created struct {
	Key db.APIKey
	Raw string
}

// Create generates a new API key for ownerID. Requires the owner to be on
// the paid tier — API access is a paid feature.
func (s *Service) Create(ctx context.Context, ownerID, name string, restricted bool) (Created, error) {
	user, ok, err := s.db.GetUserByID(ctx, ownerID)
	if err != nil {
		return Created{}, err
	}
	if !ok || user.Tier != "paid" {
		return Created{}, ErrTierRequired
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return Created{}, fmt.Errorf("generate key: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(rawBytes)

	key, err := s.db.CreateAPIKey(ctx, ownerID, name, hash(raw), restricted)
	if err != nil {
		return Created{}, err
	}
	return Created{Key: key, Raw: raw}, nil
}

func (s *Service) List(ctx context.Context, ownerID string) ([]db.APIKey, error) {
	return s.db.ListAPIKeys(ctx, ownerID)
}

func (s *Service) Update(ctx context.Context, id, ownerID, name string, restricted bool) error {
	return s.db.UpdateAPIKey(ctx, id, ownerID, name, restricted)
}

func (s *Service) Revoke(ctx context.Context, id, ownerID string) error {
	return s.db.RevokeAPIKey(ctx, id, ownerID)
}

func (s *Service) AttachRole(ctx context.Context, apiKeyID, roleID, ownerID string) error {
	return s.db.AttachRole(ctx, apiKeyID, roleID, ownerID)
}

func (s *Service) DetachRole(ctx context.Context, apiKeyID, roleID, ownerID string) error {
	return s.db.DetachRole(ctx, apiKeyID, roleID, ownerID)
}

// AttachedRoleIDs returns the role IDs attached to apiKeyID, scoped to keys
// owned by ownerID.
func (s *Service) AttachedRoleIDs(ctx context.Context, apiKeyID, ownerID string) ([]string, error) {
	return s.db.RoleIDsForOwnedAPIKey(ctx, apiKeyID, ownerID)
}

// Authenticate resolves a raw API key (from an Authorization: Bearer header)
// to its record and attached role IDs. Returns db.ErrAPIKeyNotFound if the
// key is unknown or revoked.
func (s *Service) Authenticate(ctx context.Context, raw string) (db.APIKey, []string, error) {
	key, err := s.db.GetAPIKeyByHash(ctx, hash(raw))
	if err != nil {
		return db.APIKey{}, nil, err
	}
	roleIDs, err := s.db.RoleIDsForAPIKey(ctx, key.ID)
	if err != nil {
		return db.APIKey{}, nil, err
	}
	return key, roleIDs, nil
}

func hash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
