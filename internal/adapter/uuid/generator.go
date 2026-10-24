package uuid

import (
	"github.com/google/uuid"
	"http-api-template/internal/core/port"
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
