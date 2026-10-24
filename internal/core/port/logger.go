package port

import "context"

type Logger interface {
	Info(context.Context, string, ...any)
	Error(context.Context, string, ...any)
}
