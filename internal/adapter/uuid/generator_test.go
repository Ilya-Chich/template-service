package uuid

import (
	"github.com/google/uuid"
	"http-api-template/internal/core/domain"
	"testing"
)

func TestGenerator(t *testing.T) {
	first, err := (Generator{}).NewID()
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Generator{}).NewID()
	if err != nil {
		t.Fatal(err)
	}
	id, err := uuid.Parse(first)
	if err != nil || id.Version() != 4 || domain.ValidateID(first) != nil || domain.ValidateID(second) != nil || first == second {
		t.Fatal("generator must return distinct UUIDv4 values")
	}
}
