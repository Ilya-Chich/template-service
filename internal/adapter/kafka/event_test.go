package kafka

import (
	"errors"
	"http-api-template/internal/core/domain"
	"strings"
	"testing"
)

const testDummyID = "550e8400-e29b-41d4-a716-446655440000"
const otherDummyID = "550e8400-e29b-41d4-a716-446655440001"
const createJSON = `{"event_type":"create","dummy_id":"` + testDummyID + `","dummy":{"id":"` + testDummyID + `","name":"Example","note":"note"}}`
const updateJSON = `{"event_type":"update","dummy_id":"` + testDummyID + `","patch":{"note":""}}`
const deleteJSON = `{"event_type":"delete","dummy_id":"` + testDummyID + `"}`

func TestEventCodec(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		kind string
	}{
		{eventCreate, createJSON, eventCreate},
		{eventUpdate, updateJSON, eventUpdate},
		{eventDelete, deleteJSON, eventDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoded, err := decodeEvent([]byte(tc.data))
			if err != nil || decoded.kind != tc.kind || decoded.id != testDummyID {
				t.Fatalf("event=%v error=%v", decoded, err)
			}
			if tc.kind == eventCreate && decoded.dummy != (domain.Dummy{ID: testDummyID, Name: "Example", Note: "note"}) {
				t.Fatal("create payload was changed")
			}
			if tc.kind == eventUpdate && (decoded.patch.Note == nil || *decoded.patch.Note != "" || decoded.patch.Name != nil) {
				t.Fatal("explicit empty note or omitted name was lost")
			}
		})
	}
	decoded, err := decodeEvent([]byte(strings.ReplaceAll(createJSON, testDummyID, strings.ToUpper(testDummyID))))
	if err != nil || decoded.id != testDummyID || decoded.dummy.ID != testDummyID {
		t.Fatal("UUID spelling must not change partition identity")
	}
}
func TestInvalidEvents(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
	}{
		{"malformed", "not JSON"},
		{"null", "null"},
		{"trailing JSON", deleteJSON + "{}"},
		{"unknown event", strings.Replace(deleteJSON, eventDelete, "unknown", 1)},
		{"missing ID", `{"event_type":"delete"}`},
		{"invalid ID", strings.ReplaceAll(deleteJSON, testDummyID, "invalid")},
		{"mismatched create ID", strings.Replace(createJSON, testDummyID, otherDummyID, 1)},
		{"missing dummy", `{"event_type":"create","dummy_id":"` + testDummyID + `"}`},
		{"missing name", strings.Replace(createJSON, `"name":"Example",`, "", 1)},
		{"blank name", strings.Replace(createJSON, "Example", "  ", 1)},
		{"null name", strings.Replace(createJSON, `"name":"Example"`, `"name":null`, 1)},
		{"null note", strings.Replace(createJSON, `"note":"note"`, `"note":null`, 1)},
		{"wrong note type", strings.Replace(createJSON, `"note":"note"`, `"note":1`, 1)},
		{"empty patch", strings.Replace(updateJSON, `{"note":""}`, "{}", 1)},
		{"null patch field", strings.Replace(updateJSON, `"note":""`, `"note":null`, 1)},
		{"null patch", strings.Replace(updateJSON, `{"note":""}`, "null", 1)},
		{"delete extra payload", strings.TrimSuffix(deleteJSON, "}") + `,"patch":{}}`},
		{"unknown fields", strings.TrimSuffix(deleteJSON, "}") + `,"extra":"private-value"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeEvent([]byte(tc.data)); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
