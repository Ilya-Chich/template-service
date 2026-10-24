package kafka

import (
	"context"
	"errors"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	portmocks "http-api-template/internal/mocks/port"
	"strings"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/mock"
)

const testBroker = "localhost:9092"

var (
	_ MessageReader = (*MockMessageReader)(nil)
	_ MessageWriter = (*MockMessageWriter)(nil)
)

type testLog struct {
	messages []string
}

func (l *testLog) Info(_ context.Context, message string, _ ...any) {
	l.messages = append(l.messages, message)
}
func (l *testLog) Error(_ context.Context, message string, _ ...any) {
	l.messages = append(l.messages, message)
}

func TestPublisher(t *testing.T) {
	for _, kind := range []string{eventCreate, eventUpdate, eventDelete} {
		t.Run(kind, func(t *testing.T) {
			writer := NewMockMessageWriter(t)
			logger := portmocks.NewMockLogger(t)
			publisher := NewPublisher(writer, logger, &fakeIDs{})
			ctx := requestid.WithID(context.Background(), incomingID)
			var message kafka.Message
			writer.EXPECT().WriteMessages(ctx, mock.Anything).Run(func(_ context.Context, messages ...kafka.Message) {
				message = messages[0]
			}).Return(nil).Once()
			logger.EXPECT().Info(ctx, "Kafka event published", "event_type", kind).Once()
			id := strings.ToUpper(testDummyID)
			var err error
			switch kind {
			case eventCreate:
				err = publisher.PublishCreate(ctx, domain.Dummy{ID: id, Name: "Example", Note: "note"})
			case eventUpdate:
				note := ""
				err = publisher.PublishUpdate(ctx, id, domain.Patch{Note: &note})
			case eventDelete:
				err = publisher.PublishDelete(ctx, id)
			}
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodeEvent(message.Value)
			if err != nil || decoded.kind != kind || string(message.Key) != testDummyID {
				t.Fatal("message codec or partition key failed")
			}
			messageCtx, err := MessageContext(context.Background(), message, &fakeIDs{})
			if err != nil || requestid.ID(messageCtx) != incomingID {
				t.Fatal("correlation header was not published")
			}
		})
	}
}
func TestPublisherFailures(t *testing.T) {
	unavailable := errors.New("broker unavailable")
	writer := NewMockMessageWriter(t)
	writer.EXPECT().WriteMessages(mock.MatchedBy(func(ctx context.Context) bool { return requestid.ID(ctx) == generatedID }), mock.Anything).Return(unavailable).Once()
	writer.EXPECT().Close().Return(unavailable).Once()
	publisher := NewPublisher(writer, &testLog{}, &fakeIDs{})
	if err := publisher.PublishDelete(context.Background(), testDummyID); !errors.Is(err, unavailable) {
		t.Fatalf("publication error=%v", err)
	}
	if err := publisher.Close(); !errors.Is(err, unavailable) {
		t.Fatalf("close error=%v", err)
	}
	for _, call := range []func() error{
		func() error { return publisher.PublishCreate(context.Background(), domain.Dummy{ID: testDummyID}) },
		func() error { return publisher.PublishDelete(context.Background(), "invalid") },
		func() error { return publisher.PublishUpdate(context.Background(), testDummyID, domain.Patch{}) },
	} {
		if err := call(); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid input reached the writer")
		}
	}
	ids := &fakeIDs{err: unavailable}
	if err := NewPublisher(writer, &testLog{}, ids).PublishDelete(context.Background(), testDummyID); !errors.Is(err, unavailable) {
		t.Fatal("correlation error was not returned")
	}
}
func TestClientConfiguration(t *testing.T) {
	writer, err := NewWriter([]string{testBroker}, "dummy-events")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := writer.Balancer.(*kafka.Hash); !ok || writer.RequiredAcks != kafka.RequireAll || writer.Async {
		t.Fatal("writes must use a hash balancer and synchronous all-replica acknowledgments")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	for _, brokers := range [][]string{nil, {""}} {
		if _, err := NewWriter(brokers, "dummy-events"); err == nil {
			t.Fatal("invalid brokers accepted")
		}
	}
	if _, err := NewWriter([]string{testBroker}, ""); err == nil {
		t.Fatal("missing topic accepted")
	}
	cfg, err := readerConfig([]string{testBroker}, "dummy-events", "test-group")
	if err != nil || cfg.GroupID != "test-group" || cfg.CommitInterval != 0 {
		t.Fatal("consumer configuration must use its configured group and explicit commits")
	}
	if _, err := readerConfig(nil, "dummy-events", "test-group"); err == nil {
		t.Fatal("missing reader brokers accepted")
	}
	if _, err := readerConfig([]string{testBroker}, "dummy-events", ""); err == nil {
		t.Fatal("missing group accepted")
	}
}
