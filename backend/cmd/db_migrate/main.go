package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func main() {
	dir := flag.String("dir", getenvDefault("MIGRATIONS_DIR", "./migrations"), "path to goose migrations directory")
	flag.Parse()

	command := "up"
	if flag.NArg() > 0 {
		command = strings.ToLower(flag.Arg(0))
	}

	dsn := os.Getenv("DATABASE_URL")
	if strings.TrimSpace(dsn) == "" {
		dsn = os.Getenv("DATABASE_URI")
	}
	if strings.TrimSpace(dsn) == "" {
		log.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("ping database: %v", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set goose dialect: %v", err)
	}

	if err := run(command, db, *dir); err != nil {
		log.Fatalf("goose %s failed: %v", command, err)
	}
}

func run(command string, db *sql.DB, dir string) error {
	switch command {
	case "up":
		return goose.Up(db, dir)
	case "down":
		return goose.Down(db, dir)
	case "status":
		return goose.Status(db, dir)
	case "version":
		version, err := goose.GetDBVersion(db)
		if err != nil {
			return err
		}
		fmt.Printf("goose db version: %d\n", version)
		return nil
	case "reset":
		return goose.Reset(db, dir)
	default:
		return fmt.Errorf("unknown command %q (supported: up, down, status, version, reset)", command)
	}
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
