package repository

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPostgresIngestionRunRoundTripIntegration(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()

	sourceName := fmt.Sprintf("integration_test_%d", time.Now().UTC().UnixNano())
	runID, err := db.StartIngestionRun("dataset", sourceName, "integration_dataset", "dev")
	if err != nil {
		t.Fatalf("start ingestion run: %v", err)
	}
	defer func() {
		if _, err := db.db.Exec(`DELETE FROM ingestion_runs WHERE id = $1`, runID); err != nil {
			t.Fatalf("cleanup ingestion run: %v", err)
		}
	}()

	if err := db.FinishIngestionRun(runID, "completed", "integration smoke"); err != nil {
		t.Fatalf("finish ingestion run: %v", err)
	}

	runs, err := db.GetIngestionRuns(50)
	if err != nil {
		t.Fatalf("get ingestion runs: %v", err)
	}

	for _, run := range runs {
		if run.ID != runID {
			continue
		}
		if run.SourceType != "dataset" ||
			run.SourceName != sourceName ||
			run.DatasetName != "integration_dataset" ||
			run.DatasetSplit != "dev" ||
			run.Status != "completed" ||
			run.Notes != "integration smoke" ||
			run.FinishedAt == nil {
			t.Fatalf("unexpected ingestion run: %#v", run)
		}
		return
	}

	t.Fatalf("ingestion run %d not found in recent runs", runID)
}

func TestPostgresAuditEventRoundTripIntegration(t *testing.T) {
	db := openIntegrationDB(t)
	defer db.Close()

	requestID := fmt.Sprintf("request_%d", time.Now().UTC().UnixNano())
	eventID, err := db.SaveAuditEvent(AuditEventRecord{
		ActorType:  "api_client",
		ActorID:    "127.0.0.1:12345",
		Action:     "http_request",
		EntityType: "http_endpoint",
		EntityID:   "/api/cases",
		Status:     "succeeded",
		RequestID:  requestID,
		SourceName: "integration",
		Payload: map[string]interface{}{
			"method": "GET",
			"status": 200,
		},
	})
	if err != nil {
		t.Fatalf("save audit event: %v", err)
	}
	defer func() {
		if _, err := db.db.Exec(`DELETE FROM audit_events WHERE id = $1`, eventID); err != nil {
			t.Fatalf("cleanup audit event: %v", err)
		}
	}()

	var action, status, storedRequestID string
	if err := db.db.QueryRow(`
		SELECT action, status, request_id
		FROM audit_events
		WHERE id = $1
	`, eventID).Scan(&action, &status, &storedRequestID); err != nil {
		t.Fatalf("read audit event: %v", err)
	}
	if action != "http_request" || status != "succeeded" || storedRequestID != requestID {
		t.Fatalf("unexpected audit row: action=%q status=%q request_id=%q", action, status, storedRequestID)
	}
}

func openIntegrationDB(t *testing.T) *PostgresDB {
	t.Helper()
	if os.Getenv("RUN_DB_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_DB_INTEGRATION_TESTS=1 to run PostgreSQL integration tests")
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URI")
	}
	if dsn == "" {
		t.Fatal("DATABASE_URL or DATABASE_URI is required")
	}

	db, err := NewPostgresDB(dsn)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	return db
}
