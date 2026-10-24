package auth

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v4"
	"http-api-template/internal/core/domain"
	"testing"
	"time"
)

const testSecret = "unit-test-signing-key"
const testEmail = "user@example.com"

func testJWT(t *testing.T) *JWT {
	t.Helper()
	adapter, err := NewJWT(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	adapter.now = func() time.Time { return time.Unix(1800000000, 0) }
	return adapter
}
func signedToken(t *testing.T, method jwt.SigningMethod, payload jwt.Claims, key any) string {
	t.Helper()
	encoded, err := jwt.NewWithClaims(method, payload).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
func TestIssue(t *testing.T) {
	adapter := testJWT(t)
	token, err := adapter.Issue(context.Background(), testEmail)
	if err != nil {
		t.Fatal(err)
	}
	if token.TokenType != "Bearer" || token.ExpiresIn != 3600 {
		t.Fatal("invalid token response")
	}
	payload := &claims{}
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsed, err := parser.ParseWithClaims(token.AccessToken, payload, func(_ *jwt.Token) (any, error) { return []byte(testSecret), nil })
	if err != nil || !parsed.Valid || parsed.Method != jwt.SigningMethodHS256 {
		t.Fatalf("invalid signature or algorithm: %v", err)
	}
	if payload.UserID != "1" || payload.Email != testEmail || payload.ExpiresAt == nil || payload.IssuedAt == nil || payload.ExpiresAt.Unix() != adapter.now().Add(time.Hour).Unix() || payload.IssuedAt.Unix() != adapter.now().Unix() {
		t.Fatal("stub claims or expiry were not preserved")
	}
	if err := adapter.Verify(context.Background(), token.AccessToken); err != nil {
		t.Fatal(err)
	}
	issuedAt := adapter.now()
	adapter.now = func() time.Time { return issuedAt.Add(time.Hour) }
	if err := adapter.Verify(context.Background(), token.AccessToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token error=%v", err)
	}
}
func TestRejectTokens(t *testing.T) {
	adapter := testJWT(t)
	valid := jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(adapter.now().Add(time.Hour))}
	for _, tc := range []struct {
		name  string
		token string
	}{
		{"wrong signature", signedToken(t, jwt.SigningMethodHS256, valid, []byte("different-key"))},
		{"wrong algorithm", signedToken(t, jwt.SigningMethodHS384, valid, []byte(testSecret))},
		{"unsigned", signedToken(t, jwt.SigningMethodNone, valid, jwt.UnsafeAllowNoneSignatureType)},
		{"missing expiry", signedToken(t, jwt.SigningMethodHS256, jwt.RegisteredClaims{}, []byte(testSecret))},
		{"future validity", signedToken(t, jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: valid.ExpiresAt, NotBefore: valid.ExpiresAt}, []byte(testSecret))},
		{"future issue", signedToken(t, jwt.SigningMethodHS256, jwt.RegisteredClaims{ExpiresAt: valid.ExpiresAt, IssuedAt: valid.ExpiresAt}, []byte(testSecret))},
		{"malformed", "invalid.jwt"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := adapter.Verify(context.Background(), tc.token); !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
func TestJWTConfigurationAndCancellation(t *testing.T) {
	if _, err := NewJWT(" "); err == nil {
		t.Fatal("empty signing key must be rejected")
	}
	adapter := testJWT(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Issue(ctx, testEmail); !errors.Is(err, context.Canceled) {
		t.Fatalf("issuance error=%v", err)
	}
	if err := adapter.Verify(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("verification error=%v", err)
	}
}
