package notebook

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jbeck018/howlerops/internal/params"
)

type fakeRunner struct {
	mu     sync.Mutex
	calls  []string
	failOn map[string]error
}

func newFakeRunner() *fakeRunner { return &fakeRunner{failOn: map[string]error{}} }

func (f *fakeRunner) RunSQL(_ context.Context, _, sql string) (*QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sql)
	for sub, err := range f.failOn {
		if strings.Contains(sql, sub) {
			return nil, err
		}
	}
	return &QueryResult{Columns: []string{"n"}, Rows: []map[string]any{{"n": 1}}, RowCount: 1}, nil
}

func TestExecute_MarkdownAndSQL(t *testing.T) {
	nb := Notebook{
		Name:   "Sales analysis",
		Inputs: []params.Definition{{Name: "region", Type: params.TypeString, Required: true}},
		Cells: []Cell{
			{ID: "intro", Kind: CellMarkdown, Markdown: "# Sales for {{region}}"},
			{ID: "q", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT * FROM sales WHERE region = {{region}}"},
		},
	}
	fr := newFakeRunner()
	res, err := Execute(context.Background(), nb, map[string]any{"region": "west"}, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Cells)
	}
	if len(res.Cells) != 2 {
		t.Fatalf("expected 2 cell results, got %d", len(res.Cells))
	}
	if res.Cells[0].Markdown != "# Sales for west" {
		t.Errorf("markdown not rendered: %q", res.Cells[0].Markdown)
	}
	if res.Cells[1].SQL != "SELECT * FROM sales WHERE region = 'west'" {
		t.Errorf("SQL not bound: %q", res.Cells[1].SQL)
	}
	if res.Cells[1].Result == nil || res.Cells[1].Result.RowCount != 1 {
		t.Errorf("expected query result: %+v", res.Cells[1].Result)
	}
}

func TestExecute_OrderPreserved(t *testing.T) {
	nb := Notebook{Cells: []Cell{
		{ID: "a", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 'a'"},
		{ID: "b", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 'b'"},
		{ID: "c", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 'c'"},
	}}
	fr := newFakeRunner()
	res, err := Execute(context.Background(), nb, nil, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].CellID != "a" || res.Cells[2].CellID != "c" {
		t.Errorf("cell order not preserved: %+v", res.Cells)
	}
	if len(fr.calls) != 3 {
		t.Errorf("expected 3 queries, got %d", len(fr.calls))
	}
}

func TestExecute_ContinuesPastErrorByDefault(t *testing.T) {
	nb := Notebook{Cells: []Cell{
		{ID: "bad", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT boom"},
		{ID: "ok", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT fine"},
	}}
	fr := newFakeRunner()
	fr.failOn["boom"] = errors.New("syntax error")

	res, err := Execute(context.Background(), nb, nil, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Error("expected Failed")
	}
	if res.Cells[0].Status != StatusError {
		t.Errorf("bad cell should error, got %s", res.Cells[0].Status)
	}
	if res.Cells[1].Status != StatusSuccess {
		t.Errorf("later cell should still run by default, got %s", res.Cells[1].Status)
	}
}

func TestExecute_StopOnErrorSkipsRest(t *testing.T) {
	nb := Notebook{Cells: []Cell{
		{ID: "bad", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT boom"},
		{ID: "later", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT fine"},
	}}
	fr := newFakeRunner()
	fr.failOn["boom"] = errors.New("syntax error")

	res, err := Execute(context.Background(), nb, nil, fr, Options{StopOnError: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].Status != StatusError {
		t.Errorf("bad cell should error, got %s", res.Cells[0].Status)
	}
	if res.Cells[1].Status != StatusSkipped {
		t.Errorf("later cell should be skipped, got %s", res.Cells[1].Status)
	}
	if len(fr.calls) != 1 {
		t.Errorf("only the first query should run, got %d", len(fr.calls))
	}
}

func TestExecute_MissingRequiredInput(t *testing.T) {
	nb := Notebook{
		Inputs: []params.Definition{{Name: "x", Type: params.TypeString, Required: true}},
		Cells:  []Cell{{ID: "m", Kind: CellMarkdown, Markdown: "{{x}}"}},
	}
	if _, err := Execute(context.Background(), nb, map[string]any{}, newFakeRunner(), Options{}); err == nil {
		t.Error("expected error for missing required input")
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]Notebook{
		"empty cell id": {Cells: []Cell{{ID: "", Kind: CellMarkdown}}},
		"dup cell id":   {Cells: []Cell{{ID: "a", Kind: CellMarkdown}, {ID: "a", Kind: CellMarkdown}}},
		"sql no conn":   {Cells: []Cell{{ID: "a", Kind: CellSQL, SQL: "SELECT 1"}}},
		"sql no sql":    {Cells: []Cell{{ID: "a", Kind: CellSQL, ConnectionID: "c"}}},
		"bad kind":      {Cells: []Cell{{ID: "a", Kind: "video"}}},
		"empty input":   {Inputs: []params.Definition{{Name: ""}}, Cells: []Cell{{ID: "a", Kind: CellMarkdown}}},
	}
	for name, nb := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(nb); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestExecute_UndefinedPlaceholderErrorsCell(t *testing.T) {
	nb := Notebook{
		Inputs: []params.Definition{{Name: "a", Type: params.TypeString, Required: true}},
		Cells:  []Cell{{ID: "q", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT {{a}}, {{typo}}"}},
	}
	res, err := Execute(context.Background(), nb, map[string]any{"a": "x"}, newFakeRunner(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].Status != StatusError {
		t.Errorf("undefined placeholder should error the cell, got %s", res.Cells[0].Status)
	}
}
