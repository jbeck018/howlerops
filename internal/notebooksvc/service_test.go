package notebooksvc

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/pkg/storage"
)

type fakeDB struct {
	mu      sync.Mutex
	queried []string
}

func (f *fakeDB) Query(_ context.Context, _, sql string) (*notebook.QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queried = append(f.queried, sql)
	return &notebook.QueryResult{Columns: []string{"n"}, Rows: []map[string]any{{"n": 1}}, RowCount: 1}, nil
}

func newStore(t *testing.T) *storage.NotebookStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := storage.NewNotebookStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return store
}

func sampleNotebook() notebook.Notebook {
	return notebook.Notebook{
		Name:   "Explore",
		Inputs: []params.Definition{{Name: "region", Type: params.TypeString, Required: true}},
		Cells: []notebook.Cell{
			{ID: "intro", Kind: notebook.CellMarkdown, Markdown: "# {{region}}"},
			{ID: "q", Kind: notebook.CellSQL, ConnectionID: "c", SQL: "SELECT * FROM t WHERE r = {{region}}"},
		},
	}
}

func TestService_SaveGetListDeleteRun(t *testing.T) {
	db := &fakeDB{}
	svc := New(Deps{Store: newStore(t), Query: db})

	id, err := svc.Save(sampleNotebook())
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
	if len(got.Cells) != 2 || len(got.Inputs) != 1 {
		t.Errorf("round-trip lost data: %+v", got)
	}

	list, _ := svc.List()
	if len(list) != 1 || list[0].ID != id {
		t.Errorf("list mismatch: %+v", list)
	}

	res, err := svc.Run(context.Background(), id, RunOptions{Inputs: map[string]any{"region": "west"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Cells)
	}
	if res.Cells[0].Markdown != "# west" {
		t.Errorf("markdown not rendered: %q", res.Cells[0].Markdown)
	}
	if len(db.queried) != 1 || !strings.Contains(db.queried[0], "'west'") {
		t.Errorf("SQL cell not bound/run: %v", db.queried)
	}

	if err := svc.Delete(id); err != nil {
		t.Fatal(err)
	}
	if g, _ := svc.Get(id); g != nil {
		t.Error("expected gone after delete")
	}
}

func TestService_SaveRejectsInvalid(t *testing.T) {
	svc := New(Deps{Store: newStore(t), Query: &fakeDB{}})
	bad := notebook.Notebook{Name: "x", Cells: []notebook.Cell{{ID: "a", Kind: notebook.CellSQL, ConnectionID: "c"}}}
	if _, err := svc.Save(bad); err == nil {
		t.Error("expected validation error for SQL cell with no SQL")
	}
}

func TestService_RunMissing(t *testing.T) {
	svc := New(Deps{Store: newStore(t), Query: &fakeDB{}})
	if _, err := svc.Run(context.Background(), "nope", RunOptions{}); err == nil {
		t.Error("expected error for missing notebook")
	}
}

func TestWire_DefinitionRoundTrip(t *testing.T) {
	d := DefinitionDTO{
		Name:   "Explore",
		Inputs: []InputDTO{{Name: "region", Type: "string", Required: true}},
		Cells: []CellDTO{
			{ID: "intro", Kind: "markdown", Markdown: "# {{region}}"},
			{ID: "q", Kind: "sql", ConnectionID: "c", SQL: "SELECT 1"},
		},
	}
	nb := d.ToNotebook()
	if len(nb.Cells) != 2 || nb.Cells[1].Kind != notebook.CellSQL {
		t.Fatalf("ToNotebook mismatch: %+v", nb)
	}
	back := DefinitionFromNotebook(&nb)
	if len(back.Cells) != 2 || back.Inputs[0].Type != "string" {
		t.Errorf("round-trip mismatch: %+v", back)
	}
}

func TestWire_ResultToDTO(t *testing.T) {
	res := &notebook.RunResult{
		Failed: true,
		Cells: []notebook.CellResult{
			{CellID: "m", Kind: notebook.CellMarkdown, Status: notebook.StatusSuccess, Markdown: "hi"},
			{CellID: "q", Kind: notebook.CellSQL, Status: notebook.StatusSuccess, SQL: "SELECT 1",
				Result: &notebook.QueryResult{Columns: []string{"n"}, Rows: []map[string]any{{"n": 1}}, RowCount: 1}},
			{CellID: "e", Kind: notebook.CellSQL, Status: notebook.StatusError, Error: "boom"},
		},
	}
	dto := ResultToDTO(res)
	if !dto.Failed || len(dto.Cells) != 3 {
		t.Fatalf("dto wrong: %+v", dto)
	}
	if dto.Cells[1].RowCount != 1 || dto.Cells[1].Columns[0] != "n" {
		t.Errorf("sql cell output lost: %+v", dto.Cells[1])
	}
	if dto.Cells[2].Error != "boom" {
		t.Errorf("error lost: %+v", dto.Cells[2])
	}
}
