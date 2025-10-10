package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"qazna.org/api/spec"
	"qazna.org/internal/httpapi"
	"qazna.org/internal/ledger"
	"qazna.org/internal/obs"
	pgstore "qazna.org/internal/store/pg"
)

const serviceName = "qazna-api"
const serviceVersion = "0.6.1"

func main() {
	// Ledger backend: PostgreSQL или in-memory
	var svc ledger.Service

	if dsn := getenv("QAZNA_PG_DSN", ""); dsn != "" {
		store, err := pgstore.Open(dsn)
		if err != nil {
			log.Fatalf("postgres open failed: %v", err)
		}
		defer func() {
			if err := store.Close(); err != nil {
				log.Printf("postgres close error: %v", err)
			}
		}()
		svc = store
		log.Println(`{"level":"info","msg":"Using PostgreSQL store"}`)
	} else {
		mem := ledger.NewInMemory()
		svc = mem
		log.Println(`{"level":"info","msg":"Using in-memory store"}`)
	}

	// Prometheus metrics
	obs.Init()

	api := httpapi.New(svc)
	mux := http.NewServeMux()

	// ---- System
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"` + serviceName + `","version":"` + serviceVersion + `"}`))
	})
	mux.HandleFunc("/v1/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":    serviceName,
			"version": serviceVersion,
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
	// Prometheus metrics
	mux.Handle("/metrics", obs.Handler())

	// ---- OpenAPI / Docs
	mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write(spec.OpenAPI)
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(redocHTML("/openapi.yaml")))
	})

	// ---- Accounts & Ledger
	mux.HandleFunc("/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			api.CreateAccount(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/v1/accounts/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			path := r.URL.Path
			if len(path) >= len("/v1/accounts/") && hasSuffix(path, "/balance") {
				api.GetBalance(w, r)
				return
			}
			api.GetAccount(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/v1/transfers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			api.Transfer(w, r)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/v1/ledger/transactions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			api.ListTransactions(w, r)
			return
		}
		http.NotFound(w, r)
	})

	addr := getenv("QAZNA_API_ADDR", ":8080")

	// Middleware chain:
	// RequestID -> Instrument (Prometheus) -> LoggingJSON -> RateLimit -> MaxBody -> CORS -> Security
	handler := httpapi.RequestID(
		obs.Instrument(
			httpapi.LoggingJSON(
				httpapi.RateLimit(
					httpapi.MaxBodyBytes(
						httpapi.CORS(
							httpapi.SecurityHeaders(mux),
						),
						1<<20, // 1 MiB
					),
					100, // burst
					100, // tokens per second
				),
			),
		),
	)

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf(`{"level":"info","msg":"Starting %s","version":"%s","addr":"%s","metrics":"/metrics"}`, serviceName, serviceVersion, addr)

	// graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf(`{"level":"fatal","msg":"server error","error":%q}`, err.Error())
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	log.Println(`{"level":"info","msg":"Shutting down..."}`)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf(`{"level":"error","msg":"graceful shutdown error","error":%q}`, err.Error())
	} else {
		log.Println(`{"level":"info","msg":"Server stopped"}`)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func hasSuffix(s, suf string) bool {
	if len(suf) > len(s) {
		return false
	}
	return s[len(s)-len(suf):] == suf
}

func redocHTML(specPath string) string {
	return `<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8"/>
    <title>Qazna API Docs</title>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>body{margin:0;padding:0}</style>
    <link rel="icon" href="data:,">
  </head>
  <body>
    <redoc spec-url="` + specPath + `"></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"></script>
  </body>
</html>`
}
