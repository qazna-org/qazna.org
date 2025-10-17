package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"qazna.org/api/spec"
	"qazna.org/internal/audit"
	"qazna.org/internal/auth"
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
	auth        *auth.Service
	rbac        *auth.RBACService
	templates   *template.Template
	bodyMaxSize int64
	rateBurst   int
	ratePerSec  int
}

func New(
	r readinessChecker,
	version string,
	ledgerService ledger.Service,
	s *stream.Stream,
	tmpl *template.Template,
	authSvc *auth.Service,
	rbacSvc *auth.RBACService,
) *API {
	a := &API{
		mux:         http.NewServeMux(),
		readiness:   r,
		version:     version,
		ledger:      ledgerService,
		stream:      s,
		auth:        authSvc,
		rbac:        rbacSvc,
		templates:   tmpl,
		bodyMaxSize: 1 << 20, // 1 MiB per request body
		rateBurst:   400,
		ratePerSec:  200,
	}

	a.rateBurst = envInt("QAZNA_RATE_LIMIT_BURST", a.rateBurst)
	a.ratePerSec = envInt("QAZNA_RATE_LIMIT_RPS", a.ratePerSec)

	// health/ready/info
	a.mux.HandleFunc("/healthz", a.Healthz)
	a.mux.HandleFunc("/readyz", a.Ready)
	a.mux.HandleFunc("/v1/info", a.Info)
	a.mux.HandleFunc("/v1/auth/token", a.handleAuthToken)
	a.mux.HandleFunc("/v1/auth/jwks", a.handleJWKS)
	a.mux.HandleFunc("/v1/auth/oauth/authorize", a.handleOAuthAuthorize)
	a.mux.HandleFunc("/v1/auth/oauth/token", a.handleOAuthToken)

	// OpenAPI YAML
	a.mux.HandleFunc("/openapi.yaml", a.OpenAPISpec)

	// Static assets (CSS/JS/brand)
	a.mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("web/assets"))))

	// Streaming endpoint (SSE)
	a.mux.HandleFunc("/v1/stream", a.Stream)

	// Ledger endpoints
	a.mux.Handle("/v1/accounts", RequireRole("admin")(http.HandlerFunc(a.handleAccountsCollection)))
	a.mux.HandleFunc("/v1/accounts/", a.handleAccountResource)
	a.mux.Handle("/v1/transfers", RequireRole("admin")(http.HandlerFunc(a.handleTransfers)))
	a.mux.HandleFunc("/v1/ledger/transactions", a.handleTransactions)

	// RBAC management endpoints
	a.mux.Handle("/v1/organizations", http.HandlerFunc(a.handleOrganizations))
	a.mux.HandleFunc("/v1/organizations/", a.handleOrganizationScoped)
	a.mux.HandleFunc("/v1/roles/", a.handleRoleResource)
	a.mux.HandleFunc("/v1/users/", a.handleUserResource)

	// Prometheus metrics
	a.mux.Handle("/metrics", obs.Handler())

	// Map pages
	a.mux.HandleFunc("/map", a.MapPage)
	a.mux.HandleFunc("/admin/dashboard", a.AdminDashboard)
	a.mux.HandleFunc("/banks/dashboard", a.BankDashboard)
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
	h = a.withAuth(h)
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

func (a *API) handleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, r, http.MethodGet)
		return
	}
	if a.auth == nil {
		writeError(w, r, http.StatusNotImplemented, "jwks unavailable")
		return
	}
	jwks, err := a.auth.JWKS(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "jwks generation failed")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(jwks)
}

func (a *API) MapPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Title":             "Qazna Global Flow",
		"ContentTemplate":   "content-map",
		"BodyClass":         "map-page",
		"Layout":            "map",
		"IncludeMapScripts": true,
		"ActivePage":        "map",
	}
	a.renderTemplate(w, r, "map", data)
}

func (a *API) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	lastUpdated := time.Now().UTC().Format("2006-01-02 15:04 MST")
	readyStatus := "Operational"
	readyBadge := "success"
	readyDetail := "All probes passing"
	var alerts []map[string]string

	if err := a.readiness.Check(r.Context()); err != nil {
		readyStatus = "Attention"
		readyBadge = "warning"
		readyDetail = err.Error()
		alerts = append(alerts, map[string]string{
			"Title":        "Readiness probe degraded",
			"Severity":     "Warning",
			"BadgeVariant": "warning",
			"Timestamp":    lastUpdated,
			"Detail":       err.Error(),
		})
	}

	ledgerStatus := "Synchronized"
	ledgerBadge := "success"
	ledgerDetail := "Ledger adapter responding"
	ctx, cancel := context.WithTimeout(r.Context(), 250*time.Millisecond)
	defer cancel()
	if _, _, err := a.ledger.ListTransactions(ctx, 1, 0); err != nil {
		ledgerStatus = "Degraded"
		ledgerBadge = "warning"
		ledgerDetail = fmt.Sprintf("Ledger access issue: %v", err)
		alerts = append(alerts, map[string]string{
			"Title":        "Ledger synchronization degraded",
			"Severity":     "Warning",
			"BadgeVariant": "warning",
			"Timestamp":    lastUpdated,
			"Detail":       err.Error(),
		})
	}

	streamStatus := "Streaming"
	streamBadge := "success"
	streamDetail := "Event fan-out active"
	if a.stream == nil {
		streamStatus = "Paused"
		streamBadge = "secondary"
		streamDetail = "Event stream disabled"
	}

	metrics := []map[string]string{
		{"Title": "Network Uptime", "Value": "99.982%", "Description": "Rolling 30-day availability", "Trend": "↑ Stable", "TrendVariant": "success"},
		{"Title": "Active Institutions", "Value": "18", "Description": "Connected sovereign actors", "Trend": "3 new this quarter", "TrendVariant": "primary"},
		{"Title": "Pending Settlements", "Value": "12", "Description": "Awaiting reconciliation", "Trend": "", "TrendVariant": ""},
		{"Title": "Audit Log (24h)", "Value": "642", "Description": "Recorded governance events", "Trend": "", "TrendVariant": ""},
	}

	processStatuses := []map[string]string{
		{"Name": "API readiness", "Status": readyStatus, "Detail": readyDetail, "BadgeVariant": readyBadge},
		{"Name": "Ledger adapter", "Status": ledgerStatus, "Detail": ledgerDetail, "BadgeVariant": ledgerBadge},
		{"Name": "Event stream", "Status": streamStatus, "Detail": streamDetail, "BadgeVariant": streamBadge},
	}

	headerNotifications := make([]map[string]string, 0, len(alerts))
	for _, alert := range alerts {
		headerNotifications = append(headerNotifications, map[string]string{
			"Title":        alert["Title"],
			"Description":  alert["Detail"],
			"Time":         alert["Timestamp"],
			"BadgeVariant": alert["BadgeVariant"],
			"BadgeLabel":   alert["Severity"],
		})
	}
	if len(headerNotifications) == 0 {
		headerNotifications = []map[string]string{
			{
				"Title":        "All systems operational",
				"Description":  "Operational probes report nominal performance.",
				"Time":         "Updated " + lastUpdated,
				"BadgeVariant": "success",
				"BadgeLabel":   "Healthy",
			},
		}
	}

	headerMessages := []map[string]string{
		{"Sender": "Operations Desk", "Preview": "Liquidity sweep completed without exceptions.", "Time": "5m ago"},
		{"Sender": "Compliance Office", "Preview": "Quarterly attestation package ready for review.", "Time": "45m ago"},
	}

	headerUser := map[string]string{
		"Name": "Qazna Control",
		"Role": "Administrator",
	}

	data := map[string]any{
		"Title":               "Qazna Control Center",
		"ContentTemplate":     "content-admin-dashboard",
		"BodyClass":           "dashboard d-flex flex-column min-vh-100",
		"Layout":              "dashboard",
		"ActivePage":          "admin",
		"Version":             a.version,
		"LastUpdated":         lastUpdated,
		"SystemMetrics":       metrics,
		"ProcessStatuses":     processStatuses,
		"RecentAlerts":        alerts,
		"RecentTransactions":  a.getRecentTransactions(r.Context(), 8),
		"HeaderNotifications": headerNotifications,
		"HeaderMessages":      headerMessages,
		"HeaderUser":          headerUser,
	}
	a.renderTemplate(w, r, "admin-dashboard", data)
}

func (a *API) BankDashboard(w http.ResponseWriter, r *http.Request) {
	lastUpdated := time.Now().UTC().Format("2006-01-02 15:04 MST")
	bankMetrics := []map[string]string{
		{"Title": "Reserve balance", "Value": "1.8B QZN", "Description": "Fully collateralised reserves", "Accent": "primary"},
		{"Title": "Intraday transfers", "Value": "264", "Description": "Processed in the last 24h", "Accent": "success"},
		{"Title": "Open requests", "Value": "6", "Description": "Awaiting approval", "Accent": "warning"},
		{"Title": "Compliance status", "Value": "Aligned", "Description": "Basel III / AML / CFT", "Accent": "info"},
	}

	liquidity := []map[string]string{
		{"Pool": "USD Correspondent", "Coverage": "112% coverage", "Status": "Balanced", "BadgeVariant": "success"},
		{"Pool": "EUR TARGET2", "Coverage": "105% coverage", "Status": "Stable", "BadgeVariant": "primary"},
		{"Pool": "CNY Swap Line", "Coverage": "89% coverage", "Status": "Monitor", "BadgeVariant": "warning"},
	}

	queue := []map[string]string{
		{"Counterparty": "Bank of Astana", "SettlementWindow": "T+0 · 14:00 UTC", "Amount": "42.5M QZN"},
		{"Counterparty": "National Bank of Kazakhstan", "SettlementWindow": "T+0 · 16:30 UTC", "Amount": "18.0M QZN"},
		{"Counterparty": "Kazakh Sovereign Fund", "SettlementWindow": "T+1 · 09:15 UTC", "Amount": "5.4M QZN"},
	}

	regions := []map[string]string{
		{"Region": "Eurasia", "Today": "+58.2M QZN", "Trend": "↑ 12%", "TrendVariant": "success"},
		{"Region": "MENA", "Today": "+21.7M QZN", "Trend": "→ Stable", "TrendVariant": "primary"},
		{"Region": "APAC", "Today": "-6.3M QZN", "Trend": "↓ 5%", "TrendVariant": "warning"},
	}

	headerNotifications := []map[string]string{
		{
			"Title":        "Settlement window approaching",
			"Description":  "Bank of Astana request closes at 14:00 UTC.",
			"Time":         "13m",
			"BadgeVariant": "warning",
			"BadgeLabel":   "Action",
		},
		{
			"Title":        "Liquidity coverage stable",
			"Description":  "EUR TARGET2 pool remains above 105% coverage.",
			"Time":         "1h",
			"BadgeVariant": "success",
			"BadgeLabel":   "Stable",
		},
	}

	headerMessages := []map[string]string{
		{"Sender": "Settlement Desk", "Preview": "Confirm T+0 instructions with National Bank of Kazakhstan.", "Time": "12m ago"},
		{"Sender": "Compliance Team", "Preview": "Updated AML checklist ready for acknowledgement.", "Time": "1h ago"},
	}

	headerUser := map[string]string{
		"Name": "Sovereign Reserve Bank",
		"Role": "Operator",
	}

	data := map[string]any{
		"Title":               "Central Bank Dashboard",
		"ContentTemplate":     "content-bank-dashboard",
		"BodyClass":           "dashboard d-flex flex-column min-vh-100",
		"Layout":              "dashboard",
		"ActivePage":          "banks",
		"InstitutionName":     "Sovereign Reserve Bank",
		"LastUpdated":         lastUpdated,
		"BankMetrics":         bankMetrics,
		"LiquidityOverview":   liquidity,
		"SettlementQueue":     queue,
		"RegionalBreakdown":   regions,
		"RecentTransactions":  a.getRecentTransactions(r.Context(), 5),
		"HeaderNotifications": headerNotifications,
		"HeaderMessages":      headerMessages,
		"HeaderUser":          headerUser,
	}
	a.renderTemplate(w, r, "bank-dashboard", data)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) renderTemplate(w http.ResponseWriter, r *http.Request, tmpl string, data map[string]any) {
	if a.templates == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.templates.ExecuteTemplate(w, tmpl, data); err != nil {
		http.Error(w, "template rendering error", http.StatusInternalServerError)
	}
}

func (a *API) getRecentTransactions(ctx context.Context, limit int) []map[string]string {
	if limit <= 0 {
		limit = 5
	}
	txs, _, err := a.ledger.ListTransactions(ctx, limit, 0)
	if err != nil {
		return nil
	}
	items := make([]map[string]string, 0, len(txs))
	for _, tx := range txs {
		items = append(items, map[string]string{
			"CreatedAt":     formatTimestamp(tx.CreatedAt),
			"Sequence":      fmt.Sprintf("%d", tx.Sequence),
			"FromAccountID": tx.FromAccountID,
			"ToAccountID":   tx.ToAccountID,
			"Amount":        formatAmount(tx.Amount),
			"Currency":      tx.Currency,
		})
	}
	return items
}

func formatAmount(minorUnits int64) string {
	major := float64(minorUnits) / 100.0
	return fmt.Sprintf("%0.2f", major)
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05")
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

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return def
	}
	return val
}

func (a *API) audit(ctx context.Context, action, resourceType, resourceID string, metadata map[string]string) {
	fields := map[string]any{}
	if resourceType != "" {
		fields["resource_type"] = resourceType
	}
	if resourceID != "" {
		fields["resource_id"] = resourceID
	}
	if len(metadata) > 0 {
		for k, v := range metadata {
			fields[k] = v
		}
	}
	if err := audit.LogEvent(ctx, action, fields); err != nil {
		obs.LogRequest(map[string]any{
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"level": "error",
			"msg":   "audit_log_failed",
			"event": action,
			"error": err.Error(),
		})
	}
}
