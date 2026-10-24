package logger

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"http-api-template/internal/core/port"
	"http-api-template/internal/core/requestid"
)

type Logger struct {
	log *slog.Logger
	ids port.IDGenerator
}

var _ port.Logger = (*Logger)(nil)

func New(output io.Writer, ids port.IDGenerator) *Logger {
	handler := slog.NewJSONHandler(
		output,
		&slog.HandlerOptions{ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch strings.ToLower(attr.Key) {
			case "password", "jwt_secret", "authorization", "access_token", "payload":
				return slog.String(attr.Key, "[redacted]")
			}
			return attr
		}},
	)
	return &Logger{log: slog.New(handler), ids: ids}
}

func (l *Logger) Info(ctx context.Context, message string, attrs ...any) {
	l.write(ctx, slog.LevelInfo, message, attrs...)
}

func (l *Logger) Error(ctx context.Context, message string, attrs ...any) {
	l.write(ctx, slog.LevelError, message, attrs...)
}

func (l *Logger) write(ctx context.Context, level slog.Level, message string, attrs ...any) {
	ctx, err := requestid.Ensure(ctx, l.ids)
	id := requestid.ID(ctx)
	if err != nil {
		id = "unavailable"
	}
	attrs = append(attrs, "request_id", id)
	l.log.Log(ctx, level, message, attrs...)
}
