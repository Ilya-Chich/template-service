package uuid

import (
	"http-api-template/internal/core/port"

	"github.com/google/uuid"
)

type Generator struct{}

var _ port.IDGenerator = Generator{}

func (Generator) NewID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
