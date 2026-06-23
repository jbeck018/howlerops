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

// Notebook is the persisted form of an exploratory, cell-based document. The
// Definition holds the serialized inputs + cells (internal/notebook.Notebook),
// kept opaque here so storage stays decoupled from the execution engine.
type Notebook struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Definition  json.RawMessage `json:"definition"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// NotebookSummary is the lightweight listing shape.
type NotebookSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updatedAt"`
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
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notebooks_updated_at ON notebooks(updated_at DESC);
`
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("failed to ensure notebook schema: %w", err)
	}
	return nil
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
	row := s.db.QueryRow(`SELECT id, name, description, definition, created_at, updated_at FROM notebooks WHERE id = ?`, id)
	var (
		nb         Notebook
		definition string
	)
	err := row.Scan(&nb.ID, &nb.Name, &nb.Description, &definition, &nb.CreatedAt, &nb.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load notebook: %w", err)
	}
	nb.Definition = json.RawMessage(definition)
	return &nb, nil
}

// ListNotebooks returns summaries ordered by most recently updated.
func (s *NotebookStore) ListNotebooks() ([]NotebookSummary, error) {
	if s.db == nil {
		return nil, errors.New("notebook storage database not available")
	}
	rows, err := s.db.Query(`SELECT id, name, description, updated_at FROM notebooks ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list notebooks: %w", err)
	}
	defer rows.Close()

	var out []NotebookSummary
	for rows.Next() {
		var ns NotebookSummary
		if err := rows.Scan(&ns.ID, &ns.Name, &ns.Description, &ns.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan notebook: %w", err)
		}
		out = append(out, ns)
	}
	return out, rows.Err()
}

// DeleteNotebook removes a notebook.
func (s *NotebookStore) DeleteNotebook(id string) error {
	if s.db == nil {
		return errors.New("notebook storage database not available")
	}
	if _, err := s.db.Exec(`DELETE FROM notebooks WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete notebook: %w", err)
	}
	return nil
}
