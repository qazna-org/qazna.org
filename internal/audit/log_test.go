package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"qazna.org/internal/auth"
	"qazna.org/internal/obs"
)

func TestLogEvent(t *testing.T) {
	logger := obs.Logger()
	original := logger.Writer()
	logger.SetFlags(0)
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	defer logger.SetOutput(original)

	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-123")
	ctx = auth.ContextWithUser(ctx, "user-42", []string{"admin"})

	if err := LogEvent(ctx, "audit.test", map[string]any{"foo": "bar"}); err != nil {
		t.Fatalf("LogEvent failed: %v", err)
	}

	line := buf.String()
	if line == "" {
		t.Fatal("expected log output")
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log not valid JSON: %v", err)
	}
	if entry["type"] != "audit" {
		t.Fatalf("unexpected type: %v", entry["type"])
	}
	if entry["event"] != "audit.test" {
		t.Fatalf("unexpected event: %v", entry["event"])
	}
	if entry["request_id"] != "req-123" {
		t.Fatalf("unexpected request id: %v", entry["request_id"])
	}
	if entry["user_id"] != "user-42" {
		t.Fatalf("unexpected user id: %v", entry["user_id"])
	}
	fields, ok := entry["fields"].(map[string]any)
	if !ok || fields["foo"] != "bar" {
		t.Fatalf("fields missing or incorrect: %v", entry["fields"])
	}
}
