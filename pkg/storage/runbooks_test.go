package storage

import (
	"database/sql"
	"encoding/json"
	"io"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

func newRunbookStore(t *testing.T) *RunbookStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable fk: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := NewRunbookStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return store
}

func TestRunbookStore_SaveGet(t *testing.T) {
	store := newRunbookStore(t)
	def := json.RawMessage(`{"inputs":[{"name":"status"}],"steps":[{"id":"s","kind":"query"}]}`)
	rb := &Runbook{Name: "Cleanup", Description: "removes stale rows", Definition: def}

	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	if rb.ID == "" {
		t.Fatal("expected an ID to be assigned")
	}
	if rb.CreatedAt.IsZero() || rb.UpdatedAt.IsZero() {
		t.Error("expected timestamps to be set")
	}

	got, err := store.GetRunbook(rb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected to load the runbook")
	}
	if got.Name != "Cleanup" || got.Description != "removes stale rows" {
		t.Errorf("loaded runbook mismatch: %+v", got)
	}
	if string(got.Definition) != string(def) {
		t.Errorf("definition mismatch: %s", got.Definition)
	}
}

func TestRunbookStore_GetMissing(t *testing.T) {
	store := newRunbookStore(t)
	got, err := store.GetRunbook("nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for missing runbook, got %+v", got)
	}
}

func TestRunbookStore_Upsert(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "v1", Definition: json.RawMessage(`{}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	id := rb.ID
	rb.Name = "v2"
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	if rb.ID != id {
		t.Errorf("ID changed on update: %s != %s", rb.ID, id)
	}
	got, _ := store.GetRunbook(id)
	if got.Name != "v2" {
		t.Errorf("update did not persist, name = %s", got.Name)
	}
	list, _ := store.ListRunbooks()
	if len(list) != 1 {
		t.Errorf("expected 1 runbook after upsert, got %d", len(list))
	}
}

func TestRunbookStore_Validation(t *testing.T) {
	store := newRunbookStore(t)
	if err := store.SaveRunbook(&Runbook{Definition: json.RawMessage(`{}`)}); err == nil {
		t.Error("expected error for missing name")
	}
	if err := store.SaveRunbook(&Runbook{Name: "x"}); err == nil {
		t.Error("expected error for missing definition")
	}
	if err := store.SaveRunbook(nil); err == nil {
		t.Error("expected error for nil runbook")
	}
}

func TestRunbookStore_ListOrdered(t *testing.T) {
	store := newRunbookStore(t)
	for _, n := range []string{"a", "b", "c"} {
		if err := store.SaveRunbook(&Runbook{Name: n, Definition: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // ensure distinct updated_at
	}
	list, err := store.ListRunbooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	// Most recently updated ("c") first.
	if list[0].Name != "c" {
		t.Errorf("expected newest-first ordering, got %s first", list[0].Name)
	}
}

func TestRunbookStore_Delete(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "doomed", Definition: json.RawMessage(`{}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRunbook(rb.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRunbook(rb.ID)
	if got != nil {
		t.Error("runbook should be gone after delete")
	}
}

func TestRunbookStore_UpdateRunState(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "rb", Definition: json.RawMessage(`{}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC()
	if err := store.UpdateRunState(rb.ID, "success", when); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRunbook(rb.ID)
	if got.LastRunStatus != "success" {
		t.Errorf("last run status = %q, want success", got.LastRunStatus)
	}
	if got.LastRunAt == nil {
		t.Error("expected last_run_at to be set")
	}
}

func TestRunbookStore_UpsertPreservesRunState(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "rb", Definition: json.RawMessage(`{"v":1}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateRunState(rb.ID, "success", when); err != nil {
		t.Fatal(err)
	}

	// A plain edit (new definition, no run state) must not clobber the
	// recorded last-run status/time.
	edit := &Runbook{ID: rb.ID, Name: "rb-renamed", Definition: json.RawMessage(`{"v":2}`)}
	if err := store.SaveRunbook(edit); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetRunbook(rb.ID)
	if got.Name != "rb-renamed" || string(got.Definition) != `{"v":2}` {
		t.Errorf("edit did not persist: %+v", got)
	}
	if got.LastRunStatus != "success" {
		t.Errorf("last_run_status wiped on edit: %q", got.LastRunStatus)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(when) {
		t.Errorf("last_run_at wiped/changed on edit: %v", got.LastRunAt)
	}
}

func TestRunbookStore_RunHistory(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "rb", Definition: json.RawMessage(`{}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}

	finished := time.Now().UTC()
	for i, status := range []string{"success", "failed"} {
		run := &RunbookRun{
			RunbookID:  rb.ID,
			Status:     status,
			DryRun:     i == 0,
			FinishedAt: &finished,
			Outcomes:   json.RawMessage(`{"ok":true}`),
		}
		if err := store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	runs, err := store.ListRuns(rb.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	// Newest first -> the "failed" run.
	if runs[0].Status != "failed" {
		t.Errorf("expected newest run first, got %s", runs[0].Status)
	}
	if string(runs[1].Outcomes) != `{"ok":true}` {
		t.Errorf("outcomes not round-tripped: %s", runs[1].Outcomes)
	}
}

func TestRunbookStore_RunCascadeDelete(t *testing.T) {
	store := newRunbookStore(t)
	rb := &Runbook{Name: "rb", Definition: json.RawMessage(`{}`)}
	if err := store.SaveRunbook(rb); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRun(&RunbookRun{RunbookID: rb.ID, Status: "success"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRunbook(rb.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := store.ListRuns(rb.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("runs should be cascade-deleted, got %d", len(runs))
	}
}
