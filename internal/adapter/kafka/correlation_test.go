package kafka

import (
	"context"
	"errors"
	"testing"

	"http-api-template/internal/core/requestid"
	portmocks "http-api-template/internal/mocks/port"

	"github.com/segmentio/kafka-go"
)

const (
	incomingID  = "incoming-id"
	generatedID = "generated-id"
)

func TestPropagation(t *testing.T) {
	ids := portmocks.NewMockIDGenerator(t)
	ctx := requestid.WithID(context.Background(), incomingID)
	message := kafka.Message{
		Headers: []kafka.Header{{Key: "other", Value: []byte("kept")}, {Key: RequestIDHeader, Value: []byte("old")}},
	}
	publishedCtx, err := SetMessageRequestID(ctx, ids, &message)
	if err != nil || requestid.ID(publishedCtx) != incomingID || len(message.Headers) != 2 ||
		string(message.Headers[0].Value) != "kept" {
		t.Fatal("publication did not preserve the request context and unrelated headers")
	}
	consumedCtx, err := MessageContext(context.Background(), message, ids)
	if err != nil || requestid.ID(consumedCtx) != incomingID {
		t.Fatal("consumption did not preserve the originating request ID")
	}
	for range 3 {
		consumedCtx, err = requestid.Ensure(consumedCtx, ids)
		if err != nil || requestid.ID(consumedCtx) != incomingID {
			t.Fatal("retry correlation changed")
		}
	}
}

func TestExternalMessages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers []kafka.Header
	}{
		{"missing", nil},
		{"invalid", []kafka.Header{{Key: RequestIDHeader, Value: []byte("invalid ID")}}},
		{"duplicate", []kafka.Header{{Key: RequestIDHeader, Value: []byte(incomingID)}, {Key: RequestIDHeader, Value: []byte(incomingID)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := portmocks.NewMockIDGenerator(t)
			ids.EXPECT().NewID().Return(generatedID, nil).Once()
			parent := requestid.WithID(context.Background(), "unrelated-parent")
			ctx, err := MessageContext(parent, kafka.Message{Headers: tc.headers}, ids)
			if err != nil || requestid.ID(ctx) != generatedID {
				t.Fatalf("error=%v", err)
			}
		})
	}
	failure := errors.New("entropy unavailable")
	ids := portmocks.NewMockIDGenerator(t)
	ids.EXPECT().NewID().Return("", failure).Twice()
	if _, err := MessageContext(context.Background(), kafka.Message{}, ids); !errors.Is(err, failure) {
		t.Fatal("consumer generation error was not propagated")
	}
	if _, err := SetMessageRequestID(context.Background(), ids, &kafka.Message{}); !errors.Is(err, failure) {
		t.Fatal("producer generation error was not propagated")
	}
}
