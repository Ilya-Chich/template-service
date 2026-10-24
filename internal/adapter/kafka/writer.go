package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"

	"github.com/segmentio/kafka-go"
)

type MessageWriter interface {
	WriteMessages(context.Context, ...kafka.Message) error
	Close() error
}
type Publisher struct {
	writer MessageWriter
	logger port.Logger
	ids    port.IDGenerator
}

var _ port.EventPublisher = (*Publisher)(nil)

func NewWriter(brokers []string, topic string) (*kafka.Writer, error) {
	if len(brokers) == 0 || strings.TrimSpace(topic) == "" {
		return nil, errors.New("kafka brokers and topic are required")
	}
	for _, broker := range brokers {
		if strings.TrimSpace(broker) == "" {
			return nil, errors.New("kafka broker is empty")
		}
	}
	return &kafka.Writer{
		Addr: kafka.TCP(brokers...), Topic: topic,
		Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false,
	}, nil
}

func NewPublisher(writer MessageWriter, logger port.Logger, ids port.IDGenerator) *Publisher {
	return &Publisher{writer: writer, logger: logger, ids: ids}
}

func (p *Publisher) Close() error {
	return p.writer.Close()
}

func (p *Publisher) PublishCreate(ctx context.Context, dummy domain.Dummy) error {
	if err := dummy.Validate(); err != nil {
		return err
	}
	dummy.ID = strings.ToLower(dummy.ID)
	payload, err := json.Marshal(dummyPayload{ID: dummy.ID, Name: dummy.Name, Note: dummy.Note})
	if err != nil {
		return err
	}
	return p.publish(ctx, envelope{EventType: eventCreate, DummyID: dummy.ID, Dummy: payload})
}

func (p *Publisher) PublishUpdate(ctx context.Context, id string, patch domain.Patch) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	if err := patch.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(patchPayload{Name: patch.Name, Note: patch.Note})
	if err != nil {
		return err
	}
	return p.publish(ctx, envelope{EventType: eventUpdate, DummyID: strings.ToLower(id), Patch: payload})
}

func (p *Publisher) PublishDelete(ctx context.Context, id string) error {
	if err := domain.ValidateID(id); err != nil {
		return err
	}
	return p.publish(ctx, envelope{EventType: eventDelete, DummyID: strings.ToLower(id)})
}

func (p *Publisher) publish(ctx context.Context, wire envelope) error {
	data, err := json.Marshal(wire)
	if err != nil {
		return err
	}
	message := kafka.Message{Key: []byte(wire.DummyID), Value: data}
	ctx, err = SetMessageRequestID(ctx, p.ids, &message)
	if err != nil {
		return err
	}
	if err := p.writer.WriteMessages(ctx, message); err != nil {
		p.logger.Error(ctx, "Kafka publication failed", "event_type", wire.EventType)
		return err
	}
	p.logger.Info(ctx, "Kafka event published", "event_type", wire.EventType)
	return nil
}
