package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"

	"github.com/golang-jwt/jwt/v4"
)

type claims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}
type JWT struct {
	key []byte
	now func() time.Time
}

var (
	_ port.TokenIssuer   = (*JWT)(nil)
	_ port.TokenVerifier = (*JWT)(nil)
)

func NewJWT(secret string) (*JWT, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("JWT secret is required")
	}
	return &JWT{key: []byte(secret), now: time.Now}, nil
}

func (a *JWT) Issue(ctx context.Context, email string) (domain.Token, error) {
	if err := ctx.Err(); err != nil {
		return domain.Token{}, err
	}
	now := a.now()
	payload := claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID: "1", Email: email,
	}
	encoded, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString(a.key)
	if err != nil {
		return domain.Token{}, errors.New("cannot sign JWT")
	}
	return domain.Token{AccessToken: encoded, TokenType: "Bearer", ExpiresIn: 3600}, nil
}

func (a *JWT) Verify(ctx context.Context, encoded string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := &claims{}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithoutClaimsValidation())
	token, err := parser.ParseWithClaims(encoded, payload, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, domain.ErrUnauthorized
		}
		return a.key, nil
	})
	if err != nil || token == nil || !token.Valid {
		return domain.ErrUnauthorized
	}
	now := a.now().Unix()
	if payload.ExpiresAt == nil || payload.ExpiresAt.Unix() <= now ||
		(payload.NotBefore != nil && payload.NotBefore.Unix() > now) ||
		(payload.IssuedAt != nil && payload.IssuedAt.Unix() > now) {
		return domain.ErrUnauthorized
	}
	return nil
}
