// Package notebooksvc orchestrates notebook persistence and execution: it loads
// a notebook definition from storage and runs it on the internal/notebook
// engine using host-provided SQL execution. It is Wails-free so the whole flow
// is unit-testable; the app layer supplies a SQLExecutor over the database
// service.
package notebooksvc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// SQLExecutor runs a notebook SQL cell (read-only).
type SQLExecutor interface {
	Query(ctx context.Context, connectionID, sql string) (*notebook.QueryResult, error)
}

// Store is the persistence surface, satisfied by *storage.NotebookStore.
type Store interface {
	SaveNotebook(*storage.Notebook) error
	GetNotebook(id string) (*storage.Notebook, error)
	ListNotebooks() ([]storage.NotebookSummary, error)
	DeleteNotebook(id string) error
}

// Service coordinates notebook CRUD and execution.
type Service struct {
	store Store
	db    SQLExecutor
}

// New constructs a Service.
func New(store Store, db SQLExecutor) *Service { return &Service{store: store, db: db} }

// Save validates and persists a notebook definition, returning its ID.
func (s *Service) Save(nb notebook.Notebook) (string, error) {
	if err := notebook.Validate(nb); err != nil {
		return "", err
	}
	def, err := json.Marshal(nb)
	if err != nil {
		return "", fmt.Errorf("notebooksvc: marshal definition: %w", err)
	}
	rec := &storage.Notebook{ID: nb.ID, Name: nb.Name, Description: nb.Description, Definition: def}
	if err := s.store.SaveNotebook(rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

// Get loads a notebook definition by ID (nil, nil when not found).
func (s *Service) Get(id string) (*notebook.Notebook, error) {
	rec, err := s.store.GetNotebook(id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	var nb notebook.Notebook
	if err := json.Unmarshal(rec.Definition, &nb); err != nil {
		return nil, fmt.Errorf("notebooksvc: unmarshal definition: %w", err)
	}
	nb.ID = rec.ID
	nb.Name = rec.Name
	nb.Description = rec.Description
	return &nb, nil
}

// List returns notebook summaries.
func (s *Service) List() ([]storage.NotebookSummary, error) { return s.store.ListNotebooks() }

// Delete removes a notebook.
func (s *Service) Delete(id string) error { return s.store.DeleteNotebook(id) }

// Run executes a stored notebook with the given inputs and returns the per-cell
// outputs.
func (s *Service) Run(ctx context.Context, id string, inputs map[string]any, stopOnError bool) (*notebook.RunResult, error) {
	nb, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if nb == nil {
		return nil, fmt.Errorf("notebooksvc: notebook %q not found", id)
	}
	return notebook.Execute(ctx, *nb, inputs, queryAdapter{s.db}, notebook.Options{StopOnError: stopOnError})
}

type queryAdapter struct{ db SQLExecutor }

func (a queryAdapter) RunSQL(ctx context.Context, connID, sql string) (*notebook.QueryResult, error) {
	return a.db.Query(ctx, connID, sql)
}
