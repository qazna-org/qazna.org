package obs

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Common HTTP metrics and readiness gauge.
var (
	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_inflight_requests",
		Help: "In-flight HTTP requests.",
	})

	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latencies in seconds.",
			Buckets: prometheus.DefBuckets, // [0.005..10]
		},
		[]string{"method", "path", "status"},
	)

	readyGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "qazna_ready",
		Help: "Readiness state (1 when ready).",
	})
)

func Init() {
	prometheus.MustRegister(httpInFlight, httpRequestsTotal, httpRequestDuration, readyGauge)
	readyGauge.Set(0)
}

func Handler() http.Handler {
	return promhttp.Handler()
}

func Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := canonicalPath(r.URL.Path)
		method := r.Method

		httpInFlight.Inc()
		start := time.Now()

		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(sw.code)

		httpRequestDuration.WithLabelValues(method, path, status).Observe(duration)
		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
		httpInFlight.Dec()
	})
}

func canonicalPath(path string) string {
	if path == "" {
		return "/"
	}
	if path == "/" || path == "/metrics" || path == "/healthz" || path == "/readyz" || path == "/v1/info" || path == "/openapi.yaml" {
		return path
	}
	if strings.HasPrefix(path, "/v1/accounts/") {
		rest := strings.TrimPrefix(path, "/v1/accounts/")
		if strings.HasSuffix(path, "/balance") && strings.Count(rest, "/") == 1 {
			return "/v1/accounts/:id/balance"
		}
		if !strings.Contains(rest, "/") {
			return "/v1/accounts/:id"
		}
	}
	if strings.HasPrefix(path, "/v1/ledger/transactions") {
		return "/v1/ledger/transactions"
	}
	if strings.HasPrefix(path, "/v1/transfers") {
		return "/v1/transfers"
	}
	return path
}

func SetReady(state bool) {
	if state {
		readyGauge.Set(1)
		return
	}
	readyGauge.Set(0)
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
