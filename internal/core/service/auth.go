package service

import (
	"context"
	"fmt"
	"net/mail"

	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"
)

type Auth struct {
	issuer port.TokenIssuer
}

var _ port.AuthService = (*Auth)(nil)

func NewAuth(issuer port.TokenIssuer) *Auth {
	return &Auth{issuer: issuer}
}

func (s *Auth) Login(ctx context.Context, email, password string) (domain.Token, error) {
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || password == "" {
		return domain.Token{}, fmt.Errorf("%w: valid email and nonempty password are required", domain.ErrInvalid)
	}
	return s.issuer.Issue(ctx, email)
}
