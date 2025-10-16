package main

import (
	"context"
	"database/sql"
	"errors"
	"html/template"
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
	"qazna.org/internal/auth"
	"qazna.org/internal/httpapi"
	"qazna.org/internal/ledger"
	"qazna.org/internal/ledger/remote"
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

	// Choose ledger backend: remote gRPC, Postgres (DSN), or in-memory.
	var (
		db           *sql.DB
		ledgerSvc    ledger.Service
		storeClose   func() error
		remoteClient *remote.Client
		authSvc      *auth.Service
	)
	if addr := os.Getenv("QAZNA_LEDGER_GRPC_ADDR"); addr != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client, err := remote.Dial(ctx, addr)
		cancel()
		if err != nil {
			log.Fatalf("dial remote ledger: %v", err)
		}
		ledgerSvc = remote.NewService(client)
		remoteClient = client
		log.Printf("Using remote ledger at %s", addr)
	} else if dsn := os.Getenv("QAZNA_PG_DSN"); dsn != "" {
		store, errPG := pg.Open(dsn)
		if errPG != nil {
			log.Fatalf("open db: %v", errPG)
		}
		db = store.DB()
		ledgerSvc = store
		storeClose = store.Close

		svc, err := auth.NewService(db)
		if err != nil {
			log.Fatalf("init auth service: %v", err)
		}
		authSvc = svc

	} else {
		ledgerSvc = ledger.NewInMemory()
		log.Println("running without persistent database; authentication disabled")
	}

	tmpl := mustParseTemplates()

	rp := httpapi.ReadyProbe{DB: db}

	evtStream := stream.New()

	// HTTP API setup.
	api := httpapi.New(rp, version, ledgerSvc, evtStream, tmpl, authSvc)

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
	if remoteClient != nil {
		_ = remoteClient.Close()
	}
	if storeClose != nil {
		_ = storeClose()
	} else if db != nil {
		_ = db.Close()
	}
	log.Println("Stopped")
}

func mustParseTemplates() *template.Template {
	base := template.New("base")
	patterns := []string{
		"web/templates/layout/*.html",
		"web/templates/parts/*.html",
		"web/templates/pages/*.html",
	}
	var err error
	for _, pattern := range patterns {
		base, err = base.ParseGlob(pattern)
		if err != nil {
			log.Fatalf("parse templates %s: %v", pattern, err)
		}
	}
	return base
}
