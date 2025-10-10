package httpapi

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// Logging: method, path, status, duration
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, code: 200}
		start := time.Now()
		next.ServeHTTP(sw, r)
		d := time.Since(start)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.code, d)
	})
}

// SecurityHeaders: hardening + CSP tuned for ReDoc
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "no-referrer")

		// CSP:
		// - script: только self + jsdelivr (ReDoc bundle)
		// - img: self + data: + cdn.redoc.ly (логотип)
		// - style: self + inline (ReDoc инлайн-стили)
		// - connect: self + jsdelivr (sourcemap; безопасно)
		// - worker: self + blob: (нужен web worker ReDoc)
		// - frame-ancestors: запрет встраивания
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"img-src 'self' data: https://cdn.redoc.ly; "+
				"style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' https://cdn.jsdelivr.net; "+
				"connect-src 'self' https://cdn.jsdelivr.net; "+
				"worker-src 'self' blob:; "+
				"frame-ancestors 'none'")

		next.ServeHTTP(w, r)
	})
}

// CORS: locked but practical (adjust origins if needed)
func CORS(next http.Handler) http.Handler {
	allowedMethods := "GET,POST,OPTIONS"
	allowedHeaders := "Content-Type,Idempotency-Key"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if isLocalOrigin(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
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

// MaxBodyBytes: limit request body size
func MaxBodyBytes(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}

// RateLimit: token-bucket per client IP
func RateLimit(next http.Handler, burst int, perSecond int) http.Handler {
	type bucket struct {
		lim *rate.Limiter
		ts  time.Time
	}
	var (
		buckets = make(map[string]*bucket)
		ttl     = 5 * time.Minute
	)
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			now := time.Now()
			for k, b := range buckets {
				if now.Sub(b.ts) > ttl {
					delete(buckets, k)
				}
			}
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip == "" {
			ip = "unknown"
		}
		b, ok := buckets[ip]
		if !ok {
			lim := rate.NewLimiter(rate.Limit(perSecond), burst)
			b = &bucket{lim: lim, ts: time.Now()}
			buckets[ip] = b
		}
		b.ts = time.Now()
		if !b.lim.Allow() {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// X-Forwarded-For support (first IP)
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
	// allow localhost during dev; extend list for prod domains later
	return strings.HasPrefix(o, "http://localhost:") || strings.HasPrefix(o, "http://127.0.0.1:")
}
