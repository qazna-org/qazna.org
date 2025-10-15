package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qazna.org/internal/obs"
)

func TestRateLimitExceeded(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestID(RateLimit(base, 1, 1))

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req.Clone(context.Background()))
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected first call 200, got %d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req.Clone(context.Background()))
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}
	if rr2.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header")
	}

	var body map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode rate limit body: %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("expected error message in body")
	}
	if body["request_id"] == "" {
		t.Fatalf("expected request_id in body")
	}
}

func TestLoggingJSONEmitsStructuredEntry(t *testing.T) {
	logger := obs.Logger()
	origWriter := logger.Writer()
	logger.SetFlags(0)

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	defer logger.SetOutput(origWriter)

	handler := RequestID(LoggingJSON(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/log-test", nil)
	req.Header.Set("User-Agent", "middleware-test")
	req.RemoteAddr = "127.0.0.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req.Clone(context.Background()))

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected log line")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log is not valid JSON: %v", err)
	}
	for _, key := range []string{"ts", "level", "msg", "request_id", "method", "path", "status", "duration_ms"} {
		if _, ok := entry[key]; !ok {
			t.Fatalf("expected key %q in log entry", key)
		}
	}
	if entry["msg"] != "request_complete" {
		t.Fatalf("unexpected msg: %v", entry["msg"])
	}
	if entry["status"] != float64(http.StatusTeapot) {
		t.Fatalf("unexpected status: %v", entry["status"])
	}
}
