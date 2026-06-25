// Package notebooksvc orchestrates notebook persistence and execution: it loads
// a notebook definition from storage and runs it on the internal/notebook
// engine using host-provided capabilities (read queries, writes, notifications,
// approval, and the DuckDB compute engine for cross-cell composition). It is
// Wails-free so the whole flow is unit-testable; the app layer supplies the
// adapters over the database service.
package notebooksvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// SQLExecutor runs a read-only SQL cell against a real connection.
type SQLExecutor interface {
	Query(ctx context.Context, connectionID, sql string) (*notebook.QueryResult, error)
}

// ActionExecutor runs a write/mutation cell against a real connection.
type ActionExecutor interface {
	Exec(ctx context.Context, connectionID, sql string) (rowsAffected int64, err error)
}

// Notifier delivers a notify cell's message.
type Notifier interface {
	Notify(ctx context.Context, channel, message string) error
}

// StagerProvider returns the per-notebook DuckDB compute session used for
// cross-cell composition. Returning the same Stager across runs of one notebook
// keeps staged tables warm for reactive partial re-runs. May return NoStager
// when DuckDB is unavailable.
type StagerProvider interface {
	StagerFor(notebookID string) notebook.Stager
}

// Store is the persistence surface, satisfied by *storage.NotebookStore.
type Store interface {
	SaveNotebook(*storage.Notebook) error
	GetNotebook(id string) (*storage.Notebook, error)
	ListNotebooks() ([]storage.NotebookSummary, error)
	DeleteNotebook(id string) error
	SaveRun(*storage.NotebookRun) error
	ListRuns(notebookID string, limit int) ([]storage.NotebookRun, error)
	UpdateRunState(id, status string, runAt time.Time) error
}

// Deps wires the Service's capabilities. Only Store and Query are required; the
// rest enable writes, notifications, approval, and composition respectively.
type Deps struct {
	Store   Store
	Query   SQLExecutor
	Action  ActionExecutor
	Notify  Notifier
	Approve notebook.ApproveFunc
	Stagers StagerProvider
}

// Service coordinates notebook CRUD and execution.
type Service struct {
	deps Deps
}

// New constructs a Service from its dependencies.
func New(deps Deps) *Service { return &Service{deps: deps} }

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
	if err := s.deps.Store.SaveNotebook(rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

// Get loads a notebook definition by ID (nil, nil when not found).
func (s *Service) Get(id string) (*notebook.Notebook, error) {
	rec, err := s.deps.Store.GetNotebook(id)
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
func (s *Service) List() ([]storage.NotebookSummary, error) { return s.deps.Store.ListNotebooks() }

// Delete removes a notebook.
func (s *Service) Delete(id string) error { return s.deps.Store.DeleteNotebook(id) }

// History returns recent runs for a notebook, newest first.
func (s *Service) History(id string, limit int) ([]storage.NotebookRun, error) {
	return s.deps.Store.ListRuns(id, limit)
}

// RunOptions tune a notebook run.
type RunOptions struct {
	Inputs         map[string]any
	DryRun         bool
	AutoApprove    bool
	StopOnError    bool
	MaxParallel    int
	DefaultTimeout time.Duration
	// Only restricts execution to the given cell IDs plus their descendants (a
	// reactive partial re-run). Empty means a full run.
	Only []string
}

// Run executes a stored notebook and returns the per-cell outputs, recording the
// run in the history (unless it is a dry run or a partial re-run).
func (s *Service) Run(ctx context.Context, id string, opts RunOptions) (*notebook.RunResult, error) {
	nb, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if nb == nil {
		return nil, fmt.Errorf("notebooksvc: notebook %q not found", id)
	}

	deps := notebook.Deps{
		Query:   queryAdapter{s.deps.Query},
		Approve: s.deps.Approve,
	}
	if s.deps.Action != nil {
		deps.Action = actionAdapter{s.deps.Action}
	}
	if s.deps.Notify != nil {
		deps.Notify = s.deps.Notify
	}
	if s.deps.Stagers != nil {
		deps.Stage = s.deps.Stagers.StagerFor(id)
	}

	started := time.Now().UTC()
	res, err := notebook.Execute(ctx, *nb, opts.Inputs, deps, notebook.Options{
		StopOnError:    opts.StopOnError,
		DryRun:         opts.DryRun,
		AutoApprove:    opts.AutoApprove,
		MaxParallel:    opts.MaxParallel,
		DefaultTimeout: opts.DefaultTimeout,
		Only:           opts.Only,
	})
	if err != nil {
		return nil, err
	}

	// Record history for full, non-dry runs (partial reactive re-runs are
	// transient and not worth a history row).
	if !opts.DryRun && len(opts.Only) == 0 {
		s.recordRun(id, started, res)
	}
	return res, nil
}

// recordRun persists the run outcome and updates the notebook's last-run state.
// Failures here are non-fatal: a run that executed should still return.
func (s *Service) recordRun(id string, started time.Time, res *notebook.RunResult) {
	status := runStatus(res)
	finished := time.Now().UTC()
	outcomes, _ := json.Marshal(ResultToDTO(res).Cells)
	_ = s.deps.Store.SaveRun(&storage.NotebookRun{
		NotebookID: id,
		StartedAt:  started,
		FinishedAt: &finished,
		Status:     status,
		DryRun:     res.DryRun,
		Outcomes:   outcomes,
	})
	_ = s.deps.Store.UpdateRunState(id, status, started)
}

// runStatus reduces per-cell outcomes to success | failed | partial.
func runStatus(res *notebook.RunResult) string {
	if !res.Failed {
		return "success"
	}
	for _, c := range res.Cells {
		if c.Status == notebook.StatusSuccess {
			return "partial"
		}
	}
	return "failed"
}

type queryAdapter struct{ db SQLExecutor }

func (a queryAdapter) RunSQL(ctx context.Context, connID, sql string) (*notebook.QueryResult, error) {
	if a.db == nil {
		return nil, fmt.Errorf("notebooksvc: no query executor configured")
	}
	return a.db.Query(ctx, connID, sql)
}

type actionAdapter struct{ db ActionExecutor }

func (a actionAdapter) ExecSQL(ctx context.Context, connID, sql string) (int64, error) {
	return a.db.Exec(ctx, connID, sql)
}
