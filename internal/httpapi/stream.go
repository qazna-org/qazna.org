package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
)

// Stream handles Server-Sent Events for transfer flows.
func (a *API) Stream(w http.ResponseWriter, r *http.Request) {
	if a.stream == nil {
		http.Error(w, "streaming disabled", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	ch := a.stream.Subscribe(ctx)

	// Send an initial comment to establish the stream
	_, _ = w.Write([]byte(": stream started\n\n"))
	flusher.Flush()

	for event := range ch {
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
}
