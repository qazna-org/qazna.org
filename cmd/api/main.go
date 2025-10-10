package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"qazna.org/internal/httpapi"
	"qazna.org/internal/obs"
)

var version = "0.6.2"

func main() {
	// Инициализация observability (регистрация метрик, JSON-логгер и т.п.)
	obs.Init()

	// Подключение к БД (если задан DSN), чтобы /readyz мог пинговать БД
	var db *sql.DB
	if dsn := os.Getenv("QAZNA_PG_DSN"); dsn != "" {
		var err error
		db, err = sql.Open("pgx", dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	// HTTP API
	api := httpapi.New(httpapi.ReadyProbe{DB: db}, version)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           api.Handler(), // уже обёрнут метриками в httpapi
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Starting qazna-api %s on %s", version, srv.Addr)

	// graceful shutdown
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
	if db != nil {
		_ = db.Close()
	}
	log.Println("Stopped")
}
