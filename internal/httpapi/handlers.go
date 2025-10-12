package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"time"

	"qazna.org/api/spec"
	"qazna.org/internal/ledger"
	"qazna.org/internal/obs"
	"qazna.org/internal/stream"
)

type readinessChecker interface {
	Check(ctx context.Context) error
}

// ReadyProbe performs a basic readiness check (for example, database ping).
type ReadyProbe struct {
	DB *sql.DB
}

func (rp ReadyProbe) Check(ctx context.Context) error {
	if rp.DB == nil {
		return nil
	}
	return rp.DB.PingContext(ctx)
}

// API implements the HTTP layer.
type API struct {
	mux         *http.ServeMux
	readiness   readinessChecker
	version     string
	ledger      ledger.Service
	stream      *stream.Stream
	templates   *template.Template
	bodyMaxSize int64
	rateBurst   int
	ratePerSec  int
}

func New(r readinessChecker, version string, ledgerService ledger.Service, s *stream.Stream, tmpl *template.Template) *API {
	a := &API{
		mux:         http.NewServeMux(),
		readiness:   r,
		version:     version,
		ledger:      ledgerService,
		stream:      s,
		templates:   tmpl,
		bodyMaxSize: 1 << 20, // 1 MiB per request body
		rateBurst:   20,
		ratePerSec:  10,
	}

	// health/ready/info
	a.mux.HandleFunc("/healthz", a.Healthz)
	a.mux.HandleFunc("/readyz", a.Ready)
	a.mux.HandleFunc("/v1/info", a.Info)

	// OpenAPI YAML
	a.mux.HandleFunc("/openapi.yaml", a.OpenAPISpec)

	// Static assets (CSS/JS/brand)
	a.mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("web/assets"))))

	// Streaming endpoint (SSE)
	a.mux.HandleFunc("/v1/stream", a.Stream)

	// Ledger endpoints
	a.mux.HandleFunc("/v1/accounts", a.handleAccountsCollection)
	a.mux.HandleFunc("/v1/accounts/", a.handleAccountResource)
	a.mux.HandleFunc("/v1/transfers", a.handleTransfers)
	a.mux.HandleFunc("/v1/ledger/transactions", a.handleTransactions)

	// Prometheus metrics
	a.mux.Handle("/metrics", obs.Handler())

	// Map pages
	a.mux.HandleFunc("/map", a.MapPage)
	a.mux.HandleFunc("/", a.MapPage)

	return a
}

// Handler returns the HTTP handler fully wrapped with middlewares.
func (a *API) Handler() http.Handler {
	var h http.Handler = a.mux
	h = MaxBodyBytes(h, a.bodyMaxSize)
	h = RateLimit(h, a.rateBurst, a.ratePerSec)
	h = CORS(h)
	h = SecurityHeaders(h)
	h = Recover(h)
	h = LoggingJSON(h)
	h = RequestID(h)
	return obs.Instrument(h)
}

// --- Handlers ---

func (a *API) Healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": serviceName,
		"version": a.version,
	})
}

func (a *API) Ready(w http.ResponseWriter, r *http.Request) {
	if err := a.readiness.Check(r.Context()); err != nil {
		obs.SetReady(false)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not_ready",
			"error":  err.Error(),
		})
		return
	}
	obs.SetReady(true)
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}

func (a *API) Info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    serviceName,
		"time":    time.Now().UTC().Format(time.RFC3339),
		"version": a.version,
	})
}

func (a *API) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(spec.OpenAPI) // content is embedded via //go:embed in qazna.org/api/spec
}

func (a *API) MapPage(w http.ResponseWriter, r *http.Request) {
	if a.templates == nil {
		http.NotFound(w, r)
		return
	}
	data := map[string]any{
		"Title": "Qazna Global Flow",
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, "map", data); err != nil {
		http.Error(w, "template rendering error", http.StatusInternalServerError)
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) resolveLocation(id string) stream.Location {
	if a.stream == nil {
		return stream.Location{}
	}
	loc := a.stream.LocationForID(id)
	if loc.Name == "" {
		loc.Name = id
	}
	return loc
}
