package requestid

import (
	"context"
	"errors"

	"http-api-template/internal/core/port"
)

type contextKey struct{}

func Valid(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, ch := range []byte(id) {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' ||
			ch == '-' {
			continue
		}
		return false
	}
	return true
}

func WithID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

func ID(ctx context.Context) string {
	id, _ := ctx.Value(contextKey{}).(string)
	return id
}

func Ensure(ctx context.Context, ids port.IDGenerator) (context.Context, error) {
	if Valid(ID(ctx)) {
		return ctx, nil
	}
	id, err := ids.NewID()
	if err != nil {
		return ctx, err
	}
	if !Valid(id) {
		return ctx, errors.New("request ID generator returned an invalid ID")
	}
	return WithID(ctx, id), nil
}
