package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type statusWriter struct {
	http.ResponseWriter
	code int
	n    int // bytes written
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.n += n
	return n, err
}

/* =========================
   Request ID (context)
   ========================= */

type ctxKey int

const requestIDKey ctxKey = iota

func genID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = genID()
		}
		w.Header().Set("X-Request-Id", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

/* =========================
   JSON logging
   ========================= */

type logRecord struct {
	Time       string  `json:"time"`
	Level      string  `json:"level"`
	RequestID  string  `json:"request_id,omitempty"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	RemoteIP   string  `json:"remote_ip"`
	Status     int     `json:"status"`
	DurationMs float64 `json:"duration_ms"`
	Bytes      int     `json:"bytes"`
	UserAgent  string  `json:"user_agent,omitempty"`
	Referer    string  `json:"referer,omitempty"`
}

func LoggingJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		d := time.Since(start)

		rec := logRecord{
			Time:       time.Now().UTC().Format(time.RFC3339Nano),
			Level:      "info",
			RequestID:  RequestIDFromContext(r.Context()),
			Method:     r.Method,
			Path:       r.URL.Path,
			RemoteIP:   clientIP(r),
			Status:     sw.code,
			DurationMs: float64(d.Microseconds()) / 1000.0,
			Bytes:      sw.n,
			UserAgent:  r.UserAgent(),
			Referer:    r.Referer(),
		}
		b, _ := json.Marshal(rec)
		log.Println(string(b))
	})
}

/* =========================
   Panic recovery (500 + лог)
   ========================= */

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func(start time.Time) {
			if rec := recover(); rec != nil {
				log.Println(`{"level":"error","msg":"panic recovered","request_id":"` + RequestIDFromContext(r.Context()) + `"}`)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}(time.Now())
		next.ServeHTTP(w, r)
	})
}

/* =========================
   Security / CORS / Limits
   ========================= */

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// CSP tuned for ReDoc (worker/img/jsdelivr)
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"img-src 'self' data: https://cdn.redoc.ly https://*.tile.openstreetmap.org; "+
				"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; "+
				"script-src 'self' https://cdn.jsdelivr.net; "+
				"connect-src 'self' https://cdn.jsdelivr.net; "+
				"worker-src 'self' blob:; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func CORS(next http.Handler) http.Handler {
	allowedMethods := "GET,POST,OPTIONS"
	allowedHeaders := "Content-Type,Idempotency-Key,X-Request-Id"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
		w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func MaxBodyBytes(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

func RateLimit(next http.Handler, burst int, perSecond int) http.Handler {
	if burst <= 0 || perSecond <= 0 {
		return next
	}

	type bucket struct {
		lim  *rate.Limiter
		last time.Time
	}

	var (
		mu          sync.Mutex
		buckets     = make(map[string]*bucket)
		ttl         = 5 * time.Minute
		cleanupTick = 1 * time.Minute
		lastSweep   time.Time
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		ip := clientIP(r)
		if ip == "" {
			ip = "unknown"
		}
		now := time.Now()

		mu.Lock()
		if now.Sub(lastSweep) >= cleanupTick {
			for key, entry := range buckets {
				if now.Sub(entry.last) > ttl {
					delete(buckets, key)
				}
			}
			lastSweep = now
		}

		entry, ok := buckets[ip]
		if !ok {
			entry = &bucket{
				lim:  rate.NewLimiter(rate.Limit(perSecond), burst),
				last: now,
			}
			buckets[ip] = entry
		}
		entry.last = now
		lim := entry.lim
		mu.Unlock()

		if !lim.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limit exceeded"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func isLocalOrigin(o string) bool {
	return strings.HasPrefix(o, "http://localhost:") || strings.HasPrefix(o, "http://127.0.0.1:")
}

var (
	allowedOriginsOnce sync.Once
	allowedOriginsList []string
)

func loadAllowedOrigins() {
	allowedOriginsOnce.Do(func() {
		raw := os.Getenv("QAZNA_ALLOWED_ORIGINS")
		if raw == "" {
			allowedOriginsList = nil
			return
		}
		parts := strings.Split(raw, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			allowedOriginsList = append(allowedOriginsList, p)
		}
	})
}

func isAllowedOrigin(origin string) bool {
	loadAllowedOrigins()
	if len(allowedOriginsList) > 0 {
		for _, allowed := range allowedOriginsList {
			if strings.EqualFold(allowed, origin) {
				return true
			}
		}
		return false
	}
	return isLocalOrigin(origin)
}
