package requestid

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeIDs struct {
	value string
	err   error
	calls int
}

func (f *fakeIDs) NewID() (string, error) {
	f.calls++
	return f.value, f.err
}
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
	ids := &fakeIDs{value: "generated-id"}
	parent, cancel := context.WithCancel(context.Background())
	ctx, err := Ensure(parent, ids)
	if err != nil || ID(ctx) != ids.value || ids.calls != 1 {
		t.Fatalf("error=%v calls=%d", err, ids.calls)
	}
	again, err := Ensure(ctx, ids)
	if err != nil || again != ctx || ids.calls != 1 {
		t.Fatal("existing IDs must remain stable")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("parent cancellation was lost")
	}
	ids.err = errors.New("entropy unavailable")
	if _, err := Ensure(context.Background(), ids); !errors.Is(err, ids.err) {
		t.Fatal("generation failure was not propagated")
	}
	ids.err, ids.value = nil, "invalid value"
	if _, err := Ensure(context.Background(), ids); err == nil {
		t.Fatal("invalid generated IDs must fail")
	}
}
