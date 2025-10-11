package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	v1 "qazna.org/api/gen/go/api/proto/qazna/v1"
	"qazna.org/internal/httpapi"
	"qazna.org/internal/ledger"
	"qazna.org/internal/obs"
	"qazna.org/internal/store/pg"
	"qazna.org/internal/stream"

	"google.golang.org/grpc"
)

var (
	version = "0.6.2"
	commit  = "dev"
)

func main() {
	// Initialize observability (register metrics, logging, etc.).
	obs.Init()
	obs.InitBuildInfo(version, commit)

	// Choose ledger backend: Postgres (when DSN provided) or in-memory (default).
	var (
		db         *sql.DB
		ledgerSvc  ledger.Service
		storeClose func() error
	)
	if dsn := os.Getenv("QAZNA_PG_DSN"); dsn != "" {
		store, err := pg.Open(dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		db = store.DB()
		ledgerSvc = store
		storeClose = store.Close
	} else {
		ledgerSvc = ledger.NewInMemory()
	}

	rp := httpapi.ReadyProbe{DB: db}

	evtStream := stream.New()

	// HTTP API setup.
	api := httpapi.New(rp, version, ledgerSvc, evtStream)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           api.Handler(), // already wrapped with observability middleware
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("Starting qazna-api %s on %s", version, srv.Addr)

	// Run HTTP server.
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http listen: %v", err)
		}
	}()

	// gRPC server setup.
	grpcAddr := os.Getenv("QAZNA_GRPC_ADDR")
	if grpcAddr == "" {
		grpcAddr = ":9090"
	}

	grpcSrv := grpc.NewServer()
	grpcAPI := httpapi.NewGRPCServer(rp, version)
	v1.RegisterInfoServiceServer(grpcSrv, grpcAPI)
	v1.RegisterHealthServiceServer(grpcSrv, grpcAPI)

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}

	var stopDemo func()
	if v := os.Getenv("QAZNA_STREAM_DEMO"); strings.EqualFold(v, "1") || strings.EqualFold(v, "true") {
		stopDemo = evtStream.StartDemo(3 * time.Second)
	}
	log.Printf("gRPC listening on %s", grpcAddr)

	go func() {
		if err := grpcSrv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
	grpcSrv.GracefulStop()
	_ = lis.Close()
	if stopDemo != nil {
		stopDemo()
	}
	if storeClose != nil {
		_ = storeClose()
	} else if db != nil {
		_ = db.Close()
	}
	log.Println("Stopped")
}
