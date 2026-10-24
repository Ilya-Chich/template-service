package service

import (
	"context"
	"errors"

	"http-api-template/internal/core/domain"
)

func (s *Dummy) ProcessCreate(ctx context.Context, dummy domain.Dummy) error {
	if err := dummy.Validate(); err != nil {
		return err
	}
	if _, err := s.repository.Get(ctx, dummy.ID); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if _, err := s.repository.GetByName(ctx, dummy.Name); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if err := s.repository.Insert(ctx, dummy); err != nil && !errors.Is(err, domain.ErrConflict) {
		return err
	}
	return nil
}

func (s *Dummy) ProcessUpdate(ctx context.Context, id string, patch domain.Patch) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	if err := patch.Validate(); err != nil {
		return err
	}
	if err := s.repository.Update(ctx, id, patch); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

func (s *Dummy) ProcessDelete(ctx context.Context, id string) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}
