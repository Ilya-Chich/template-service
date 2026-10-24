package kafka

import (
	"context"

	"http-api-template/internal/core/port"
	"http-api-template/internal/core/requestid"

	"github.com/segmentio/kafka-go"
)

const RequestIDHeader = "request_id"

func SetMessageRequestID(ctx context.Context, ids port.IDGenerator, message *kafka.Message) (context.Context, error) {
	ctx, err := requestid.Ensure(ctx, ids)
	if err != nil {
		return ctx, err
	}
	headers := make([]kafka.Header, 0, len(message.Headers)+1)
	for _, header := range message.Headers {
		if header.Key != RequestIDHeader {
			headers = append(headers, header)
		}
	}
	headers = append(headers, kafka.Header{Key: RequestIDHeader, Value: []byte(requestid.ID(ctx))})
	message.Headers = headers
	return ctx, nil
}

func MessageContext(parent context.Context, message kafka.Message, ids port.IDGenerator) (context.Context, error) {
	id, count := "", 0
	for _, header := range message.Headers {
		if header.Key == RequestIDHeader {
			id = string(header.Value)
			count++
		}
	}
	if count != 1 || !requestid.Valid(id) {
		id = ""
	}
	return requestid.Ensure(requestid.WithID(parent, id), ids)
}
