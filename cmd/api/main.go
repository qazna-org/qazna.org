package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"qazna.org/internal/httpapi"
	"qazna.org/internal/ledger"
	pgstore "qazna.org/internal/store/pg"
)

const serviceName = "qazna-api"
const serviceVersion = "0.3.0"

func main() {
	// Select ledger backend: PostgreSQL (if QAZNA_PG_DSN is set) or in-memory
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
		log.Println("Using PostgreSQL store")
	} else {
		mem := ledger.NewInMemory()
		svc = mem
		log.Println("Using in-memory store")
	}

	// HTTP API
	api := httpapi.New(svc) // NOTE: New must accept ledger.Service

	mux := http.NewServeMux()

	// Basic endpoints
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

	// Accounts
	mux.HandleFunc("/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			api.CreateAccount(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	// GET /v1/accounts/{id}  and  GET /v1/accounts/{id}/balance?currency=QZN
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

	// Transfers
	mux.HandleFunc("/v1/transfers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			api.Transfer(w, r)
			return
		}
		http.NotFound(w, r)
	})

	// Transactions listing
	mux.HandleFunc("/v1/ledger/transactions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			api.ListTransactions(w, r)
			return
		}
		http.NotFound(w, r)
	})

	addr := getenv("QAZNA_API_ADDR", ":8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpapi.Logging(mux), // lightweight request logging
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Starting %s %s on %s", serviceName, serviceVersion, addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// tiny suffix helper (avoids importing path libs for simple check)
func hasSuffix(s, suf string) bool {
	if len(suf) > len(s) {
		return false
	}
	return s[len(s)-len(suf):] == suf
}
