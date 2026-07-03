package acl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zeropad-backend/adapters/db"
)

var ErrInvalidSlugPattern = errors.New("invalid slug pattern")

type Action string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionDelete Action = "delete"
)

type Service struct {
	db *db.DB
}

func New(database *db.DB) *Service {
	return &Service{db: database}
}

// ValidateSlugPattern accepts an exact slug ("notes") or a single trailing
// wildcard ("team/eng/*"). Mid-path or multi-segment wildcards are rejected.
func ValidateSlugPattern(pattern string) error {
	if pattern == "" {
		return fmt.Errorf("%w: empty", ErrInvalidSlugPattern)
	}
	if !strings.Contains(pattern, "*") {
		return nil
	}
	if strings.Count(pattern, "*") != 1 || !strings.HasSuffix(pattern, "/*") {
		return fmt.Errorf("%w: wildcard must be a single trailing \"/*\"", ErrInvalidSlugPattern)
	}
	return nil
}

// MatchesSlugPattern reports whether slug is covered by pattern: an exact
// string match, or (if pattern ends in "/*") a prefix match on everything
// before the wildcard.
func MatchesSlugPattern(pattern, slug string) bool {
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(slug, pattern[:len(pattern)-1])
	}
	return pattern == slug
}

func (s *Service) Grant(ctx context.Context, ownerID, slugPattern, roleID string) (db.ACL, error) {
	if err := ValidateSlugPattern(slugPattern); err != nil {
		return db.ACL{}, err
	}
	return s.db.CreateACL(ctx, ownerID, slugPattern, roleID)
}

func (s *Service) List(ctx context.Context, ownerID string) ([]db.ACL, error) {
	return s.db.ListACL(ctx, ownerID)
}

func (s *Service) Revoke(ctx context.Context, id, ownerID string) error {
	return s.db.DeleteACL(ctx, id, ownerID)
}

// Check reports whether an API key may perform action on slug, whose pad is
// owned by padOwnerID (per pad_meta). See docs/roadmap.md and the API-access
// plan for the two-step rule this implements:
//  1. Unrestricted keys get full access to pads their own account owns.
//  2. Otherwise, access requires a role attached to the key with a matching
//     ACL grant made by the pad's actual owner.
func (s *Service) Check(
	ctx context.Context,
	padOwnerID, keyOwnerID string,
	keyRestricted bool,
	keyRoleIDs []string,
	slug string,
	action Action,
) (bool, error) {
	if !keyRestricted && padOwnerID == keyOwnerID {
		return true, nil
	}
	if len(keyRoleIDs) == 0 {
		return false, nil
	}
	grants, err := s.db.ACLGrantsForRoles(ctx, padOwnerID, keyRoleIDs)
	if err != nil {
		return false, err
	}
	return decide(grants, slug, action), nil
}

// decide is the pure decision core of Check, factored out so it can be
// exercised with hand-built grants in tests without a live database.
func decide(grants []db.ACLGrant, slug string, action Action) bool {
	for _, g := range grants {
		if !MatchesSlugPattern(g.SlugPattern, slug) {
			continue
		}
		if allows(g, action) {
			return true
		}
	}
	return false
}

func allows(g db.ACLGrant, action Action) bool {
	switch action {
	case ActionRead:
		return g.CanRead
	case ActionWrite:
		return g.CanWrite
	case ActionDelete:
		return g.CanDelete
	default:
		return false
	}
}
