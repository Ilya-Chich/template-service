package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateID(t *testing.T) {
	for _, tc := range []struct {
		name  string
		id    string
		valid bool
	}{
		{"valid", "550e8400-e29b-41d4-a716-446655440000", true},
		{"uppercase", "550E8400-E29B-41D4-A716-446655440000", true},
		{"empty", "", false},
		{"compact", "550e8400e29b41d4a716446655440000", false},
		{"invalid hex", "z50e8400-e29b-41d4-a716-446655440000", false},
		{"wrong separators", "550e8400_e29b-41d4-a716-446655440000", false},
		{"extra separator", "550e8400-e29b-41d4-a716-44665544-000", false},
		{"whitespace", " 550e8400-e29b-41d4-a716-446655440000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateID(tc.id)
			if (err == nil) != tc.valid || err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("error=%v valid=%v", err, tc.valid)
			}
		})
	}
}
func TestValidateName(t *testing.T) {
	for _, name := range []string{"", " ", "\t\n", "\u2003"} {
		if err := ValidateName(name); !errors.Is(err, ErrInvalid) {
			t.Fatalf("name=%q error=%v", name, err)
		}
	}
	for _, name := range []string{"Example", " Example ", "example", strings.Repeat("a", 20)} {
		if err := ValidateName(name); err != nil {
			t.Fatal(err)
		}
	}
}
func TestPatchValidation(t *testing.T) {
	name, empty := "Example", ""
	for _, tc := range []struct {
		name  string
		patch Patch
		valid bool
	}{
		{"empty", Patch{}, false},
		{"name", Patch{Name: &name}, true},
		{"both", Patch{Name: &name, Note: &empty}, true},
		{"clear note", Patch{Note: &empty}, true},
		{"empty name", Patch{Name: &empty}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.patch.Validate()
			if (err == nil) != tc.valid || err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
