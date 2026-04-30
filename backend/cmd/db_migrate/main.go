package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	applog "github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/logger"
	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
	"github.com/pressly/goose/v3"
)

func main() {
	logFile, err := applog.ConfigureStandardLog("db_migrate")
	if err != nil {
		log.Printf("configure file logging failed: %v", err)
	}
	if logFile != nil {
		defer logFile.Close()
	}

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

	runErr := run(command, db, *dir)
	auditMigration(dsn, command, *dir, db, runErr)
	if runErr != nil {
		log.Fatalf("goose %s failed: %v", command, runErr)
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

func auditMigration(dsn, command, dir string, db *sql.DB, runErr error) {
	repo, err := repository.NewPostgresDB(dsn)
	if err != nil {
		log.Printf("migration audit skipped: connect repository failed: %v", err)
		return
	}
	defer repo.Close()

	status := "succeeded"
	var errorMessage string
	if runErr != nil {
		status = "failed"
		errorMessage = runErr.Error()
	}

	payload := map[string]interface{}{
		"command":        command,
		"migrations_dir": dir,
		"executed_at":    time.Now().UTC().Format(time.RFC3339Nano),
	}
	if version, err := goose.GetDBVersion(db); err == nil {
		payload["schema_version"] = version
	}

	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "local"
	}

	if _, err := repo.SaveAuditEvent(repository.AuditEventRecord{
		ActorType:    "cli",
		ActorID:      hostname,
		Action:       "cli.db_migrate",
		EntityType:   "cli_command",
		EntityID:     "db_migrate",
		Status:       status,
		RequestID:    fmt.Sprintf("cli-db-migrate-%d", time.Now().UTC().UnixNano()),
		Payload:      payload,
		ErrorMessage: errorMessage,
	}); err != nil {
		log.Printf("migration audit skipped: save event failed: %v", err)
	}
}
