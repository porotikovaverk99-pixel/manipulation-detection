package audittrail

import (
	"testing"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type fakeAuditRepo struct {
	events []repository.AuditEventRecord
}

func (f *fakeAuditRepo) SaveAuditEvent(event repository.AuditEventRecord) (int64, error) {
	f.events = append(f.events, event)
	return int64(len(f.events)), nil
}

func TestCLIJobWritesStartedAndSucceededEvents(t *testing.T) {
	repo := &fakeAuditRepo{}
	job := NewCLIJob(repo, "case_scorer").
		WithSource("dataset", "pheme", "pheme", "eventcv").
		WithPayload(map[string]interface{}{"limit": 10})

	job.Start()
	job.Set("processed", 3)
	job.FinishAndExit()

	if len(repo.events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(repo.events))
	}
	started := repo.events[0]
	if started.Action != "cli.case_scorer" ||
		started.EntityType != "cli_command" ||
		started.EntityID != "case_scorer" ||
		started.Status != "started" ||
		started.SourceType != "dataset" ||
		started.DatasetName != "pheme" {
		t.Fatalf("unexpected started event: %#v", started)
	}

	finished := repo.events[1]
	if finished.Status != "succeeded" {
		t.Fatalf("expected succeeded event, got %#v", finished)
	}
	if finished.Payload["processed"] != 3 {
		t.Fatalf("expected processed payload, got %#v", finished.Payload)
	}
	if finished.RequestID == "" || finished.RequestID != started.RequestID {
		t.Fatalf("expected stable request id, started=%q finished=%q", started.RequestID, finished.RequestID)
	}
}

func TestCLIJobMarksFailure(t *testing.T) {
	repo := &fakeAuditRepo{}
	job := NewCLIJob(repo, "dataset_loader")

	job.Start()
	job.Failf("load failed: %s", "bad json")

	if job.status != "failed" || job.exitCode != 1 || job.err == nil {
		t.Fatalf("unexpected failed job state: status=%s exit=%d err=%v", job.status, job.exitCode, job.err)
	}

	job.exitCode = 0
	job.FinishAndExit()

	if got := repo.events[len(repo.events)-1]; got.Status != "failed" || got.ErrorMessage == "" {
		t.Fatalf("expected failed audit event with error, got %#v", got)
	}
}

func TestCLIJobMarksSkippedWithoutFailureExit(t *testing.T) {
	repo := &fakeAuditRepo{}
	job := NewCLIJob(repo, "case_ml_scorer")

	job.Start()
	job.Skipf("no cases selected")
	job.FinishAndExit()

	if got := repo.events[len(repo.events)-1]; got.Status != "skipped" || got.Payload["skip_reason"] != "no cases selected" {
		t.Fatalf("expected skipped audit event, got %#v", got)
	}
}
