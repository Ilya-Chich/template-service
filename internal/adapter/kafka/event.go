package kafka

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"http-api-template/internal/core/domain"
	"io"
	"strings"
)

const (
	eventCreate = "create"
	eventUpdate = "update"
	eventDelete = "delete"
)

type envelope struct {
	EventType string          `json:"event_type"`
	DummyID   string          `json:"dummy_id"`
	Dummy     json.RawMessage `json:"dummy,omitempty"`
	Patch     json.RawMessage `json:"patch,omitempty"`
}
type dummyPayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Note string `json:"note"`
}
type patchPayload struct {
	Name *string `json:"name,omitempty"`
	Note *string `json:"note,omitempty"`
}
type event struct {
	kind  string
	id    string
	dummy domain.Dummy
	patch domain.Patch
}

func invalidEvent() error {
	return fmt.Errorf("%w: invalid Kafka event", domain.ErrInvalid)
}
func decodeJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return invalidEvent()
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return invalidEvent()
	}
	return nil
}
func stringField(raw json.RawMessage, required bool) (string, error) {
	if len(raw) == 0 {
		if required {
			return "", invalidEvent()
		}
		return "", nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", invalidEvent()
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", invalidEvent()
	}
	return value, nil
}
func decodeEvent(data []byte) (event, error) {
	var wire envelope
	if err := decodeJSON(data, &wire); err != nil {
		return event{}, err
	}
	if err := domain.ValidateID(wire.DummyID); err != nil {
		return event{}, err
	}
	result := event{kind: wire.EventType, id: strings.ToLower(wire.DummyID)}
	switch wire.EventType {
	case eventCreate:
		if len(wire.Patch) != 0 {
			return event{}, invalidEvent()
		}
		var payload struct {
			ID   json.RawMessage `json:"id"`
			Name json.RawMessage `json:"name"`
			Note json.RawMessage `json:"note"`
		}
		if err := decodeJSON(wire.Dummy, &payload); err != nil {
			return event{}, err
		}
		id, idErr := stringField(payload.ID, true)
		name, nameErr := stringField(payload.Name, true)
		note, noteErr := stringField(payload.Note, false)
		if idErr != nil || nameErr != nil || noteErr != nil || !strings.EqualFold(id, result.id) {
			return event{}, invalidEvent()
		}
		result.dummy = domain.Dummy{ID: result.id, Name: name, Note: note}
		if err := result.dummy.Validate(); err != nil {
			return event{}, err
		}
	case eventUpdate:
		if len(wire.Dummy) != 0 {
			return event{}, invalidEvent()
		}
		var payload struct {
			Name json.RawMessage `json:"name"`
			Note json.RawMessage `json:"note"`
		}
		if err := decodeJSON(wire.Patch, &payload); err != nil {
			return event{}, err
		}
		for _, field := range []struct {
			raw    json.RawMessage
			target **string
		}{{payload.Name, &result.patch.Name}, {payload.Note, &result.patch.Note}} {
			if len(field.raw) != 0 {
				value, err := stringField(field.raw, true)
				if err != nil {
					return event{}, err
				}
				*field.target = &value
			}
		}
		if err := result.patch.Validate(); err != nil {
			return event{}, err
		}
	case eventDelete:
		if len(wire.Dummy) != 0 || len(wire.Patch) != 0 {
			return event{}, invalidEvent()
		}
	default:
		return event{}, invalidEvent()
	}
	return result, nil
}
