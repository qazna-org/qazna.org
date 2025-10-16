package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"qazna.org/internal/migrate"
)

func main() {
	log.SetFlags(0)
	var (
		dsn            = flag.String("dsn", os.Getenv("QAZNA_PG_DSN"), "PostgreSQL DSN")
		migrationsPath = flag.String("migrations", "ops/migrations/sql", "Path to SQL migrations")
		seedsPath      = flag.String("seeds", "ops/migrations/seeds", "Path to SQL seeds")
	)
	flag.Parse()

	if *dsn == "" {
		log.Fatal("missing DSN: provide via -dsn or QAZNA_PG_DSN")
	}
	if len(flag.Args()) == 0 {
		log.Fatal("usage: migrate [up|down|seed|status]")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	mgr := migrate.NewManager(db, *migrationsPath, *seedsPath)

	switch flag.Arg(0) {
	case "up":
		err = mgr.Up(ctx)
	case "down":
		err = mgr.Down(ctx)
	case "seed":
		err = mgr.Seed(ctx)
	case "status":
		var history []string
		history, err = mgr.Status(ctx)
		if err == nil {
			for _, item := range history {
				fmt.Println(item)
			}
		}
	default:
		log.Fatalf("unknown command %q", flag.Arg(0))
	}
	if err != nil {
		log.Fatalf("migrate %s: %v", flag.Arg(0), err)
	}
}
