package service

import (
	"context"
	"errors"
	"testing"

	"http-api-template/internal/core/domain"
	portmocks "http-api-template/internal/mocks/port"
)

const (
	testEmail    = "user@example.com"
	testPassword = "example"
)

func TestLogin(t *testing.T) {
	for _, tc := range []struct {
		name      string
		email     string
		password  string
		issuerErr error
		wantErr   error
		wantCalls int
	}{
		{"valid", testEmail, testPassword, nil, nil, 1},
		{"empty email", "", testPassword, nil, domain.ErrInvalid, 0},
		{"bad email", "not-an-email", testPassword, nil, domain.ErrInvalid, 0},
		{"display name", "User <user@example.com>", testPassword, nil, domain.ErrInvalid, 0},
		{"empty password", testEmail, "", nil, domain.ErrInvalid, 0},
		{"issuer failure", testEmail, testPassword, errDependency, errDependency, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := domain.Token{AccessToken: "test-token", TokenType: "Bearer", ExpiresIn: 3600}
			issuer := portmocks.NewMockTokenIssuer(t)
			ctx := context.Background()
			if tc.wantCalls != 0 {
				issuer.EXPECT().Issue(ctx, tc.email).Return(want, tc.issuerErr).Once()
			}
			token, err := NewAuth(issuer).Login(ctx, tc.email, tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v", err)
			}
			if err == nil && token != want {
				t.Fatal("token was not preserved")
			}
		})
	}
}
