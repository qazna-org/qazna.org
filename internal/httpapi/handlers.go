package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"qazna.org/api/spec"
	"qazna.org/internal/obs"
)

// ReadyProbe — простая проверка готовности (например, ping БД).
type ReadyProbe struct {
	DB *sql.DB
}

func (rp ReadyProbe) Check(ctx context.Context) error {
	if rp.DB == nil {
		return nil
	}
	return rp.DB.PingContext(ctx)
}

// API — HTTP слой.
type API struct {
	mux        *http.ServeMux
	readyProbe ReadyProbe
	version    string
}

func New(rp ReadyProbe, version string) *API {
	a := &API{
		mux:        http.NewServeMux(),
		readyProbe: rp,
		version:    version,
	}

	// health/ready/info
	a.mux.HandleFunc("/healthz", a.Healthz)
	a.mux.HandleFunc("/readyz", a.Ready)
	a.mux.HandleFunc("/v1/info", a.Info)

	// OpenAPI YAML
	a.mux.HandleFunc("/openapi.yaml", a.OpenAPISpec)

	// Prometheus metrics
	a.mux.Handle("/metrics", obs.Handler())

	// (опционально) корень — 404
	a.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	return a
}

// Handler возвращает http.Handler для сервера (без доп. аргументов).
func (a *API) Handler() http.Handler {
	// оборачиваем весь mux метриками
	return obs.Instrument(a.mux)
}

// --- Handlers ---

func (a *API) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": "qazna-api",
		"version": a.version,
	})
}

func (a *API) Ready(w http.ResponseWriter, r *http.Request) {
	if err := a.readyProbe.Check(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}

func (a *API) Info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "qazna-api",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": a.version,
	})
}

func (a *API) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(spec.OpenAPI) // в пакете qazna.org/api/spec: //go:embed openapi.yaml
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
