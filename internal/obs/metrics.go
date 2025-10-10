package obs

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Общие HTTP-метрики
var (
	httpInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "http_in_flight_requests",
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
)

// Регистрация метрик в default-регистре.
func Init() {
	prometheus.MustRegister(httpInFlight, httpRequestsTotal, httpRequestDuration)
}

// Хэндлер Prometheus.
func Handler() http.Handler {
	return promhttp.Handler()
}

// Обёртка для измерения RPS/latency/в полёте.
func Instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path // без роутера берём как есть
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

// statusWriter — локальная копия, чтобы знать код ответа.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}
