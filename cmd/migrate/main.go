package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	"github.com/pressly/goose/v3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|status>")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}

	command := os.Args[1]
	ctx := context.Background()

	switch command {
	case "up":
		if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
			log.Fatalf("migrate up: %v", err)
		}
	case "down":
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			log.Fatalf("migrate down: %v", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
			log.Fatalf("migrate status: %v", err)
		}
	default:
		log.Fatalf("unknown command: %s", command)
	}
}
