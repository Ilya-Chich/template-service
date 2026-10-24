package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

const exampleURL = "postgres://example/db"

var _ io.Writer = (*MockWriter)(nil)

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override string
		wantCode int
	}{
		{"override", exampleURL, 0},
		{"missing configuration", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("POSTGRES_URL", tc.override)
			t.Setenv("POSTGRES_HOST", "")
			var stdout, stderr bytes.Buffer
			code := run(&stdout, &stderr)
			if code != tc.wantCode {
				t.Fatalf("exit code=%d", code)
			}
			if code == 0 && (stdout.String() != exampleURL+"\n" || stderr.Len() != 0) {
				t.Fatal("unexpected successful command output")
			}
			if code != 0 && (stdout.Len() != 0 || !strings.Contains(stderr.String(), "POSTGRES_HOST")) {
				t.Fatal("missing configuration must produce only an error")
			}
		})
	}
}

func TestRunWriteFailure(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("POSTGRES_URL", exampleURL)
	var stderr bytes.Buffer
	writer := NewMockWriter(t)
	writer.EXPECT().Write([]byte(exampleURL+"\n")).Return(0, errors.New("write failed")).Once()
	if code := run(writer, &stderr); code != 1 || !strings.Contains(stderr.String(), "cannot write migration URL") {
		t.Fatal("write failure did not produce a nonzero exit")
	}
}
