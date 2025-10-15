package audit

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"qazna.org/internal/auth"
	"qazna.org/internal/obs"
)

type ctxKey string

const requestIDKey ctxKey = "audit_request_id"

// WithRequestID attaches the request identifier to the context for audit logging.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

// requestIDFromContext extracts the audit request id from context if present.
func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// LogEvent writes an audit log entry enriched with request and user context.
func LogEvent(ctx context.Context, event string, fields map[string]any) error {
	event = strings.TrimSpace(event)
	if event == "" {
		return errors.New("event name is required")
	}
	entry := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"type":  "audit",
		"event": event,
	}
	if rid := requestIDFromContext(ctx); rid != "" {
		entry["request_id"] = rid
	}
	if userID, ok := auth.UserIDFromContext(ctx); ok {
		entry["user_id"] = userID
	}
	if len(fields) > 0 {
		copyFields := make(map[string]any, len(fields))
		for k, v := range fields {
			copyFields[k] = v
		}
		entry["fields"] = copyFields
	} else {
		entry["fields"] = map[string]any{}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	obs.Logger().Println(string(data))
	return nil
}
