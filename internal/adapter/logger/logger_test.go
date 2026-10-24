package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"http-api-template/internal/core/requestid"
	"log/slog"
	"strings"
	"testing"
)

type fakeIDs struct {
	err error
}

func (f fakeIDs) NewID() (string, error) {
	return "generated-id", f.err
}
func TestJSONLogging(t *testing.T) {
	var output bytes.Buffer
	log := New(&output, fakeIDs{})
	ctx := requestid.WithID(context.Background(), "incoming-id")
	log.Info(ctx, "request accepted", "status", 202)
	log.Error(ctx, "operation failed", "password", "private-password", "JWT_SECRET", "private-key", "Authorization", "private-token", "payload", "private-payload", slog.Group("nested", slog.String("access_token", "private-access")))
	decoder := json.NewDecoder(&output)
	for _, level := range []string{"INFO", "ERROR"} {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		if record["request_id"] != "incoming-id" || record["level"] != level || record["time"] == nil {
			t.Fatalf("invalid log record: %v", record)
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "private-") {
			t.Fatal("sensitive attribute value was logged")
		}
	}
}
func TestFallbackCorrelation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"generated", nil, "generated-id"},
		{"generation failure", errors.New("entropy unavailable"), "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			log := New(&output, fakeIDs{err: tc.err})
			log.Info(context.Background(), "worker")
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			if record["request_id"] != tc.want {
				t.Fatal("log lacks fallback correlation")
			}
		})
	}
}
