package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx driver for database/sql

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	if len(os.Args) < 2 {
		usageAndExit()
	}

	cmd := os.Args[1]

	// Parse flags for DB connection
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	dsn := fs.String("dsn", "", "Postgres DSN, e.g. postgres://user:pass@localhost:5432/dbname?sslmode=disable")
	migrationsDir := fs.String("dir", "", "Migrations directory")
	fs.Parse(os.Args[2:])

	if *dsn == "" {
		log.Fatal("missing required flag -dsn")
	}

	// Use "pgx" as driver name
	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		log.Fatalf("failed to open DB with pgx: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("failed to create migration driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://"+*migrationsDir,
		"postgres", driver)
	if err != nil {
		log.Fatalf("failed to create migrate instance: %v", err)
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate up failed: %v", err)
		}
		fmt.Println("✅ Migration up complete")

	case "down":
		if err := m.Steps(-1); err != nil {
			log.Fatalf("migrate down failed: %v", err)
		}
		fmt.Println("✅ Migration down complete")

	case "force":
		if fs.NArg() < 1 {
			log.Fatalf("force command requires version argument")
		}
		versionStr := fs.Arg(0)
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			log.Fatalf("invalid version number: %v", err)
		}
		if err := m.Force(version); err != nil {
			log.Fatalf("migrate force failed: %v", err)
		}
		fmt.Printf("✅ Migration version forcibly set to %d\n", version)

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("failed to get migration version: %v", err)
		}
		fmt.Printf("Current migration version: %d, dirty: %v\n", version, dirty)

	default:
		usageAndExit()
	}
}

func usageAndExit() {
	fmt.Fprintf(os.Stderr, `Usage: migrate <command> [flags]

Commands:
  up       Apply all up migrations
  down     Rollback last migration
  force    Set migration version without running migrations (requires version argument)
  version  Print current migration version

Flags:
  -dsn string    Postgres connection string (required)
  -dir string    Path to migrations directory (default "internal/db/migrations")

Examples:
  migrate up -dsn "postgres://user:pass@localhost:5432/dbname?sslmode=disable"
  migrate down -dsn "..."
  migrate force 3 -dsn "..."
  migrate version -dsn "..."

`)
	os.Exit(1)
}
