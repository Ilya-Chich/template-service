package kafka

import (
	"context"
	"errors"
	"github.com/segmentio/kafka-go"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/requestid"
	"strings"
	"testing"
)

const testBroker = "localhost:9092"

type testLog struct {
	messages []string
}

func (l *testLog) Info(_ context.Context, message string, _ ...any) {
	l.messages = append(l.messages, message)
}
func (l *testLog) Error(_ context.Context, message string, _ ...any) {
	l.messages = append(l.messages, message)
}

type fakeWriter struct {
	messages []kafka.Message
	err      error
	closeErr error
}

func (f *fakeWriter) WriteMessages(ctx context.Context, messages ...kafka.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.messages = append(f.messages, messages...)
	return f.err
}
func (f *fakeWriter) Close() error {
	return f.closeErr
}
func TestPublisher(t *testing.T) {
	for _, kind := range []string{eventCreate, eventUpdate, eventDelete} {
		t.Run(kind, func(t *testing.T) {
			writer := &fakeWriter{}
			publisher := NewPublisher(writer, &testLog{}, &fakeIDs{})
			ctx := requestid.WithID(context.Background(), incomingID)
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
			if err != nil || len(writer.messages) != 1 {
				t.Fatalf("error=%v messages=%d", err, len(writer.messages))
			}
			message := writer.messages[0]
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
	writer := &fakeWriter{err: unavailable, closeErr: unavailable}
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
		before := len(writer.messages)
		if err := call(); !errors.Is(err, domain.ErrInvalid) || len(writer.messages) != before {
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
