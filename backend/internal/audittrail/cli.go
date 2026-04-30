package audittrail

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/porotikovaverk99-pixel/manipulation-detection/backend/internal/repository"
)

type Repository interface {
	SaveAuditEvent(repository.AuditEventRecord) (int64, error)
}

type CLIJob struct {
	repo         Repository
	command      string
	action       string
	requestID    string
	sourceType   string
	sourceName   string
	datasetName  string
	datasetSplit string
	payload      map[string]interface{}
	status       string
	err          error
	exitCode     int
	startedAt    time.Time
	started      bool
}

func NewCLIJob(repo Repository, command string) *CLIJob {
	command = strings.TrimSpace(command)
	if command == "" {
		command = "unknown"
	}
	return &CLIJob{
		repo:      repo,
		command:   command,
		action:    "cli." + command,
		requestID: fmt.Sprintf("cli-%s-%d", sanitizeRequestIDPart(command), time.Now().UTC().UnixNano()),
		payload: map[string]interface{}{
			"command": command,
			"pid":     os.Getpid(),
			"argv":    append([]string(nil), os.Args...),
		},
		status: "succeeded",
	}
}

func (j *CLIJob) WithSource(sourceType, sourceName, datasetName, datasetSplit string) *CLIJob {
	j.sourceType = strings.TrimSpace(sourceType)
	j.sourceName = strings.TrimSpace(sourceName)
	j.datasetName = strings.TrimSpace(datasetName)
	j.datasetSplit = strings.TrimSpace(datasetSplit)
	return j
}

func (j *CLIJob) WithPayload(payload map[string]interface{}) *CLIJob {
	for key, value := range payload {
		j.Set(key, value)
	}
	return j
}

func (j *CLIJob) Set(key string, value interface{}) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	j.payload[key] = value
}

func (j *CLIJob) Start() {
	j.startedAt = time.Now().UTC()
	j.started = true
	j.Set("started_at", j.startedAt.Format(time.RFC3339Nano))
	j.save("started", nil)
}

func (j *CLIJob) Skipf(format string, args ...interface{}) {
	j.status = "skipped"
	j.err = nil
	j.exitCode = 0
	message := fmt.Sprintf(format, args...)
	j.Set("skip_reason", message)
	log.Printf("%s", message)
}

func (j *CLIJob) Failf(format string, args ...interface{}) {
	j.status = "failed"
	j.err = fmt.Errorf(format, args...)
	j.exitCode = 1
	log.Printf("%v", j.err)
}

func (j *CLIJob) FinishAndExit() {
	j.Finish()
	if j.exitCode != 0 {
		os.Exit(j.exitCode)
	}
}

func (j *CLIJob) Finish() {
	if !j.started {
		j.Start()
	}
	if j.startedAt.IsZero() {
		j.startedAt = time.Now().UTC()
	}
	j.Set("duration_ms", time.Since(j.startedAt).Milliseconds())
	j.save(j.status, j.err)
}

func (j *CLIJob) save(status string, err error) {
	if j.repo == nil {
		return
	}
	event := repository.AuditEventRecord{
		ActorType:    "cli",
		ActorID:      hostActorID(),
		Action:       j.action,
		EntityType:   "cli_command",
		EntityID:     j.command,
		Status:       status,
		RequestID:    j.requestID,
		SourceType:   j.sourceType,
		SourceName:   j.sourceName,
		DatasetName:  j.datasetName,
		DatasetSplit: j.datasetSplit,
		Payload:      clonePayload(j.payload),
	}
	if err != nil {
		event.ErrorMessage = err.Error()
	}
	if _, saveErr := j.repo.SaveAuditEvent(event); saveErr != nil {
		log.Printf("save CLI audit event failed: %v", saveErr)
	}
}

func hostActorID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		return "local"
	}
	return hostname
}

func sanitizeRequestIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
			continue
		}
		b.WriteByte('-')
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "command"
	}
	return out
}

func clonePayload(payload map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}
