package requestid

import (
	"context"
	"errors"
	"strings"
	"testing"

	portmocks "http-api-template/internal/mocks/port"
)

func TestValid(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    string
		valid bool
	}{
		{"allowed", "AZaz09._-", true},
		{"maximum", strings.Repeat("x", 128), true},
		{"empty", "", false},
		{"too long", strings.Repeat("x", 129), false},
		{"space", "a b", false},
		{"newline", "a\nb", false},
		{"unicode", "ž", false},
		{"slash", "a/b", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Valid(tc.id); got != tc.valid {
				t.Fatalf("valid=%v", got)
			}
		})
	}
}

func TestEnsure(t *testing.T) {
	ids := portmocks.NewMockIDGenerator(t)
	ids.EXPECT().NewID().Return("generated-id", nil).Once()
	parent, cancel := context.WithCancel(context.Background())
	ctx, err := Ensure(parent, ids)
	if err != nil || ID(ctx) != "generated-id" {
		t.Fatalf("error=%v", err)
	}
	again, err := Ensure(ctx, ids)
	if err != nil || again != ctx {
		t.Fatal("existing IDs must remain stable")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("parent cancellation was lost")
	}
	failure := errors.New("entropy unavailable")
	ids.EXPECT().NewID().Return("", failure).Once()
	if _, err := Ensure(context.Background(), ids); !errors.Is(err, failure) {
		t.Fatal("generation failure was not propagated")
	}
	ids.EXPECT().NewID().Return("invalid value", nil).Once()
	if _, err := Ensure(context.Background(), ids); err == nil {
		t.Fatal("invalid generated IDs must fail")
	}
}
