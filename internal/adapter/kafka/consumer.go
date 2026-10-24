package kafka

import (
	"context"
	"errors"
	"time"

	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"

	"github.com/segmentio/kafka-go"
)

const (
	initialBackoff = 100 * time.Millisecond
	maxBackoff     = 5 * time.Second
)

type MessageReader interface {
	FetchMessage(context.Context) (kafka.Message, error)
	CommitMessages(context.Context, ...kafka.Message) error
	Close() error
}
type Consumer struct {
	reader    MessageReader
	processor port.DummyProcessor
	logger    port.Logger
	ids       port.IDGenerator
	wait      func(context.Context, time.Duration) error
}

func NewReader(brokers []string, topic, group string) (*kafka.Reader, error) {
	cfg, err := readerConfig(brokers, topic, group)
	if err != nil {
		return nil, err
	}
	return kafka.NewReader(cfg), nil
}

func readerConfig(brokers []string, topic, group string) (kafka.ReaderConfig, error) {
	if group == "" {
		return kafka.ReaderConfig{}, errors.New("kafka consumer group is required")
	}
	cfg := kafka.ReaderConfig{
		Brokers: brokers, Topic: topic, GroupID: group,
		MinBytes: 1, MaxBytes: 10e6, CommitInterval: 0,
	}
	if err := cfg.Validate(); err != nil {
		return kafka.ReaderConfig{}, err
	}
	return cfg, nil
}

func NewConsumer(
	reader MessageReader,
	processor port.DummyProcessor,
	logger port.Logger,
	ids port.IDGenerator,
) *Consumer {
	return &Consumer{reader: reader, processor: processor, logger: logger, ids: ids, wait: waitBackoff}
}

func (c *Consumer) Run(ctx context.Context) (err error) {
	defer func() { err = errors.Join(err, c.reader.Close()) }()
	for {
		var message kafka.Message
		err := c.retry(ctx, func() error {
			var fetchErr error
			message, fetchErr = c.reader.FetchMessage(ctx)
			return fetchErr
		}, "Kafka read failed")
		if err != nil {
			return cancellationResult(ctx, err)
		}
		messageCtx, err := MessageContext(ctx, message, c.ids)
		if err != nil {
			return err
		}
		decoded, decodeErr := decodeEvent(message.Value)
		if decodeErr != nil {
			c.logger.Error(messageCtx, "Kafka event skipped", "reason", "invalid_event")
		} else {
			err = c.retry(messageCtx, func() error {
				processErr := c.process(messageCtx, decoded)
				if errors.Is(processErr, domain.ErrInvalid) || errors.Is(processErr, domain.ErrConflict) {
					c.logger.Error(messageCtx, "Kafka event skipped", "reason", "permanent_command_failure", "event_type", decoded.kind)
					return nil
				}
				return processErr
			}, "Kafka processing failed")
			if err != nil {
				return cancellationResult(ctx, err)
			}
		}
		if err := c.retry(messageCtx, func() error {
			return c.reader.CommitMessages(messageCtx, message)
		}, "Kafka commit failed"); err != nil {
			return cancellationResult(ctx, err)
		}
	}
}

func (c *Consumer) process(ctx context.Context, decoded event) error {
	switch decoded.kind {
	case eventCreate:
		return c.processor.ProcessCreate(ctx, decoded.dummy)
	case eventUpdate:
		return c.processor.ProcessUpdate(ctx, decoded.id, decoded.patch)
	default:
		return c.processor.ProcessDelete(ctx, decoded.id)
	}
}

func (c *Consumer) retry(ctx context.Context, operation func() error, message string) error {
	delay := initialBackoff
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := operation(); err == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		c.logger.Error(ctx, message)
		if err := c.wait(ctx, delay); err != nil {
			return err
		}
		delay = min(delay*2, maxBackoff)
	}
}

func waitBackoff(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func cancellationResult(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return nil
	}
	return err
}
