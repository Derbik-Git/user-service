package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Derbik-Git/user-service/internal/migrate"
)

func main() {
	var (
		dsn            string
		migrationsPath string
		timeout        time.Duration
	)

	flag.StringVar(&dsn, "dsn", "", "PostgreSQL DSN (required)")
	flag.StringVar(&migrationsPath, "migrations", "./migrations", "Path to migrations folder")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "DB ping timeout")
	flag.Parse()

	if dsn == "" {
		log.Fatal("dsn is required, example: -dsn postgres://user:pass@localhost:5432/users?sslmode=disable")
	}

	// Важно: драйвер "pgx" регистрируется импортом _ "github.com/jackc/pgx/v5/stdlib"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatal("ping db: %v", err)
	}

	if err := migrate.MigrateUp(db, migrationsPath); err != nil {
		log.Fatal("migrate up: %v", err)
	}

	log.Println("migrations applied successfully")

}
