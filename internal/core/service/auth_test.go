package service

import (
	"context"
	"errors"
	"http-api-template/internal/core/domain"
	"http-api-template/internal/core/port"
	"testing"
)

const testEmail = "user@example.com"
const testPassword = "example"

type fakeIssuer struct {
	calls int
	email string
	ctx   context.Context
	err   error
	token domain.Token
}

var _ port.TokenIssuer = (*fakeIssuer)(nil)

func (f *fakeIssuer) Issue(ctx context.Context, email string) (domain.Token, error) {
	f.calls++
	f.ctx, f.email = ctx, email
	return f.token, f.err
}
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
			issuer := &fakeIssuer{err: tc.issuerErr, token: want}
			ctx := context.Background()
			token, err := NewAuth(issuer).Login(ctx, tc.email, tc.password)
			if !errors.Is(err, tc.wantErr) || issuer.calls != tc.wantCalls {
				t.Fatalf("error=%v issuer calls=%d", err, issuer.calls)
			}
			if err == nil && token != want {
				t.Fatal("token was not preserved")
			}
			if tc.wantCalls != 0 && (issuer.email != tc.email || issuer.ctx != ctx) {
				t.Fatal("issuer input was not preserved")
			}
		})
	}
}
