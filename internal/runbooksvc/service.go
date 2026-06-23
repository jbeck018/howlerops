// Package runbooksvc orchestrates runbook persistence and execution: it loads a
// runbook definition from storage, runs it on the internal/runbook engine using
// host-provided SQL execution and notification, and records the run history. It
// is Wails-free so the whole flow is unit-testable; the app layer supplies a
// SQLExecutor over the database service and a Notifier over the event emitter.
package runbooksvc

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// SQLExecutor runs SQL for runbook steps: Query for read-only steps, Exec for
// write actions (returning rows affected).
type SQLExecutor interface {
	Query(ctx context.Context, connectionID, sql string) (*runbook.QueryResult, error)
	Exec(ctx context.Context, connectionID, sql string) (rowsAffected int64, err error)
}

// Notifier delivers notify-step messages (e.g. via the app event emitter).
type Notifier interface {
	Notify(ctx context.Context, channel, message string) error
}

// Store is the persistence surface, satisfied by *storage.RunbookStore.
type Store interface {
	SaveRunbook(*storage.Runbook) error
	GetRunbook(id string) (*storage.Runbook, error)
	ListRunbooks() ([]storage.RunbookSummary, error)
	DeleteRunbook(id string) error
	UpdateRunState(id, status string, runAt time.Time) error
	SaveRun(*storage.RunbookRun) error
	ListRuns(runbookID string, limit int) ([]storage.RunbookRun, error)
}

// Service coordinates runbook CRUD and execution.
type Service struct {
	store  Store
	db     SQLExecutor
	notify Notifier
}

// New constructs a Service. db is required to run runbooks; notify may be nil
// (notify steps then fail unless dry-run).
func New(store Store, db SQLExecutor, notify Notifier) *Service {
	return &Service{store: store, db: db, notify: notify}
}

// Save validates and persists a runbook definition, returning its ID.
func (s *Service) Save(rb runbook.Runbook) (string, error) {
	if err := runbook.Validate(rb); err != nil {
		return "", err
	}
	def, err := json.Marshal(rb)
	if err != nil {
		return "", fmt.Errorf("runbooksvc: marshal definition: %w", err)
	}
	rec := &storage.Runbook{ID: rb.ID, Name: rb.Name, Description: rb.Description, Definition: def}
	if err := s.store.SaveRunbook(rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

// Get loads a runbook definition by ID, returning (nil, nil) when not found.
func (s *Service) Get(id string) (*runbook.Runbook, error) {
	rec, err := s.store.GetRunbook(id)
	if err != nil {
		return nil, err
	}
	if rec == nil {
		return nil, nil
	}
	var rb runbook.Runbook
	if err := json.Unmarshal(rec.Definition, &rb); err != nil {
		return nil, fmt.Errorf("runbooksvc: unmarshal definition: %w", err)
	}
	// The storage row is the source of truth for identity/metadata.
	rb.ID = rec.ID
	rb.Name = rec.Name
	rb.Description = rec.Description
	return &rb, nil
}

// List returns runbook summaries.
func (s *Service) List() ([]storage.RunbookSummary, error) { return s.store.ListRunbooks() }

// Delete removes a runbook and its history.
func (s *Service) Delete(id string) error { return s.store.DeleteRunbook(id) }

// History returns recent runs for a runbook.
func (s *Service) History(id string, limit int) ([]storage.RunbookRun, error) {
	return s.store.ListRuns(id, limit)
}

// RunOptions tune a single execution.
type RunOptions struct {
	DryRun         bool
	AutoApprove    bool
	MaxParallel    int
	DefaultTimeout time.Duration
}

// Run executes a stored runbook with the given inputs, records the run in
// history, and (for non-dry runs) updates the runbook's last-run state.
func (s *Service) Run(ctx context.Context, id string, inputs map[string]any, opts RunOptions) (*runbook.RunResult, error) {
	rb, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if rb == nil {
		return nil, fmt.Errorf("runbooksvc: runbook %q not found", id)
	}

	started := time.Now().UTC()
	res, err := runbook.Execute(ctx, *rb, inputs, runbook.Deps{
		Query:  queryAdapter{s.db},
		Action: actionAdapter{s.db},
		Notify: s.notify,
	}, runbook.Options{
		DryRun:         opts.DryRun,
		AutoApprove:    opts.AutoApprove,
		MaxParallel:    opts.MaxParallel,
		DefaultTimeout: opts.DefaultTimeout,
	})
	if err != nil {
		// Pre-run failure (invalid inputs); nothing executed.
		return nil, err
	}

	finished := time.Now().UTC()
	status := runStatus(res)
	outcomes, _ := json.Marshal(res)
	_ = s.store.SaveRun(&storage.RunbookRun{
		RunbookID:  id,
		StartedAt:  started,
		FinishedAt: &finished,
		Status:     status,
		DryRun:     opts.DryRun,
		Outcomes:   outcomes,
	})
	if !opts.DryRun {
		_ = s.store.UpdateRunState(id, status, finished)
	}
	return res, nil
}

// runStatus summarizes a RunResult: failed if any step failed, partial if any
// step was skipped, otherwise success.
func runStatus(res *runbook.RunResult) string {
	if res.Failed {
		return "failed"
	}
	for _, oc := range res.Outcomes {
		if oc.Skipped != "" {
			return "partial"
		}
	}
	return "success"
}

type queryAdapter struct{ db SQLExecutor }

func (a queryAdapter) RunSQL(ctx context.Context, connID, sql string) (*runbook.QueryResult, error) {
	return a.db.Query(ctx, connID, sql)
}

type actionAdapter struct{ db SQLExecutor }

func (a actionAdapter) ExecSQL(ctx context.Context, connID, sql string) (int64, error) {
	return a.db.Exec(ctx, connID, sql)
}
