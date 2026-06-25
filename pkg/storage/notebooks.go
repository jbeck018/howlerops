package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Notebook is the persisted form of an interactive, cell-based document. The
// Definition holds the serialized inputs + cells (internal/notebook.Notebook),
// kept opaque here so storage stays decoupled from the execution engine.
type Notebook struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Definition    json.RawMessage `json:"definition"`
	LastRunAt     *time.Time      `json:"lastRunAt,omitempty"`
	LastRunStatus string          `json:"lastRunStatus,omitempty"` // success | failed | partial
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// NotebookSummary is the lightweight listing shape.
type NotebookSummary struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	LastRunAt     *time.Time `json:"lastRunAt,omitempty"`
	LastRunStatus string     `json:"lastRunStatus,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// NotebookRun records one execution of a notebook in the run history.
type NotebookRun struct {
	ID         string          `json:"id"`
	NotebookID string          `json:"notebookId"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt,omitempty"`
	Status     string          `json:"status"` // success | failed | partial
	DryRun     bool            `json:"dryRun"`
	Outcomes   json.RawMessage `json:"outcomes,omitempty"`
}

// NotebookStore persists notebook definitions to SQLite/Turso.
type NotebookStore struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewNotebookStore constructs a store.
func NewNotebookStore(db *sql.DB, logger *logrus.Logger) *NotebookStore {
	return &NotebookStore{db: db, logger: logger}
}

// EnsureSchema creates the notebooks table and index if absent.
func (s *NotebookStore) EnsureSchema() error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	stmt := `
CREATE TABLE IF NOT EXISTS notebooks (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	description TEXT,
	definition TEXT NOT NULL,
	last_run_at DATETIME,
	last_run_status TEXT,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notebooks_updated_at ON notebooks(updated_at DESC);

CREATE TABLE IF NOT EXISTS notebook_runs (
	id TEXT PRIMARY KEY,
	notebook_id TEXT NOT NULL,
	started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at DATETIME,
	status TEXT NOT NULL,
	dry_run BOOLEAN NOT NULL DEFAULT 0,
	outcomes TEXT,
	FOREIGN KEY (notebook_id) REFERENCES notebooks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_notebook_runs ON notebook_runs(notebook_id, started_at DESC);
`
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("failed to ensure notebook schema: %w", err)
	}
	// Migrate existing installs: add the last-run columns if the table predates
	// them. SQLite has no "ADD COLUMN IF NOT EXISTS", so ignore duplicate errors.
	for _, alter := range []string{
		`ALTER TABLE notebooks ADD COLUMN last_run_at DATETIME`,
		`ALTER TABLE notebooks ADD COLUMN last_run_status TEXT`,
	} {
		if _, err := s.db.Exec(alter); err != nil && !isDuplicateColumnErr(err) {
			return fmt.Errorf("failed to migrate notebook schema: %w", err)
		}
	}
	return nil
}

// isDuplicateColumnErr reports whether an ALTER TABLE ADD COLUMN failed because
// the column already exists (the expected, ignorable case on re-run).
func isDuplicateColumnErr(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

// SaveNotebook inserts or updates a notebook.
func (s *NotebookStore) SaveNotebook(nb *Notebook) error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	if nb == nil {
		return errors.New("notebook is nil")
	}
	if nb.Name == "" {
		return errors.New("notebook name is required")
	}
	if len(nb.Definition) == 0 {
		return errors.New("notebook definition is required")
	}
	if nb.ID == "" {
		nb.ID = uuid.NewString()
	}
	if nb.CreatedAt.IsZero() {
		nb.CreatedAt = time.Now().UTC()
	}
	nb.UpdatedAt = time.Now().UTC()

	_, err := s.db.Exec(`
INSERT INTO notebooks (id, name, description, definition, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	description = excluded.description,
	definition = excluded.definition,
	updated_at = excluded.updated_at;`,
		nb.ID, nb.Name, nb.Description, string(nb.Definition), nb.CreatedAt, nb.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save notebook: %w", err)
	}
	return nil
}

// GetNotebook loads a notebook by ID, returning (nil, nil) when not found.
func (s *NotebookStore) GetNotebook(id string) (*Notebook, error) {
	if s.db == nil {
		return nil, errors.New("notebook storage database not available")
	}
	row := s.db.QueryRow(`SELECT id, name, description, definition, last_run_at, last_run_status, created_at, updated_at FROM notebooks WHERE id = ?`, id)
	var (
		nb            Notebook
		definition    string
		lastRunAt     sql.NullTime
		lastRunStatus sql.NullString
	)
	err := row.Scan(&nb.ID, &nb.Name, &nb.Description, &definition, &lastRunAt, &lastRunStatus, &nb.CreatedAt, &nb.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load notebook: %w", err)
	}
	nb.Definition = json.RawMessage(definition)
	if lastRunAt.Valid {
		t := lastRunAt.Time
		nb.LastRunAt = &t
	}
	if lastRunStatus.Valid {
		nb.LastRunStatus = lastRunStatus.String
	}
	return &nb, nil
}

// ListNotebooks returns summaries ordered by most recently updated.
func (s *NotebookStore) ListNotebooks() ([]NotebookSummary, error) {
	if s.db == nil {
		return nil, errors.New("notebook storage database not available")
	}
	rows, err := s.db.Query(`SELECT id, name, description, last_run_at, last_run_status, updated_at FROM notebooks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list notebooks: %w", err)
	}
	defer rows.Close()

	var out []NotebookSummary
	for rows.Next() {
		var (
			ns            NotebookSummary
			lastRunAt     sql.NullTime
			lastRunStatus sql.NullString
		)
		if err := rows.Scan(&ns.ID, &ns.Name, &ns.Description, &lastRunAt, &lastRunStatus, &ns.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notebook: %w", err)
		}
		if lastRunAt.Valid {
			t := lastRunAt.Time
			ns.LastRunAt = &t
		}
		if lastRunStatus.Valid {
			ns.LastRunStatus = lastRunStatus.String
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// DeleteNotebook removes a notebook (and, via cascade, its run history).
func (s *NotebookStore) DeleteNotebook(id string) error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	if _, err := s.db.Exec(`DELETE FROM notebooks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete notebook: %w", err)
	}
	return nil
}

// UpdateRunState records the outcome of the most recent run on the notebook row.
func (s *NotebookStore) UpdateRunState(id, status string, runAt time.Time) error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	_, err := s.db.Exec(`
UPDATE notebooks SET last_run_status = ?, last_run_at = ?, updated_at = ? WHERE id = ?`,
		status, runAt.UTC(), time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update notebook run state: %w", err)
	}
	return nil
}

// SaveRun records a notebook execution in the history.
func (s *NotebookStore) SaveRun(run *NotebookRun) error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	if run == nil {
		return errors.New("run is nil")
	}
	if run.NotebookID == "" {
		return errors.New("run requires a notebook ID")
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
INSERT INTO notebook_runs (id, notebook_id, started_at, finished_at, status, dry_run, outcomes)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.NotebookID, run.StartedAt.UTC(), finished, run.Status, run.DryRun, outcomes)
	if err != nil {
		return fmt.Errorf("failed to save notebook run: %w", err)
	}
	return nil
}

// ListRuns returns recent runs for a notebook, newest first.
func (s *NotebookStore) ListRuns(notebookID string, limit int) ([]NotebookRun, error) {
	if s.db == nil {
		return nil, errors.New("notebook storage database not available")
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
SELECT id, notebook_id, started_at, finished_at, status, dry_run, outcomes
FROM notebook_runs WHERE notebook_id = ? ORDER BY started_at DESC LIMIT ?`, notebookID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list notebook runs: %w", err)
	}
	defer rows.Close()

	var out []NotebookRun
	for rows.Next() {
		var (
			run        NotebookRun
			finishedAt sql.NullTime
			outcomes   sql.NullString
		)
		if err := rows.Scan(&run.ID, &run.NotebookID, &run.StartedAt, &finishedAt, &run.Status, &run.DryRun, &outcomes); err != nil {
			return nil, fmt.Errorf("failed to scan notebook run: %w", err)
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
