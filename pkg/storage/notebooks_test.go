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

func newNotebookStore(t *testing.T) *NotebookStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := NewNotebookStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return store
}

func TestNotebookStore_SaveGet(t *testing.T) {
	store := newNotebookStore(t)
	def := json.RawMessage(`{"cells":[{"id":"a","kind":"markdown"}]}`)
	nb := &Notebook{Name: "Explore", Description: "scratch", Definition: def}
	if err := store.SaveNotebook(nb); err != nil {
		t.Fatal(err)
	}
	if nb.ID == "" {
		t.Fatal("expected an ID")
	}
	got, err := store.GetNotebook(nb.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "Explore" || string(got.Definition) != string(def) {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestNotebookStore_GetMissing(t *testing.T) {
	store := newNotebookStore(t)
	got, err := store.GetNotebook("nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestNotebookStore_Validation(t *testing.T) {
	store := newNotebookStore(t)
	if err := store.SaveNotebook(&Notebook{Definition: json.RawMessage(`{}`)}); err == nil {
		t.Error("expected error for missing name")
	}
	if err := store.SaveNotebook(&Notebook{Name: "x"}); err == nil {
		t.Error("expected error for missing definition")
	}
	if err := store.SaveNotebook(nil); err == nil {
		t.Error("expected error for nil")
	}
}

func TestNotebookStore_UpsertAndList(t *testing.T) {
	store := newNotebookStore(t)
	nb := &Notebook{Name: "v1", Definition: json.RawMessage(`{}`)}
	if err := store.SaveNotebook(nb); err != nil {
		t.Fatal(err)
	}
	id := nb.ID
	nb.Name = "v2"
	if err := store.SaveNotebook(nb); err != nil {
		t.Fatal(err)
	}
	if nb.ID != id {
		t.Errorf("ID changed on upsert")
	}
	got, _ := store.GetNotebook(id)
	if got.Name != "v2" {
		t.Errorf("update not persisted: %s", got.Name)
	}
	list, _ := store.ListNotebooks()
	if len(list) != 1 {
		t.Errorf("expected 1 notebook, got %d", len(list))
	}
}

func TestNotebookStore_ListOrderedAndDelete(t *testing.T) {
	store := newNotebookStore(t)
	for _, n := range []string{"a", "b"} {
		if err := store.SaveNotebook(&Notebook{Name: n, Definition: json.RawMessage(`{}`)}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
	list, _ := store.ListNotebooks()
	if len(list) != 2 || list[0].Name != "b" {
		t.Errorf("expected newest-first list, got %+v", list)
	}
	if err := store.DeleteNotebook(list[0].ID); err != nil {
		t.Fatal(err)
	}
	if g, _ := store.GetNotebook(list[0].ID); g != nil {
		t.Error("expected gone after delete")
	}
}
