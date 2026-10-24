package kafka

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	portmocks "http-api-template/internal/mocks/port"
)

var errUnavailable = errors.New("dependency unavailable")

type consumerFixture struct {
	consumer  *Consumer
	reader    *MockMessageReader
	processor *portmocks.MockDummyProcessor
	ids       *portmocks.MockIDGenerator
	logger    *portmocks.MockLogger
	ctx       context.Context
	cancel    context.CancelFunc
}

func newConsumerFixture(t *testing.T) *consumerFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := &consumerFixture{reader: NewMockMessageReader(t), processor: portmocks.NewMockDummyProcessor(t), ids: portmocks.NewMockIDGenerator(t), logger: portmocks.NewMockLogger(t), ctx: ctx, cancel: cancel}
	f.consumer = NewConsumer(f.reader, f.processor, f.logger, f.ids)
	return f
}

func (f *consumerFixture) end() *mock.Call {
	return f.reader.EXPECT().FetchMessage(f.ctx).RunAndReturn(func(ctx context.Context) (kafka.Message, error) {
		f.cancel()
		return kafka.Message{}, ctx.Err()
	}).Once()
}

func kafkaMessage(data string, offset int64) kafka.Message {
	return kafka.Message{Value: []byte(data), Offset: offset, Headers: []kafka.Header{{Key: RequestIDHeader, Value: []byte(incomingID)}}}
}
func correlatedContext(id string) interface{} {
	return mock.MatchedBy(func(ctx context.Context) bool { return requestid.ID(ctx) == id })
}
func TestConsumerDispatch(t *testing.T) {
	f := newConsumerFixture(t)
	create, update, remove := kafkaMessage(createJSON, 1), kafkaMessage(updateJSON, 2), kafkaMessage(deleteJSON, 3)
	empty := ""
	ctx := correlatedContext(incomingID)
	decoded, err := decodeEvent(create.Value)
	if err != nil {
		t.Fatal(err)
	}
	mock.InOrder(
		f.reader.EXPECT().FetchMessage(f.ctx).Return(create, nil).Once(),
		f.processor.EXPECT().ProcessCreate(ctx, decoded.dummy).Return(nil).Once(),
		f.reader.EXPECT().CommitMessages(ctx, create).Return(nil).Once(),
		f.reader.EXPECT().FetchMessage(f.ctx).Return(update, nil).Once(),
		f.processor.EXPECT().ProcessUpdate(ctx, testDummyID, domain.Patch{Note: &empty}).Return(nil).Once(),
		f.reader.EXPECT().CommitMessages(ctx, update).Return(nil).Once(),
		f.reader.EXPECT().FetchMessage(f.ctx).Return(remove, nil).Once(),
		f.processor.EXPECT().ProcessDelete(ctx, testDummyID).Return(nil).Once(),
		f.reader.EXPECT().CommitMessages(ctx, remove).Return(nil).Once(),
		f.end(),
		f.reader.EXPECT().Close().Return(nil).Once(),
	)
	if err := f.consumer.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
}
func TestConsumerRetries(t *testing.T) {
	f := newConsumerFixture(t)
	message := kafkaMessage(createJSON, 1)
	message.Headers = nil
	decoded, err := decodeEvent(message.Value)
	if err != nil {
		t.Fatal(err)
	}
	ctx := correlatedContext(generatedID)
	mock.InOrder(
		f.reader.EXPECT().FetchMessage(f.ctx).Return(kafka.Message{}, errUnavailable).Once(),
		f.logger.EXPECT().Error(f.ctx, "Kafka read failed").Once(),
		f.reader.EXPECT().FetchMessage(f.ctx).Return(message, nil).Once(),
		f.ids.EXPECT().NewID().Return(generatedID, nil).Once(),
		f.processor.EXPECT().ProcessCreate(ctx, decoded.dummy).Return(errUnavailable).Once(),
		f.logger.EXPECT().Error(ctx, "Kafka processing failed").Once(),
		f.processor.EXPECT().ProcessCreate(ctx, decoded.dummy).Return(nil).Once(),
		f.reader.EXPECT().CommitMessages(ctx, message).Return(errUnavailable).Once(),
		f.logger.EXPECT().Error(ctx, "Kafka commit failed").Once(),
		f.reader.EXPECT().CommitMessages(ctx, message).Return(errUnavailable).Once(),
		f.logger.EXPECT().Error(ctx, "Kafka commit failed").Once(),
		f.reader.EXPECT().CommitMessages(ctx, message).Return(nil).Once(),
		f.end(),
		f.reader.EXPECT().Close().Return(nil).Once(),
	)
	var delays []time.Duration
	f.consumer.wait = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	if err := f.consumer.Run(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(delays, []time.Duration{initialBackoff, initialBackoff, initialBackoff, 2 * initialBackoff}) {
		t.Fatalf("delays=%v", delays)
	}
}
func TestConsumerPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		failure    error
	}{
		{"malformed", "private-invalid-payload", nil},
		{"unknown", `{"event_type":"unknown","dummy_id":"` + testDummyID + `"}`, nil},
		{"null patch", `{"event_type":"update","dummy_id":"` + testDummyID + `","patch":{"note":null}}`, nil},
		{"invalid command", createJSON, fmt.Errorf("processor: %w", domain.ErrInvalid)},
		{"conflict", updateJSON, fmt.Errorf("processor: %w", domain.ErrConflict)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newConsumerFixture(t)
			message := kafkaMessage(tc.data, 1)
			ctx := correlatedContext(incomingID)
			calls := []*mock.Call{f.reader.EXPECT().FetchMessage(f.ctx).Return(message, nil).Once()}
			if tc.failure != nil {
				decoded, err := decodeEvent(message.Value)
				if err != nil {
					t.Fatal(err)
				}
				if tc.data == createJSON {
					calls = append(calls, f.processor.EXPECT().ProcessCreate(ctx, decoded.dummy).Return(tc.failure).Once())
				} else {
					calls = append(calls, f.processor.EXPECT().ProcessUpdate(ctx, decoded.id, decoded.patch).Return(tc.failure).Once())
				}
				calls = append(calls, f.logger.EXPECT().Error(ctx, "Kafka event skipped", "reason", "permanent_command_failure", "event_type", decoded.kind).Once())
			} else {
				calls = append(calls, f.logger.EXPECT().Error(ctx, "Kafka event skipped", "reason", "invalid_event").Once())
			}
			calls = append(calls, f.reader.EXPECT().CommitMessages(ctx, message).Return(nil).Once(), f.end(), f.reader.EXPECT().Close().Return(nil).Once())
			mock.InOrder(calls...)
			f.consumer.wait = func(context.Context, time.Duration) error {
				t.Error("permanent errors must not retry")
				return errUnavailable
			}
			if err := f.consumer.Run(f.ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestConsumerReplayAfterUncommittedProcessing(t *testing.T) {
	message := kafkaMessage(deleteJSON, 7)
	for _, replay := range []bool{false, true} {
		t.Run(fmt.Sprintf("replay=%v", replay), func(t *testing.T) {
			f := newConsumerFixture(t)
			ctx := correlatedContext(incomingID)
			calls := []*mock.Call{
				f.reader.EXPECT().FetchMessage(f.ctx).Return(message, nil).Once(),
				f.processor.EXPECT().ProcessDelete(ctx, testDummyID).Return(nil).Once(),
			}
			if replay {
				calls = append(calls, f.reader.EXPECT().CommitMessages(ctx, message).Return(nil).Once(), f.end())
			} else {
				calls = append(calls, f.reader.EXPECT().CommitMessages(ctx, message).Return(errUnavailable).Once(), f.logger.EXPECT().Error(ctx, "Kafka commit failed").Once())
				f.consumer.wait = func(ctx context.Context, _ time.Duration) error { f.cancel(); return ctx.Err() }
			}
			calls = append(calls, f.reader.EXPECT().Close().Return(nil).Once())
			mock.InOrder(calls...)
			if err := f.consumer.Run(f.ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestCancellationAndLifecycleErrors(t *testing.T) {
	t.Run("fetch cancellation closes reader and exits", func(t *testing.T) {
		f := newConsumerFixture(t)
		started := make(chan struct{})
		mock.InOrder(
			f.reader.EXPECT().FetchMessage(f.ctx).RunAndReturn(func(ctx context.Context) (kafka.Message, error) {
				close(started)
				<-ctx.Done()
				return kafka.Message{}, ctx.Err()
			}).Once(),
			f.reader.EXPECT().Close().Return(nil).Once(),
		)
		done := make(chan error, 1)
		go func() { done <- f.consumer.Run(f.ctx) }()
		<-started
		f.cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("consumer did not exit")
		}
	})
	t.Run("processing cancellation does not commit", func(t *testing.T) {
		f := newConsumerFixture(t)
		message := kafkaMessage(createJSON, 1)
		decoded, err := decodeEvent(message.Value)
		if err != nil {
			t.Fatal(err)
		}
		mock.InOrder(
			f.reader.EXPECT().FetchMessage(f.ctx).Return(message, nil).Once(),
			f.processor.EXPECT().ProcessCreate(correlatedContext(incomingID), decoded.dummy).Run(func(context.Context, domain.Dummy) { f.cancel() }).Return(errUnavailable).Once(),
			f.reader.EXPECT().Close().Return(nil).Once(),
		)
		if err := f.consumer.Run(f.ctx); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("correlation failure is returned without commit", func(t *testing.T) {
		f := newConsumerFixture(t)
		message := kafkaMessage(createJSON, 1)
		message.Headers = nil
		mock.InOrder(
			f.reader.EXPECT().FetchMessage(f.ctx).Return(message, nil).Once(),
			f.ids.EXPECT().NewID().Return("", errUnavailable).Once(),
			f.reader.EXPECT().Close().Return(nil).Once(),
		)
		if err := f.consumer.Run(f.ctx); !errors.Is(err, errUnavailable) {
			t.Fatalf("error=%v", err)
		}
	})
	t.Run("close failure is returned", func(t *testing.T) {
		f := newConsumerFixture(t)
		mock.InOrder(f.end(), f.reader.EXPECT().Close().Return(errUnavailable).Once())
		if err := f.consumer.Run(f.ctx); !errors.Is(err, errUnavailable) {
			t.Fatalf("error=%v", err)
		}
	})
}
func TestBackoff(t *testing.T) {
	f := newConsumerFixture(t)
	f.logger.EXPECT().Error(mock.Anything, "retry").Times(9)
	var delays []time.Duration
	f.consumer.wait = func(_ context.Context, delay time.Duration) error { delays = append(delays, delay); return nil }
	attempts := 0
	err := f.consumer.retry(context.Background(), func() error {
		attempts++
		if attempts == 10 {
			return nil
		}
		return errUnavailable
	}, "retry")
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond, 1600 * time.Millisecond, 3200 * time.Millisecond, maxBackoff, maxBackoff, maxBackoff}
	if err != nil || !reflect.DeepEqual(delays, want) {
		t.Fatalf("error=%v delays=%v", err, delays)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitBackoff(ctx, maxBackoff); !errors.Is(err, context.Canceled) {
		t.Fatalf("backoff cancellation error=%v", err)
	}
}
