package runbooksvc

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// fakeDB records queries and writes, returning canned results.
type fakeDB struct {
	mu       sync.Mutex
	queried  []string
	executed []string
}

func (f *fakeDB) Query(_ context.Context, _, sql string) (*runbook.QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queried = append(f.queried, sql)
	return &runbook.QueryResult{Columns: []string{"n"}, Rows: []map[string]any{{"n": 1}}, RowCount: 1}, nil
}

func (f *fakeDB) Exec(_ context.Context, _, sql string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = append(f.executed, sql)
	return 2, nil
}

type fakeNotifier struct {
	mu       sync.Mutex
	messages []string
}

func (f *fakeNotifier) Notify(_ context.Context, _, message string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, message)
	return nil
}

func newStore(t *testing.T) *storage.RunbookStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("fk: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := storage.NewRunbookStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return store
}

func queryRunbook() runbook.Runbook {
	return runbook.Runbook{
		Name:   "Daily check",
		Inputs: []params.Definition{{Name: "status", Type: params.TypeString, Required: true}},
		Steps: []runbook.Step{
			{ID: "q", Kind: runbook.StepQuery, ConnectionID: "c", SQL: "SELECT * FROM t WHERE s = {{status}}"},
			{ID: "n", Kind: runbook.StepNotify, Message: "checked {{status}}", DependsOn: []string{"q"}},
		},
	}
}

func TestService_SaveGetListDelete(t *testing.T) {
	svc := New(newStore(t), &fakeDB{}, &fakeNotifier{})

	id, err := svc.Save(queryRunbook())
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected an ID")
	}

	got, err := svc.Get(id)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "Daily check" || len(got.Steps) != 2 || len(got.Inputs) != 1 {
		t.Errorf("round-trip lost data: %+v", got)
	}

	list, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != id {
		t.Errorf("list mismatch: %+v", list)
	}

	if err := svc.Delete(id); err != nil {
		t.Fatal(err)
	}
	if g, _ := svc.Get(id); g != nil {
		t.Error("expected runbook gone after delete")
	}
}

func TestService_SaveRejectsInvalid(t *testing.T) {
	svc := New(newStore(t), &fakeDB{}, nil)
	// Missing SQL on a query step -> validation error.
	bad := runbook.Runbook{Name: "x", Steps: []runbook.Step{{ID: "s", Kind: runbook.StepQuery, ConnectionID: "c"}}}
	if _, err := svc.Save(bad); err == nil {
		t.Error("expected validation error")
	}
}

func TestService_RunRecordsHistoryAndState(t *testing.T) {
	db := &fakeDB{}
	notify := &fakeNotifier{}
	svc := New(newStore(t), db, notify)

	id, err := svc.Save(queryRunbook())
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), id, map[string]any{"status": "active"}, RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Outcomes)
	}
	// Query bound and ran; notify delivered as plain text.
	if len(db.queried) != 1 || !strings.Contains(db.queried[0], "'active'") {
		t.Errorf("query not bound/run: %v", db.queried)
	}
	if len(notify.messages) != 1 || notify.messages[0] != "checked active" {
		t.Errorf("notify wrong: %v", notify.messages)
	}

	// History recorded and run state updated.
	hist, err := svc.History(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 1 || hist[0].Status != "success" || hist[0].DryRun {
		t.Errorf("history wrong: %+v", hist)
	}
	got, _ := svc.Get(id)
	stored, _ := svc.store.GetRunbook(id)
	if stored.LastRunStatus != "success" || stored.LastRunAt == nil {
		t.Errorf("run state not updated: %+v", stored)
	}
	_ = got
}

func TestService_DryRunDoesNotExecuteOrUpdateState(t *testing.T) {
	db := &fakeDB{}
	svc := New(newStore(t), db, &fakeNotifier{})

	rb := runbook.Runbook{
		Name:   "cleanup",
		Inputs: []params.Definition{{Name: "id", Type: params.TypeInteger, Required: true}},
		Steps:  []runbook.Step{{ID: "w", Kind: runbook.StepAction, ConnectionID: "c", SQL: "DELETE FROM t WHERE id = {{id}}"}},
	}
	id, err := svc.Save(rb)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), id, map[string]any{"id": 5}, RunOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.executed) != 0 {
		t.Errorf("dry run must not execute writes: %v", db.executed)
	}
	if !res.Outcomes["w"].Planned {
		t.Error("write should be planned in dry run")
	}
	// History records the dry run, but last-run state is not updated.
	hist, _ := svc.History(id, 10)
	if len(hist) != 1 || !hist[0].DryRun {
		t.Errorf("expected one dry-run history entry: %+v", hist)
	}
	stored, _ := svc.store.GetRunbook(id)
	if stored.LastRunStatus != "" {
		t.Errorf("dry run should not update last-run state, got %q", stored.LastRunStatus)
	}
}

func TestService_AutoApproveExecutesWrite(t *testing.T) {
	db := &fakeDB{}
	svc := New(newStore(t), db, &fakeNotifier{})
	rb := runbook.Runbook{
		Name:   "cleanup",
		Inputs: []params.Definition{{Name: "id", Type: params.TypeInteger, Required: true}},
		Steps:  []runbook.Step{{ID: "w", Kind: runbook.StepAction, ConnectionID: "c", SQL: "DELETE FROM t WHERE id = {{id}}"}},
	}
	id, _ := svc.Save(rb)

	res, err := svc.Run(context.Background(), id, map[string]any{"id": 9}, RunOptions{AutoApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(db.executed) != 1 || !strings.Contains(db.executed[0], "id = 9") {
		t.Errorf("auto-approved write should execute: %v", db.executed)
	}
	if res.Outcomes["w"].RowsAffected != 2 {
		t.Errorf("rows affected = %d, want 2", res.Outcomes["w"].RowsAffected)
	}
}

func TestService_RunMissingRunbook(t *testing.T) {
	svc := New(newStore(t), &fakeDB{}, nil)
	if _, err := svc.Run(context.Background(), "nope", nil, RunOptions{}); err == nil {
		t.Error("expected error for missing runbook")
	}
}
