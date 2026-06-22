package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/internal/runbooksvc"
	"github.com/jbeck018/howlerops/pkg/database"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// WailsRunbookService exposes runbook CRUD and execution to the frontend. It
// wraps the Wails-free internal/runbooksvc orchestration, supplying SQL
// execution over the database service and notifications over the event emitter.
type WailsRunbookService struct {
	deps  *SharedDeps
	store *storage.RunbookStore
}

// NewWailsRunbookService constructs the service. The store is wired in later by
// the lifecycle once storage is initialized (see SetStore).
func NewWailsRunbookService(deps *SharedDeps) *WailsRunbookService {
	return &WailsRunbookService{deps: deps}
}

// SetStore injects the runbook store once the storage manager is ready.
func (s *WailsRunbookService) SetStore(store *storage.RunbookStore) { s.store = store }

func (s *WailsRunbookService) service() (*runbooksvc.Service, error) {
	if s.store == nil {
		return nil, fmt.Errorf("runbook storage is not initialized")
	}
	return runbooksvc.New(s.store, &runbookSQLExecutor{deps: s.deps}, &runbookNotifier{deps: s.deps}), nil
}

// SaveRunbook validates and persists a runbook definition, returning its ID.
func (s *WailsRunbookService) SaveRunbook(def runbooksvc.DefinitionDTO) (string, error) {
	svc, err := s.service()
	if err != nil {
		return "", err
	}
	return svc.Save(def.ToRunbook())
}

// ListRunbooks returns runbook summaries.
func (s *WailsRunbookService) ListRunbooks() ([]storage.RunbookSummary, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	return svc.List()
}

// GetRunbook loads a runbook definition by ID.
func (s *WailsRunbookService) GetRunbook(id string) (*runbooksvc.DefinitionDTO, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	rb, err := svc.Get(id)
	if err != nil {
		return nil, err
	}
	if rb == nil {
		return nil, fmt.Errorf("runbook %q not found", id)
	}
	def := runbooksvc.DefinitionFromRunbook(rb)
	return &def, nil
}

// DeleteRunbook removes a runbook and its history.
func (s *WailsRunbookService) DeleteRunbook(id string) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.Delete(id)
}

// RunRunbookRequest drives RunRunbook.
type RunRunbookRequest struct {
	RunbookID string         `json:"runbookId"`
	Inputs    map[string]any `json:"inputs"`
	// DryRun plans writes/notifications without performing them.
	DryRun bool `json:"dryRun"`
	// AutoApprove permits write actions without an interactive prompt. The
	// frontend is expected to confirm with the user before sending this.
	AutoApprove bool `json:"autoApprove"`
}

// RunRunbook executes a stored runbook and returns the per-step outcomes.
func (s *WailsRunbookService) RunRunbook(req RunRunbookRequest) (*runbooksvc.RunResultDTO, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	res, err := svc.Run(context.Background(), req.RunbookID, req.Inputs, runbooksvc.RunOptions{
		DryRun:         req.DryRun,
		AutoApprove:    req.AutoApprove,
		DefaultTimeout: 60 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	dto := runbooksvc.ResultToDTO(res)
	return &dto, nil
}

// RunbookHistory returns recent runs for a runbook.
func (s *WailsRunbookService) RunbookHistory(id string, limit int) ([]storage.RunbookRun, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	return svc.History(id, limit)
}

// runbookSQLExecutor adapts the database service to runbooksvc.SQLExecutor.
type runbookSQLExecutor struct{ deps *SharedDeps }

func (e *runbookSQLExecutor) Query(_ context.Context, connectionID, sql string) (*runbook.QueryResult, error) {
	res, err := e.deps.DatabaseService.ExecuteQuery(connectionID, sql, &database.QueryOptions{
		ReadOnly: true,
		Timeout:  60 * time.Second,
		Limit:    5000,
	})
	if err != nil {
		return nil, err
	}
	return toRunbookQueryResult(res), nil
}

func (e *runbookSQLExecutor) Exec(_ context.Context, connectionID, sql string) (int64, error) {
	res, err := e.deps.DatabaseService.ExecuteQuery(connectionID, sql, &database.QueryOptions{
		ReadOnly: false,
		Timeout:  60 * time.Second,
	})
	if err != nil {
		return 0, err
	}
	return res.Affected, nil
}

// toRunbookQueryResult converts the positional database result into the
// column-keyed shape the runbook engine consumes.
func toRunbookQueryResult(res *database.QueryResult) *runbook.QueryResult {
	if res == nil {
		return &runbook.QueryResult{}
	}
	out := &runbook.QueryResult{Columns: res.Columns, RowCount: res.RowCount}
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

// runbookNotifier adapts the event emitter to runbooksvc.Notifier.
type runbookNotifier struct{ deps *SharedDeps }

func (n *runbookNotifier) Notify(_ context.Context, channel, message string) error {
	n.deps.emitEvent("runbook:notification", map[string]interface{}{
		"channel": channel,
		"message": message,
	})
	return nil
}
