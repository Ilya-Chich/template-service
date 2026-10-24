package service

import (
	"context"
	"errors"
	"fmt"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"
)

type Dummy struct {
	repository port.DummyRepository
	publisher  port.EventPublisher
	ids        port.IDGenerator
}

var _ port.DummyService = (*Dummy)(nil)
var _ port.DummyProcessor = (*Dummy)(nil)

func NewDummy(repository port.DummyRepository, publisher port.EventPublisher, ids port.IDGenerator) *Dummy {
	return &Dummy{repository: repository, publisher: publisher, ids: ids}
}
func (s *Dummy) Create(ctx context.Context, name, note string) (string, error) {
	if err := domain.ValidateName(name); err != nil {
		return "", err
	}
	if _, err := s.repository.GetByName(ctx, name); err == nil {
		return "", domain.ErrConflict
	} else if !errors.Is(err, domain.ErrNotFound) {
		return "", fmt.Errorf("check dummy name: %w", err)
	}
	id, err := s.ids.NewID()
	if err != nil {
		return "", fmt.Errorf("generate dummy ID: %w", err)
	}
	if err := domain.ValidateID(id); err != nil {
		return "", fmt.Errorf("ID generator returned a malformed UUID")
	}
	if err := s.publisher.PublishCreate(ctx, domain.Dummy{ID: id, Name: name, Note: note}); err != nil {
		return "", fmt.Errorf("publish create: %w", err)
	}
	return id, nil
}
func (s *Dummy) Get(ctx context.Context, id string) (domain.Dummy, error) {
	if err := domain.ValidateID(id); err != nil {
		return domain.Dummy{}, err
	}
	return s.repository.Get(ctx, id)
}
func (s *Dummy) Patch(ctx context.Context, id string, patch domain.Patch) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	if err := patch.Validate(); err != nil {
		return err
	}
	return s.publisher.PublishUpdate(ctx, id, patch)
}
func (s *Dummy) Delete(ctx context.Context, id string) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	return s.publisher.PublishDelete(ctx, id)
}
