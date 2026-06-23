package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Runbook is the persisted form of a parameterized task. The Definition holds
// the serialized inputs + steps (the internal/runbook.Runbook), kept opaque here
// so the storage layer does not depend on the execution engine.
type Runbook struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Definition    json.RawMessage `json:"definition"`
	LastRunAt     *time.Time      `json:"lastRunAt,omitempty"`
	LastRunStatus string          `json:"lastRunStatus,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// RunbookSummary is the lightweight listing shape.
type RunbookSummary struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	LastRunAt     *time.Time `json:"lastRunAt,omitempty"`
	LastRunStatus string     `json:"lastRunStatus,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// RunbookRun is one recorded execution of a runbook.
type RunbookRun struct {
	ID         string          `json:"id"`
	RunbookID  string          `json:"runbookId"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Status     string          `json:"status"` // success | failed | partial
	DryRun     bool            `json:"dryRun"`
	Outcomes   json.RawMessage `json:"outcomes,omitempty"`
}

// RunbookStore persists runbook definitions and their run history to SQLite/Turso.
type RunbookStore struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewRunbookStore constructs a store over the given database.
func NewRunbookStore(db *sql.DB, logger *logrus.Logger) *RunbookStore {
	return &RunbookStore{db: db, logger: logger}
}

// EnsureSchema creates the runbook tables and indexes if they do not exist.
func (s *RunbookStore) EnsureSchema() error {
	if s.db == nil {
		return errors.New("runbook storage database not available")
	}
	statement := `
CREATE TABLE IF NOT EXISTS runbooks (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	definition TEXT NOT NULL,
	last_run_at DATETIME,
	last_run_status TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_runbooks_updated_at ON runbooks(updated_at DESC);

CREATE TABLE IF NOT EXISTS runbook_runs (
	id TEXT PRIMARY KEY,
	runbook_id TEXT NOT NULL,
	started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at DATETIME,
	status TEXT NOT NULL,
	dry_run BOOLEAN NOT NULL DEFAULT 0,
	outcomes TEXT,
	FOREIGN KEY (runbook_id) REFERENCES runbooks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_runbook_runs ON runbook_runs(runbook_id, started_at DESC);
`
	if _, err := s.db.Exec(statement); err != nil {
		return fmt.Errorf("failed to ensure runbook schema: %w", err)
	}
	return nil
}

// SaveRunbook inserts or updates a runbook definition, assigning an ID and
// timestamps as needed.
func (s *RunbookStore) SaveRunbook(rb *Runbook) error {
	if s.db == nil {
		return errors.New("runbook storage database not available")
	}
	if rb == nil {
		return errors.New("runbook is nil")
	}
	if rb.Name == "" {
		return errors.New("runbook name is required")
	}
	if len(rb.Definition) == 0 {
		return errors.New("runbook definition is required")
	}
	if rb.ID == "" {
		rb.ID = uuid.NewString()
	}
	if rb.CreatedAt.IsZero() {
		rb.CreatedAt = time.Now().UTC()
	}
	rb.UpdatedAt = time.Now().UTC()

	var lastRun interface{}
	if rb.LastRunAt != nil {
		lastRun = rb.LastRunAt.UTC()
	}
	var lastRunStatus interface{}
	if rb.LastRunStatus != "" {
		lastRunStatus = rb.LastRunStatus
	}

	// On update, preserve the existing run state when the incoming record does
	// not carry one. A plain edit of a runbook (rename, redefine) goes through
	// SaveRunbook with empty last-run fields, and must not clobber the run
	// history recorded by UpdateRunState.
	query := `
INSERT INTO runbooks (id, name, description, definition, last_run_at, last_run_status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	description = excluded.description,
	definition = excluded.definition,
	last_run_at = COALESCE(excluded.last_run_at, last_run_at),
	last_run_status = COALESCE(excluded.last_run_status, last_run_status),
	updated_at = excluded.updated_at;
`
	_, err := s.db.Exec(query, rb.ID, rb.Name, rb.Description, string(rb.Definition),
		lastRun, lastRunStatus, rb.CreatedAt, rb.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save runbook: %w", err)
	}
	return nil
}

// GetRunbook loads a runbook by ID. It returns (nil, nil) when not found.
func (s *RunbookStore) GetRunbook(id string) (*Runbook, error) {
	if s.db == nil {
		return nil, errors.New("runbook storage database not available")
	}
	row := s.db.QueryRow(`
SELECT id, name, description, definition, last_run_at, last_run_status, created_at, updated_at
FROM runbooks WHERE id = ?`, id)

	var (
		rb            Runbook
		definition    string
		lastRunAt     sql.NullTime
		lastRunStatus sql.NullString
	)
	err := row.Scan(&rb.ID, &rb.Name, &rb.Description, &definition, &lastRunAt, &lastRunStatus, &rb.CreatedAt, &rb.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load runbook: %w", err)
	}
	rb.Definition = json.RawMessage(definition)
	if lastRunAt.Valid {
		t := lastRunAt.Time
		rb.LastRunAt = &t
	}
	if lastRunStatus.Valid {
		rb.LastRunStatus = lastRunStatus.String
	}
	return &rb, nil
}

// ListRunbooks returns lightweight summaries ordered by most recently updated.
func (s *RunbookStore) ListRunbooks() ([]RunbookSummary, error) {
	if s.db == nil {
		return nil, errors.New("runbook storage database not available")
	}
	rows, err := s.db.Query(`
SELECT id, name, description, last_run_at, last_run_status, updated_at
FROM runbooks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list runbooks: %w", err)
	}
	defer rows.Close()

	var out []RunbookSummary
	for rows.Next() {
		var (
			rs            RunbookSummary
			lastRunAt     sql.NullTime
			lastRunStatus sql.NullString
		)
		if err := rows.Scan(&rs.ID, &rs.Name, &rs.Description, &lastRunAt, &lastRunStatus, &rs.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan runbook: %w", err)
		}
		if lastRunAt.Valid {
			t := lastRunAt.Time
			rs.LastRunAt = &t
		}
		if lastRunStatus.Valid {
			rs.LastRunStatus = lastRunStatus.String
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

// DeleteRunbook removes a runbook (and its run history via cascade).
func (s *RunbookStore) DeleteRunbook(id string) error {
	if s.db == nil {
		return errors.New("runbook storage database not available")
	}
	if _, err := s.db.Exec(`DELETE FROM runbooks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete runbook: %w", err)
	}
	return nil
}

// UpdateRunState records the latest run status/time on the runbook row.
func (s *RunbookStore) UpdateRunState(id, status string, runAt time.Time) error {
	if s.db == nil {
		return errors.New("runbook storage database not available")
	}
	_, err := s.db.Exec(`
UPDATE runbooks SET last_run_status = ?, last_run_at = ?, updated_at = ? WHERE id = ?`,
		status, runAt.UTC(), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update runbook run state: %w", err)
	}
	return nil
}

// SaveRun records a runbook execution in the history.
func (s *RunbookStore) SaveRun(run *RunbookRun) error {
	if s.db == nil {
		return errors.New("runbook storage database not available")
	}
	if run == nil {
		return errors.New("run is nil")
	}
	if run.RunbookID == "" {
		return errors.New("run requires a runbook ID")
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	var finished interface{}
	if run.FinishedAt != nil {
		finished = run.FinishedAt.UTC()
	}
	var outcomes interface{}
	if len(run.Outcomes) > 0 {
		outcomes = string(run.Outcomes)
	}
	_, err := s.db.Exec(`
INSERT INTO runbook_runs (id, runbook_id, started_at, finished_at, status, dry_run, outcomes)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.RunbookID, run.StartedAt.UTC(), finished, run.Status, run.DryRun, outcomes)
	if err != nil {
		return fmt.Errorf("failed to save runbook run: %w", err)
	}
	return nil
}

// ListRuns returns recent runs for a runbook, newest first.
func (s *RunbookStore) ListRuns(runbookID string, limit int) ([]RunbookRun, error) {
	if s.db == nil {
		return nil, errors.New("runbook storage database not available")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
SELECT id, runbook_id, started_at, finished_at, status, dry_run, outcomes
FROM runbook_runs WHERE runbook_id = ? ORDER BY started_at DESC LIMIT ?`, runbookID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list runbook runs: %w", err)
	}
	defer rows.Close()

	var out []RunbookRun
	for rows.Next() {
		var (
			run        RunbookRun
			finishedAt sql.NullTime
			outcomes   sql.NullString
		)
		if err := rows.Scan(&run.ID, &run.RunbookID, &run.StartedAt, &finishedAt, &run.Status, &run.DryRun, &outcomes); err != nil {
			return nil, fmt.Errorf("failed to scan runbook run: %w", err)
		}
		if finishedAt.Valid {
			t := finishedAt.Time
			run.FinishedAt = &t
		}
		if outcomes.Valid {
			run.Outcomes = json.RawMessage(outcomes.String)
		}
		out = append(out, run)
	}
	return out, rows.Err()
}
