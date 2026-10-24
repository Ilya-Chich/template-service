package port

import (
	"context"
	"http-api-template/internal/core/domain"
)

type AuthService interface {
	Login(context.Context, string, string) (domain.Token, error)
}
type TokenIssuer interface {
	Issue(context.Context, string) (domain.Token, error)
}
type TokenVerifier interface {
	Verify(context.Context, string) error
}
