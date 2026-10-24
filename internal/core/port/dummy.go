package port

import (
	"context"
	"http-api-template/internal/core/domain"
)

type DummyRepository interface {
	Get(context.Context, string) (domain.Dummy, error)
	GetByName(context.Context, string) (domain.Dummy, error)
	Insert(context.Context, domain.Dummy) error
	Update(context.Context, string, domain.Patch) error
	Delete(context.Context, string) error
}
type EventPublisher interface {
	PublishCreate(context.Context, domain.Dummy) error
	PublishUpdate(context.Context, string, domain.Patch) error
	PublishDelete(context.Context, string) error
}
type IDGenerator interface {
	NewID() (string, error)
}
type DummyService interface {
	Create(context.Context, string, string) (string, error)
	Get(context.Context, string) (domain.Dummy, error)
	Patch(context.Context, string, domain.Patch) error
	Delete(context.Context, string) error
}
type DummyProcessor interface {
	ProcessCreate(context.Context, domain.Dummy) error
	ProcessUpdate(context.Context, string, domain.Patch) error
	ProcessDelete(context.Context, string) error
}
