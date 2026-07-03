package pad

import (
	"errors"

	"zeropad-backend/adapters/store"
)

var ErrNotFound = errors.New("pad not found")

type Service struct {
	pads store.PadStore
}

func New(pads store.PadStore) *Service {
	return &Service{pads: pads}
}

func (s *Service) Get(slug string) (store.Pad, error) {
	pad, ok := s.pads.Get(slug)
	if !ok {
		return store.Pad{}, ErrNotFound
	}
	return pad, nil
}

func (s *Service) Set(slug string, pad store.Pad) error {
	s.pads.Set(slug, pad)
	return nil
}

func (s *Service) Delete(slug string) error {
	s.pads.Delete(slug)
	return nil
}
