package runbook

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runner"
)

// fakeRunner records the SQL it was asked to run and returns canned results or
// an error for specific connection IDs.
type fakeRunner struct {
	mu       sync.Mutex
	executed map[string]string // stepConn -> sql (last)
	calls    []string
	failOn   map[string]error // sql substring -> error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{executed: map[string]string{}, failOn: map[string]error{}}
}

func (f *fakeRunner) RunSQL(_ context.Context, connectionID, sql string) (*QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sql)
	for sub, err := range f.failOn {
		if strings.Contains(sql, sub) {
			return nil, err
		}
	}
	f.executed[connectionID] = sql
	return &QueryResult{Columns: []string{"n"}, Rows: []map[string]any{{"n": 1}}, RowCount: 1}, nil
}

func TestExecute_BindsInputsAndRuns(t *testing.T) {
	rb := Runbook{
		ID:   "rb1",
		Name: "Active users",
		Inputs: []params.Definition{
			{Name: "status", Type: params.TypeString, Required: true},
			{Name: "limit", Type: params.TypeInteger, Default: 10},
		},
		Steps: []Step{
			{
				ID:           "count",
				Kind:         StepQuery,
				ConnectionID: "conn1",
				SQL:          "SELECT count(*) FROM users WHERE status = {{status}} LIMIT {{limit}}",
			},
		},
	}
	fr := newFakeRunner()
	res, err := Execute(context.Background(), rb, map[string]any{"status": "active"}, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Outcomes)
	}
	got := res.Outcomes["count"]
	want := "SELECT count(*) FROM users WHERE status = 'active' LIMIT 10"
	if got.SQL != want {
		t.Errorf("bound SQL = %q, want %q", got.SQL, want)
	}
	if got.Status != runner.StatusSuccess {
		t.Errorf("status = %s, want success", got.Status)
	}
	if got.Result == nil || got.Result.RowCount != 1 {
		t.Errorf("expected a result with RowCount 1, got %+v", got.Result)
	}
}

func TestExecute_OrderedStepsViaDeps(t *testing.T) {
	rb := Runbook{
		ID: "rb",
		Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'a'"},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'b'", DependsOn: []string{"a"}},
			{ID: "c", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'c'", DependsOn: []string{"b"}},
		},
	}
	fr := newFakeRunner()
	res, err := Execute(context.Background(), rb, nil, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(fr.calls))
	}
	// Dependencies force a,b,c order.
	if !(strings.Contains(fr.calls[0], "'a'") && strings.Contains(fr.calls[1], "'b'") && strings.Contains(fr.calls[2], "'c'")) {
		t.Errorf("steps ran out of order: %v", fr.calls)
	}
	for _, id := range []string{"a", "b", "c"} {
		if res.Outcomes[id].Status != runner.StatusSuccess {
			t.Errorf("step %s not success: %s", id, res.Outcomes[id].Status)
		}
	}
}

func TestExecute_FailurePropagatesToDependents(t *testing.T) {
	rb := Runbook{
		ID: "rb",
		Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT bad_thing"},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT good", DependsOn: []string{"a"}},
			{ID: "indep", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT independent"},
		},
	}
	fr := newFakeRunner()
	fr.failOn["bad_thing"] = errors.New("syntax error")

	res, err := Execute(context.Background(), rb, nil, fr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Error("expected RunResult.Failed to be true")
	}
	if res.Outcomes["a"].Status != runner.StatusFailed {
		t.Errorf("a should be failed, got %s", res.Outcomes["a"].Status)
	}
	if res.Outcomes["b"].Status != runner.StatusSkipped {
		t.Errorf("b should be skipped after a failed, got %s", res.Outcomes["b"].Status)
	}
	if res.Outcomes["indep"].Status != runner.StatusSuccess {
		t.Errorf("independent step should still succeed, got %s", res.Outcomes["indep"].Status)
	}
}

func TestExecute_MissingRequiredInput(t *testing.T) {
	rb := Runbook{
		ID:     "rb",
		Inputs: []params.Definition{{Name: "owner", Type: params.TypeString, Required: true}},
		Steps: []Step{
			{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT {{owner}}"},
		},
	}
	if _, err := Execute(context.Background(), rb, map[string]any{}, newFakeRunner(), Options{}); err == nil {
		t.Error("expected error for missing required input")
	}
}

func TestExecute_EnumInputValidation(t *testing.T) {
	rb := Runbook{
		ID:     "rb",
		Inputs: []params.Definition{{Name: "period", Type: params.TypeEnum, Options: []string{"day", "week"}, Required: true}},
		Steps:  []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT {{period}}"}},
	}
	if _, err := Execute(context.Background(), rb, map[string]any{"period": "year"}, newFakeRunner(), Options{}); err == nil {
		t.Error("expected enum validation error")
	}
	if _, err := Execute(context.Background(), rb, map[string]any{"period": "week"}, newFakeRunner(), Options{}); err != nil {
		t.Errorf("valid enum should pass: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]Runbook{
		"missing conn": {Steps: []Step{{ID: "s", Kind: StepQuery, SQL: "SELECT 1"}}},
		"missing sql":  {Steps: []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c"}}},
		"bad kind":     {Steps: []Step{{ID: "s", Kind: "destroy", ConnectionID: "c", SQL: "x"}}},
		"cycle": {Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "x", DependsOn: []string{"b"}},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "x", DependsOn: []string{"a"}},
		}},
		"empty input name": {
			Inputs: []params.Definition{{Name: ""}},
			Steps:  []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "x"}},
		},
	}
	for name, rb := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(rb); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestExecute_NoRunner(t *testing.T) {
	rb := Runbook{Steps: []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 1"}}}
	if _, err := Execute(context.Background(), rb, nil, nil, Options{}); err == nil {
		t.Error("expected error when query runner is nil")
	}
}

func TestExecute_UndefinedPlaceholderFails(t *testing.T) {
	rb := Runbook{
		ID:     "rb",
		Inputs: []params.Definition{{Name: "a", Type: params.TypeString, Required: true}},
		Steps:  []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT {{a}}, {{typo}}"}},
	}
	res, err := Execute(context.Background(), rb, map[string]any{"a": "x"}, newFakeRunner(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The step fails because {{typo}} is not a defined input.
	if res.Outcomes["s"].Status != runner.StatusFailed {
		t.Errorf("expected step to fail on undefined placeholder, got %s", res.Outcomes["s"].Status)
	}
}
