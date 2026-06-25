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
// wraps the Wails-free internal/notebooksvc orchestration: read SQL cells run
// read-only, action cells run as writes (gated by the dry-run/approval guard),
// notify cells emit events, and cross-cell composition runs on the DuckDB
// compute engine when available.
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
	exec := &notebookSQLExecutor{deps: s.deps}
	return notebooksvc.New(notebooksvc.Deps{
		Store:   s.store,
		Query:   exec,
		Action:  exec,
		Notify:  &notebookNotifier{deps: s.deps},
		Stagers: newNotebookStagers(s.deps),
	}), nil
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

// DeleteNotebook removes a notebook and its history.
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
	// DryRun plans writes/notifications without performing them.
	DryRun bool `json:"dryRun"`
	// AutoApprove permits action (write) cells without an interactive prompt; the
	// frontend confirms with the user before sending this.
	AutoApprove bool `json:"autoApprove"`
	// Only restricts execution to these cell IDs plus their descendants — the
	// reactive re-run triggered when a single cell changes. Empty = full run.
	Only []string `json:"only,omitempty"`
}

// RunNotebook executes a stored notebook and returns the per-cell outputs.
func (s *WailsNotebookService) RunNotebook(req RunNotebookRequest) (*notebooksvc.RunResultDTO, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	res, err := svc.Run(context.Background(), req.NotebookID, notebooksvc.RunOptions{
		Inputs:         req.Inputs,
		StopOnError:    req.StopOnError,
		DryRun:         req.DryRun,
		AutoApprove:    req.AutoApprove,
		Only:           req.Only,
		DefaultTimeout: 60 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	dto := notebooksvc.ResultToDTO(res)
	return &dto, nil
}

// NotebookHistory returns recent runs for a notebook.
func (s *WailsNotebookService) NotebookHistory(id string, limit int) ([]storage.NotebookRun, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	return svc.History(id, limit)
}

// notebookSQLExecutor adapts the database service to the notebook engine's read
// (Query) and write (Exec) capabilities.
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
	return toNotebookQueryResult(res), nil
}

func (e *notebookSQLExecutor) Exec(_ context.Context, connectionID, sql string) (int64, error) {
	res, err := e.deps.DatabaseService.ExecuteQuery(connectionID, sql, &database.QueryOptions{
		ReadOnly: false,
		Timeout:  60 * time.Second,
	})
	if err != nil {
		return 0, err
	}
	return res.Affected, nil
}

// toNotebookQueryResult converts the positional database result into the
// column-keyed shape the notebook engine consumes.
func toNotebookQueryResult(res *database.QueryResult) *notebook.QueryResult {
	if res == nil {
		return &notebook.QueryResult{}
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
	return out
}

// notebookNotifier adapts the event emitter to the notebook engine's Notifier.
type notebookNotifier struct{ deps *SharedDeps }

func (n *notebookNotifier) Notify(_ context.Context, channel, message string) error {
	n.deps.emitEvent("notebook:notification", map[string]interface{}{
		"channel": channel,
		"message": message,
	})
	return nil
}
