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

// fakeStager is an in-memory Stager used to exercise cross-cell composition
// without a real DuckDB build.
type fakeStager struct {
	mu      sync.Mutex
	tables  map[string]*QueryResult
	queries []string
	resets  int
}

func newFakeStager() *fakeStager { return &fakeStager{tables: map[string]*QueryResult{}} }

func (s *fakeStager) Available() bool { return true }
func (s *fakeStager) Reset(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets++
	s.tables = map[string]*QueryResult{}
	return nil
}
func (s *fakeStager) Stage(_ context.Context, table string, res *QueryResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tables[table] = res
	return nil
}
func (s *fakeStager) Query(_ context.Context, sql string) (*QueryResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries = append(s.queries, sql)
	return &QueryResult{Columns: []string{"x"}, Rows: []map[string]any{{"x": 9}}, RowCount: 1}, nil
}

type fakeAction struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeAction) ExecSQL(_ context.Context, _, _ string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return 3, nil
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
	res, err := Execute(context.Background(), nb, map[string]any{"region": "west"}, Deps{Query: fr}, Options{})
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
	res, err := Execute(context.Background(), nb, nil, Deps{Query: fr}, Options{})
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
	// Two independent cells: a failure in one does not stop the other.
	nb := Notebook{Cells: []Cell{
		{ID: "bad", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT boom"},
		{ID: "ok", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT fine"},
	}}
	fr := newFakeRunner()
	fr.failOn["boom"] = errors.New("syntax error")

	res, err := Execute(context.Background(), nb, nil, Deps{Query: fr}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Error("expected Failed")
	}
	byID := resultsByID(res)
	if byID["bad"].Status != StatusError {
		t.Errorf("bad cell should error, got %s", byID["bad"].Status)
	}
	if byID["ok"].Status != StatusSuccess {
		t.Errorf("independent cell should still run by default, got %s", byID["ok"].Status)
	}
}

func TestExecute_SkipsDependentsOfFailure(t *testing.T) {
	// "later" consumes "bad"'s handle, so when "bad" fails its dependent is
	// skipped (the core DAG safety property).
	nb := Notebook{Cells: []Cell{
		{ID: "bad", Name: "bad", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT boom"},
		{ID: "later", Kind: CellSQL, SQL: "SELECT * FROM bad"},
	}}
	fr := newFakeRunner()
	fr.failOn["boom"] = errors.New("syntax error")
	st := newFakeStager()

	res, err := Execute(context.Background(), nb, nil, Deps{Query: fr, Stage: st}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	byID := resultsByID(res)
	if byID["bad"].Status != StatusError {
		t.Errorf("bad cell should error, got %s", byID["bad"].Status)
	}
	if byID["later"].Status != StatusSkipped {
		t.Errorf("dependent cell should be skipped, got %s", byID["later"].Status)
	}
	if len(st.queries) != 0 {
		t.Errorf("skipped dependent should not have queried the stager: %v", st.queries)
	}
}

func TestExecute_Composition(t *testing.T) {
	// marimo / DuckDB-UI model: cell "top" references cell "totals" by handle, so
	// it depends on it, runs on the staged compute engine, and "totals" is staged.
	nb := Notebook{Cells: []Cell{
		{ID: "a", Name: "totals", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT region, sum(amount) FROM sales GROUP BY region"},
		{ID: "b", Name: "top", Kind: CellSQL, SQL: "SELECT * FROM totals ORDER BY 2 DESC LIMIT 5"},
	}}
	fr := newFakeRunner()
	st := newFakeStager()
	res, err := Execute(context.Background(), nb, nil, Deps{Query: fr, Stage: st}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Cells)
	}
	if st.resets != 1 {
		t.Errorf("a full run should reset staging once, got %d", st.resets)
	}
	if _, ok := st.tables["totals"]; !ok {
		t.Error("cell a result was not staged under its handle 'totals'")
	}
	if len(st.queries) != 1 {
		t.Errorf("composing cell b should query the stager once, got %d (%v)", len(st.queries), st.queries)
	}
	if len(fr.calls) != 1 {
		t.Errorf("only the leaf cell a should hit the real connection, got %d", len(fr.calls))
	}
}

func TestExecute_CompositionUnavailable(t *testing.T) {
	// Without a stager, a composing cell fails with a clear message.
	nb := Notebook{Cells: []Cell{
		{ID: "a", Name: "totals", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"},
		{ID: "b", Name: "top", Kind: CellSQL, SQL: "SELECT * FROM totals"},
	}}
	res, err := Execute(context.Background(), nb, nil, Deps{Query: newFakeRunner()}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	byID := resultsByID(res)
	if byID["b"].Status != StatusError || !strings.Contains(byID["b"].Error, "compute engine") {
		t.Errorf("composing cell without a stager should error clearly, got %+v", byID["b"])
	}
}

func TestExecute_ActionGuardrail(t *testing.T) {
	nb := Notebook{
		Inputs: []params.Definition{{Name: "id", Type: params.TypeInteger, Required: true}},
		Cells:  []Cell{{ID: "w", Kind: CellAction, ConnectionID: "c", SQL: "DELETE FROM t WHERE id = {{id}}"}},
	}
	in := map[string]any{"id": 1}

	// Dry run: planned, not executed.
	fa := &fakeAction{}
	res, err := Execute(context.Background(), nb, in, Deps{Action: fa}, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Cells[0].Planned || fa.calls != 0 {
		t.Errorf("dry run should plan, not execute: planned=%v calls=%d", res.Cells[0].Planned, fa.calls)
	}

	// Not approved: error.
	res, err = Execute(context.Background(), nb, in, Deps{Action: fa}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].Status != StatusError {
		t.Errorf("unapproved action should error, got %s", res.Cells[0].Status)
	}

	// Auto-approved: executes.
	res, err = Execute(context.Background(), nb, in, Deps{Action: fa}, Options{AutoApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].Status != StatusSuccess || fa.calls != 1 {
		t.Errorf("auto-approved action should run once: status=%s calls=%d", res.Cells[0].Status, fa.calls)
	}
	if res.Cells[0].Rows != 3 {
		t.Errorf("expected rows affected captured, got %d", res.Cells[0].Rows)
	}
}

func TestExecute_PartialReRun(t *testing.T) {
	// Only re-running a seed cell preserves cells outside its descendant closure.
	nb := Notebook{Cells: []Cell{
		{ID: "a", Name: "totals", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"},
		{ID: "b", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 2"}, // independent
	}}
	fr := newFakeRunner()
	st := newFakeStager()
	res, err := Execute(context.Background(), nb, nil, Deps{Query: fr, Stage: st}, Options{Only: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	byID := resultsByID(res)
	if byID["a"].Status != StatusSuccess {
		t.Errorf("seed cell should run, got %s", byID["a"].Status)
	}
	if byID["b"].Status != StatusPreserved {
		t.Errorf("out-of-closure cell should be preserved, got %s", byID["b"].Status)
	}
	if st.resets != 0 {
		t.Errorf("partial re-run must not reset staging, got %d", st.resets)
	}
}

func TestExecute_MissingRequiredInput(t *testing.T) {
	nb := Notebook{
		Inputs: []params.Definition{{Name: "x", Type: params.TypeString, Required: true}},
		Cells:  []Cell{{ID: "m", Kind: CellMarkdown, Markdown: "{{x}}"}},
	}
	if _, err := Execute(context.Background(), nb, map[string]any{}, Deps{Query: newFakeRunner()}, Options{}); err == nil {
		t.Error("expected error for missing required input")
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]Notebook{
		"empty cell id":  {Cells: []Cell{{ID: "", Kind: CellMarkdown}}},
		"dup cell id":    {Cells: []Cell{{ID: "a", Kind: CellMarkdown}, {ID: "a", Kind: CellMarkdown}}},
		"sql no conn":    {Cells: []Cell{{ID: "a", Kind: CellSQL, SQL: "SELECT 1"}}},
		"sql no sql":     {Cells: []Cell{{ID: "a", Kind: CellSQL, ConnectionID: "c"}}},
		"action no conn": {Cells: []Cell{{ID: "a", Kind: CellAction, SQL: "DELETE FROM t"}}},
		"chart no src":   {Cells: []Cell{{ID: "a", Kind: CellChart, Chart: &ChartSpec{}}}},
		"dup handle":     {Cells: []Cell{{ID: "a", Name: "h", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"}, {ID: "b", Name: "h", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 2"}}},
		"bad handle":     {Cells: []Cell{{ID: "a", Name: "1bad", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"}}},
		"bad kind":       {Cells: []Cell{{ID: "a", Kind: "video"}}},
		"empty input":    {Inputs: []params.Definition{{Name: ""}}, Cells: []Cell{{ID: "a", Kind: CellMarkdown}}},
	}
	for name, nb := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(nb); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestValidate_ComposingCellNeedsNoConnection(t *testing.T) {
	nb := Notebook{Cells: []Cell{
		{ID: "a", Name: "totals", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"},
		{ID: "b", Kind: CellSQL, SQL: "SELECT * FROM totals"}, // composes -> no conn needed
	}}
	if err := Validate(nb); err != nil {
		t.Errorf("composing cell should validate without a connection: %v", err)
	}
}

func TestExecute_UndefinedPlaceholderErrorsCell(t *testing.T) {
	nb := Notebook{
		Inputs: []params.Definition{{Name: "a", Type: params.TypeString, Required: true}},
		Cells:  []Cell{{ID: "q", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT {{a}}, {{typo}}"}},
	}
	res, err := Execute(context.Background(), nb, map[string]any{"a": "x"}, Deps{Query: newFakeRunner()}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cells[0].Status != StatusError {
		t.Errorf("undefined placeholder should error the cell, got %s", res.Cells[0].Status)
	}
}

func TestDescendants(t *testing.T) {
	nb := Notebook{Cells: []Cell{
		{ID: "a", Name: "a", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 1"},
		{ID: "b", Name: "b", Kind: CellSQL, SQL: "SELECT * FROM a"},
		{ID: "c", Name: "c", Kind: CellSQL, SQL: "SELECT * FROM b"},
		{ID: "d", Kind: CellSQL, ConnectionID: "c", SQL: "SELECT 2"},
	}}
	got := Descendants(nb, []string{"a"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Descendants(a) = %v, want %v", got, want)
	}
}

func resultsByID(res *RunResult) map[string]CellResult {
	m := make(map[string]CellResult, len(res.Cells))
	for _, c := range res.Cells {
		m[c.CellID] = c
	}
	return m
}
