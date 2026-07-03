package role

import (
	"context"

	"zeropad-backend/adapters/db"
)

type Service struct {
	db *db.DB
}

func New(database *db.DB) *Service {
	return &Service{db: database}
}

func (s *Service) Create(ctx context.Context, ownerID, name string, canRead, canWrite, canDelete bool) (db.Role, error) {
	return s.db.CreateRole(ctx, ownerID, name, canRead, canWrite, canDelete)
}

func (s *Service) List(ctx context.Context, ownerID string) ([]db.Role, error) {
	return s.db.ListRoles(ctx, ownerID)
}

func (s *Service) Update(ctx context.Context, id, ownerID, name string, canRead, canWrite, canDelete bool) error {
	return s.db.UpdateRole(ctx, id, ownerID, name, canRead, canWrite, canDelete)
}

func (s *Service) Delete(ctx context.Context, id, ownerID string) error {
	return s.db.DeleteRole(ctx, id, ownerID)
}
