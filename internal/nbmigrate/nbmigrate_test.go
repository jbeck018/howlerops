package nbmigrate

import (
	"database/sql"
	"encoding/json"
	"io"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

func stores(t *testing.T) (*storage.RunbookStore, *storage.NotebookStore) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	rb := storage.NewRunbookStore(db, logger)
	nb := storage.NewNotebookStore(db, logger)
	if err := rb.EnsureSchema(); err != nil {
		t.Fatalf("runbook schema: %v", err)
	}
	if err := nb.EnsureSchema(); err != nil {
		t.Fatalf("notebook schema: %v", err)
	}
	return rb, nb
}

func saveRunbook(t *testing.T, store *storage.RunbookStore, rb runbook.Runbook) string {
	t.Helper()
	def, err := json.Marshal(rb)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	rec := &storage.Runbook{ID: rb.ID, Name: rb.Name, Description: rb.Description, Definition: def}
	if err := store.SaveRunbook(rec); err != nil {
		t.Fatalf("save runbook: %v", err)
	}
	return rec.ID
}

func sampleRunbook() runbook.Runbook {
	return runbook.Runbook{
		Name:        "Nightly cleanup",
		Description: "remove stale rows and notify",
		Inputs:      []params.Definition{{Name: "days", Type: params.TypeInteger, Required: true}},
		Steps: []runbook.Step{
			{ID: "find", Name: "Find stale", Kind: runbook.StepQuery, ConnectionID: "c", SQL: "SELECT id FROM t WHERE age > {{days}}"},
			{ID: "del", Name: "Delete stale", Kind: runbook.StepAction, DependsOn: []string{"find"}, ConnectionID: "c", SQL: "DELETE FROM t WHERE age > {{days}}"},
			{ID: "tell", Name: "Notify", Kind: runbook.StepNotify, DependsOn: []string{"del"}, Channel: "#ops", Message: "cleaned rows older than {{days}} days"},
		},
	}
}

func TestRunbookToNotebook_MapsKindsAndDeps(t *testing.T) {
	nb := RunbookToNotebook(sampleRunbook())
	if len(nb.Cells) != 3 || len(nb.Inputs) != 1 {
		t.Fatalf("unexpected conversion: %+v", nb)
	}
	if nb.Cells[0].Kind != notebook.CellSQL {
		t.Errorf("query step should become sql cell, got %s", nb.Cells[0].Kind)
	}
	if nb.Cells[1].Kind != notebook.CellAction {
		t.Errorf("action step should become action cell, got %s", nb.Cells[1].Kind)
	}
	if nb.Cells[2].Kind != notebook.CellNotify {
		t.Errorf("notify step should become notify cell, got %s", nb.Cells[2].Kind)
	}
	if len(nb.Cells[1].DependsOn) != 1 || nb.Cells[1].DependsOn[0] != "find" {
		t.Errorf("dependencies not preserved: %+v", nb.Cells[1].DependsOn)
	}
	if nb.Cells[0].Title != "Find stale" {
		t.Errorf("step name should become cell title, got %q", nb.Cells[0].Title)
	}
	if err := notebook.Validate(nb); err != nil {
		t.Errorf("converted notebook should validate: %v", err)
	}
}

func TestRunbooksToNotebooks_MigratesAndIsIdempotent(t *testing.T) {
	rbStore, nbStore := stores(t)
	id := saveRunbook(t, rbStore, sampleRunbook())

	n, err := RunbooksToNotebooks(rbStore, nbStore, nil)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 migrated, got %d", n)
	}

	got, err := nbStore.GetNotebook(id)
	if err != nil || got == nil {
		t.Fatalf("migrated notebook missing: %v / %v", err, got)
	}
	var nb notebook.Notebook
	if err := json.Unmarshal(got.Definition, &nb); err != nil {
		t.Fatalf("unmarshal migrated: %v", err)
	}
	if len(nb.Cells) != 3 {
		t.Errorf("migrated notebook lost cells: %+v", nb.Cells)
	}

	// Running again migrates nothing (idempotent).
	n, err = RunbooksToNotebooks(rbStore, nbStore, nil)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if n != 0 {
		t.Errorf("second run should migrate 0, got %d", n)
	}
}

func TestRunbooksToNotebooks_DoesNotClobberEdits(t *testing.T) {
	rbStore, nbStore := stores(t)
	id := saveRunbook(t, rbStore, sampleRunbook())

	if _, err := RunbooksToNotebooks(rbStore, nbStore, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Simulate the user editing the migrated notebook down to a single cell.
	edited := notebook.Notebook{ID: id, Name: "Edited", Cells: []notebook.Cell{
		{ID: "x", Kind: notebook.CellMarkdown, Markdown: "hello"},
	}}
	def, _ := json.Marshal(edited)
	if err := nbStore.SaveNotebook(&storage.Notebook{ID: id, Name: edited.Name, Definition: def}); err != nil {
		t.Fatalf("save edit: %v", err)
	}

	if _, err := RunbooksToNotebooks(rbStore, nbStore, nil); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
	got, _ := nbStore.GetNotebook(id)
	var nb notebook.Notebook
	_ = json.Unmarshal(got.Definition, &nb)
	if got.Name != "Edited" || len(nb.Cells) != 1 {
		t.Errorf("migration clobbered user edits: name=%q cells=%d", got.Name, len(nb.Cells))
	}
}
