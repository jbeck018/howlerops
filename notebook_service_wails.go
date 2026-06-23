package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/notebooksvc"
	"github.com/jbeck018/howlerops/pkg/database"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// WailsNotebookService exposes notebook CRUD and execution to the frontend. It
// wraps the Wails-free internal/notebooksvc orchestration, running SQL cells
// read-only via the database service.
type WailsNotebookService struct {
	deps  *SharedDeps
	store *storage.NotebookStore
}

// NewWailsNotebookService constructs the service; the store is wired in later.
func NewWailsNotebookService(deps *SharedDeps) *WailsNotebookService {
	return &WailsNotebookService{deps: deps}
}

// SetStore injects the notebook store once storage is ready.
func (s *WailsNotebookService) SetStore(store *storage.NotebookStore) { s.store = store }

func (s *WailsNotebookService) service() (*notebooksvc.Service, error) {
	if s.store == nil {
		return nil, fmt.Errorf("notebook storage is not initialized")
	}
	return notebooksvc.New(s.store, &notebookSQLExecutor{deps: s.deps}), nil
}

// SaveNotebook validates and persists a notebook definition, returning its ID.
func (s *WailsNotebookService) SaveNotebook(def notebooksvc.DefinitionDTO) (string, error) {
	svc, err := s.service()
	if err != nil {
		return "", err
	}
	return svc.Save(def.ToNotebook())
}

// ListNotebooks returns notebook summaries.
func (s *WailsNotebookService) ListNotebooks() ([]storage.NotebookSummary, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	return svc.List()
}

// GetNotebook loads a notebook definition by ID.
func (s *WailsNotebookService) GetNotebook(id string) (*notebooksvc.DefinitionDTO, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	nb, err := svc.Get(id)
	if err != nil {
		return nil, err
	}
	if nb == nil {
		return nil, fmt.Errorf("notebook %q not found", id)
	}
	def := notebooksvc.DefinitionFromNotebook(nb)
	return &def, nil
}

// DeleteNotebook removes a notebook.
func (s *WailsNotebookService) DeleteNotebook(id string) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.Delete(id)
}

// RunNotebookRequest drives RunNotebook.
type RunNotebookRequest struct {
	NotebookID  string         `json:"notebookId"`
	Inputs      map[string]any `json:"inputs"`
	StopOnError bool           `json:"stopOnError"`
}

// RunNotebook executes a stored notebook and returns the per-cell outputs.
func (s *WailsNotebookService) RunNotebook(req RunNotebookRequest) (*notebooksvc.RunResultDTO, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	res, err := svc.Run(context.Background(), req.NotebookID, req.Inputs, req.StopOnError)
	if err != nil {
		return nil, err
	}
	dto := notebooksvc.ResultToDTO(res)
	return &dto, nil
}

// notebookSQLExecutor adapts the database service to notebooksvc.SQLExecutor.
type notebookSQLExecutor struct{ deps *SharedDeps }

func (e *notebookSQLExecutor) Query(_ context.Context, connectionID, sql string) (*notebook.QueryResult, error) {
	res, err := e.deps.DatabaseService.ExecuteQuery(connectionID, sql, &database.QueryOptions{
		ReadOnly: true,
		Timeout:  60 * time.Second,
		Limit:    5000,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &notebook.QueryResult{}, nil
	}
	out := &notebook.QueryResult{Columns: res.Columns, RowCount: res.RowCount}
	for _, row := range res.Rows {
		m := make(map[string]any, len(res.Columns))
		for i, col := range res.Columns {
			if i < len(row) {
				m[col] = row[i]
			}
		}
		out.Rows = append(out.Rows, m)
	}
	return out, nil
}
