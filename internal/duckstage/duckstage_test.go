//go:build duckdb

package duckstage

import (
	"context"
	"testing"

	"github.com/jbeck018/howlerops/internal/notebook"
)

func newStager(t *testing.T) *Stager {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStager_StageQueryReset(t *testing.T) {
	s := newStager(t)
	ctx := context.Background()
	res := &notebook.QueryResult{
		Columns: []string{"region", "amount"},
		Rows: []map[string]any{
			{"region": "west", "amount": int64(10)},
			{"region": "east", "amount": int64(32)},
		},
		RowCount: 2,
	}
	if err := s.Stage(ctx, "totals", res); err != nil {
		t.Fatalf("stage: %v", err)
	}

	got, err := s.Query(ctx, `SELECT sum(amount) AS total FROM totals`)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got.RowCount != 1 || toInt(got.Rows[0]["total"]) != 42 {
		t.Errorf("sum(amount) = %+v, want 42", got.Rows)
	}

	// Re-staging replaces the table.
	if err := s.Stage(ctx, "totals", &notebook.QueryResult{
		Columns:  []string{"region", "amount"},
		Rows:     []map[string]any{{"region": "south", "amount": int64(5)}},
		RowCount: 1,
	}); err != nil {
		t.Fatalf("re-stage: %v", err)
	}
	got, _ = s.Query(ctx, `SELECT sum(amount) AS total FROM totals`)
	if toInt(got.Rows[0]["total"]) != 5 {
		t.Errorf("after replace, sum = %+v, want 5", got.Rows)
	}

	// Reset drops staged tables.
	if err := s.Reset(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := s.Query(ctx, `SELECT * FROM totals`); err == nil {
		t.Error("expected an error querying a dropped table")
	}
}

func TestStager_Composition_EndToEnd(t *testing.T) {
	// Prove the full reactive pipeline: a leaf cell's result is staged, and a
	// downstream cell that references it by handle computes over the staged data.
	s := newStager(t)
	nb := notebook.Notebook{Cells: []notebook.Cell{
		{ID: "a", Name: "totals", Kind: notebook.CellSQL, ConnectionID: "c", SQL: "SELECT region, amount FROM sales"},
		{ID: "b", Name: "top", Kind: notebook.CellSQL, SQL: "SELECT region, amount FROM totals WHERE amount > 20 ORDER BY amount DESC"},
	}}
	fr := &stubRunner{result: &notebook.QueryResult{
		Columns: []string{"region", "amount"},
		Rows: []map[string]any{
			{"region": "west", "amount": int64(10)},
			{"region": "east", "amount": int64(32)},
			{"region": "north", "amount": int64(25)},
		},
		RowCount: 3,
	}}

	res, err := notebook.Execute(context.Background(), nb, nil, notebook.Deps{Query: fr, Stage: s}, notebook.Options{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Cells)
	}

	var top notebook.CellResult
	for _, c := range res.Cells {
		if c.CellID == "b" {
			top = c
		}
	}
	if top.Result == nil {
		t.Fatal("composing cell produced no result")
	}
	if top.Result.RowCount != 2 {
		t.Errorf("expected 2 rows (east, north) over amount>20, got %d: %+v", top.Result.RowCount, top.Result.Rows)
	}
	// The real connection was hit only once (the leaf); the composing cell read
	// from the staged table.
	if fr.calls != 1 {
		t.Errorf("expected exactly 1 real-connection query, got %d", fr.calls)
	}
}

func TestStager_PartialReRunKeepsTablesWarm(t *testing.T) {
	// A partial re-run (Only) must not reset staging, so a downstream cell can
	// still see an upstream table staged by a prior full run.
	s := newStager(t)
	ctx := context.Background()
	if err := s.Stage(ctx, "totals", &notebook.QueryResult{
		Columns: []string{"amount"}, Rows: []map[string]any{{"amount": int64(7)}}, RowCount: 1,
	}); err != nil {
		t.Fatalf("seed stage: %v", err)
	}

	nb := notebook.Notebook{Cells: []notebook.Cell{
		{ID: "a", Name: "totals", Kind: notebook.CellSQL, ConnectionID: "c", SQL: "SELECT 1"},
		{ID: "b", Name: "top", Kind: notebook.CellSQL, SQL: "SELECT amount FROM totals"},
	}}
	// Re-run only b; a is preserved and 'totals' stays staged.
	res, err := notebook.Execute(ctx, nb, nil, notebook.Deps{Query: &stubRunner{}, Stage: s}, notebook.Options{Only: []string{"b"}})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var top notebook.CellResult
	for _, c := range res.Cells {
		if c.CellID == "b" {
			top = c
		}
	}
	if top.Result == nil || top.Result.RowCount != 1 {
		t.Errorf("composing cell should read the warm staged table: %+v", top)
	}
}

type stubRunner struct {
	result *notebook.QueryResult
	calls  int
}

func (r *stubRunner) RunSQL(context.Context, string, string) (*notebook.QueryResult, error) {
	r.calls++
	if r.result == nil {
		return &notebook.QueryResult{}, nil
	}
	return r.result, nil
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return -1
	}
}
