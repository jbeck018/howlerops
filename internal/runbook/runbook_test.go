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
// an error for specific SQL substrings.
type fakeRunner struct {
	mu     sync.Mutex
	calls  []string
	failOn map[string]error
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{failOn: map[string]error{}}
}

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

// fakeAction records executed writes.
type fakeAction struct {
	mu       sync.Mutex
	executed []string
}

func (f *fakeAction) ExecSQL(_ context.Context, _, sql string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.executed = append(f.executed, sql)
	return 3, nil
}

// fakeNotifier records notifications.
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

func TestExecute_BindsInputsAndRuns(t *testing.T) {
	rb := Runbook{
		ID: "rb1",
		Inputs: []params.Definition{
			{Name: "status", Type: params.TypeString, Required: true},
			{Name: "limit", Type: params.TypeInteger, Default: 10},
		},
		Steps: []Step{
			{ID: "count", Kind: StepQuery, ConnectionID: "conn1", SQL: "SELECT count(*) FROM users WHERE status = {{status}} LIMIT {{limit}}"},
		},
	}
	fr := newFakeRunner()
	res, err := Execute(context.Background(), rb, map[string]any{"status": "active"}, Deps{Query: fr}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("unexpected failure: %+v", res.Outcomes)
	}
	want := "SELECT count(*) FROM users WHERE status = 'active' LIMIT 10"
	if got := res.Outcomes["count"].SQL; got != want {
		t.Errorf("bound SQL = %q, want %q", got, want)
	}
}

func TestExecute_OrderedStepsViaDeps(t *testing.T) {
	rb := Runbook{
		Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'a'"},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'b'", DependsOn: []string{"a"}},
			{ID: "c", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 'c'", DependsOn: []string{"b"}},
		},
	}
	fr := newFakeRunner()
	if _, err := Execute(context.Background(), rb, nil, Deps{Query: fr}, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 3 || !strings.Contains(fr.calls[0], "'a'") || !strings.Contains(fr.calls[2], "'c'") {
		t.Errorf("steps ran out of order: %v", fr.calls)
	}
}

func TestExecute_FailurePropagatesToDependents(t *testing.T) {
	rb := Runbook{
		Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT bad_thing"},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT good", DependsOn: []string{"a"}},
			{ID: "indep", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT independent"},
		},
	}
	fr := newFakeRunner()
	fr.failOn["bad_thing"] = errors.New("syntax error")

	res, err := Execute(context.Background(), rb, nil, Deps{Query: fr}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Failed {
		t.Error("expected RunResult.Failed")
	}
	if res.Outcomes["a"].Status != runner.StatusFailed {
		t.Errorf("a should be failed, got %s", res.Outcomes["a"].Status)
	}
	if res.Outcomes["b"].Status != runner.StatusSkipped {
		t.Errorf("b should be skipped, got %s", res.Outcomes["b"].Status)
	}
	if res.Outcomes["indep"].Status != runner.StatusSuccess {
		t.Errorf("indep should succeed, got %s", res.Outcomes["indep"].Status)
	}
}

func TestExecute_MissingRequiredInput(t *testing.T) {
	rb := Runbook{
		Inputs: []params.Definition{{Name: "owner", Type: params.TypeString, Required: true}},
		Steps:  []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT {{owner}}"}},
	}
	if _, err := Execute(context.Background(), rb, map[string]any{}, Deps{Query: newFakeRunner()}, Options{}); err == nil {
		t.Error("expected error for missing required input")
	}
}

func TestExecute_NoQueryRunnerFailsStep(t *testing.T) {
	rb := Runbook{Steps: []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT 1"}}}
	res, err := Execute(context.Background(), rb, nil, Deps{}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["s"].Status != runner.StatusFailed {
		t.Errorf("expected step to fail with no query runner, got %s", res.Outcomes["s"].Status)
	}
}

// --- write actions + guardrail ------------------------------------------------

func actionRunbook() Runbook {
	return Runbook{
		Inputs: []params.Definition{{Name: "id", Type: params.TypeInteger, Required: true}},
		Steps: []Step{
			{ID: "write", Kind: StepAction, ConnectionID: "c", SQL: "DELETE FROM stale WHERE id = {{id}}"},
		},
	}
}

func TestExecute_DryRunPlansWritesWithoutExecuting(t *testing.T) {
	fa := &fakeAction{}
	fn := &fakeNotifier{}
	rb := actionRunbook()
	rb.Steps = append(rb.Steps, Step{ID: "tell", Kind: StepNotify, Message: "deleted {{id}}", DependsOn: []string{"write"}})

	res, err := Execute(context.Background(), rb, map[string]any{"id": 7}, Deps{Action: fa, Notify: fn}, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun {
		t.Error("result should be marked DryRun")
	}
	if len(fa.executed) != 0 {
		t.Errorf("dry run must not execute writes, got %v", fa.executed)
	}
	if len(fn.messages) != 0 {
		t.Errorf("dry run must not send notifications, got %v", fn.messages)
	}
	w := res.Outcomes["write"]
	if !w.Planned || w.SQL != "DELETE FROM stale WHERE id = 7" {
		t.Errorf("write not planned correctly: %+v", w)
	}
	tell := res.Outcomes["tell"]
	if !tell.Planned || tell.Message != "deleted 7" {
		t.Errorf("notify not planned correctly: %+v", tell)
	}
}

func TestExecute_ActionRequiresApproval(t *testing.T) {
	fa := &fakeAction{}
	// No approver, not auto-approved -> blocked (failed).
	res, err := Execute(context.Background(), actionRunbook(), map[string]any{"id": 1}, Deps{Action: fa}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["write"].Status != runner.StatusFailed {
		t.Errorf("unapproved action should fail, got %s", res.Outcomes["write"].Status)
	}
	if len(fa.executed) != 0 {
		t.Error("unapproved action must not execute")
	}
}

func TestExecute_ApproverAllowsAndDenies(t *testing.T) {
	fa := &fakeAction{}
	var seen ActionRequest
	approve := func(_ context.Context, req ActionRequest) (bool, error) {
		seen = req
		return true, nil
	}
	res, err := Execute(context.Background(), actionRunbook(), map[string]any{"id": 9}, Deps{Action: fa, Approve: approve}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["write"].Status != runner.StatusSuccess {
		t.Fatalf("approved action should succeed: %+v", res.Outcomes["write"])
	}
	if res.Outcomes["write"].RowsAffected != 3 {
		t.Errorf("rows affected = %d, want 3", res.Outcomes["write"].RowsAffected)
	}
	if seen.SQL != "DELETE FROM stale WHERE id = 9" {
		t.Errorf("approver saw wrong SQL: %q", seen.SQL)
	}

	// Denied.
	fa2 := &fakeAction{}
	deny := func(_ context.Context, _ ActionRequest) (bool, error) { return false, nil }
	res2, _ := Execute(context.Background(), actionRunbook(), map[string]any{"id": 1}, Deps{Action: fa2, Approve: deny}, Options{})
	if res2.Outcomes["write"].Status != runner.StatusFailed {
		t.Errorf("denied action should fail, got %s", res2.Outcomes["write"].Status)
	}
	if len(fa2.executed) != 0 {
		t.Error("denied action must not execute")
	}
}

func TestExecute_AutoApproveBypassesApproval(t *testing.T) {
	fa := &fakeAction{}
	res, err := Execute(context.Background(), actionRunbook(), map[string]any{"id": 2}, Deps{Action: fa}, Options{AutoApprove: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["write"].Status != runner.StatusSuccess {
		t.Errorf("auto-approved action should run, got %s", res.Outcomes["write"].Status)
	}
	if len(fa.executed) != 1 {
		t.Errorf("expected the write to execute once, got %v", fa.executed)
	}
}

func TestExecute_NotifyRendersPlainText(t *testing.T) {
	fn := &fakeNotifier{}
	rb := Runbook{
		Inputs: []params.Definition{{Name: "name", Type: params.TypeString, Required: true}},
		Steps:  []Step{{ID: "n", Kind: StepNotify, Channel: "ops", Message: "hello {{name}}"}},
	}
	res, err := Execute(context.Background(), rb, map[string]any{"name": "alice"}, Deps{Notify: fn}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["n"].Status != runner.StatusSuccess || !res.Outcomes["n"].Notified {
		t.Fatalf("notify should succeed: %+v", res.Outcomes["n"])
	}
	// Plain text, NOT SQL-quoted.
	if len(fn.messages) != 1 || fn.messages[0] != "hello alice" {
		t.Errorf("notify message = %v, want [hello alice]", fn.messages)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]Runbook{
		"missing conn":  {Steps: []Step{{ID: "s", Kind: StepQuery, SQL: "SELECT 1"}}},
		"missing sql":   {Steps: []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c"}}},
		"action no sql": {Steps: []Step{{ID: "s", Kind: StepAction, ConnectionID: "c"}}},
		"notify no msg": {Steps: []Step{{ID: "s", Kind: StepNotify}}},
		"bad kind":      {Steps: []Step{{ID: "s", Kind: "destroy", ConnectionID: "c", SQL: "x"}}},
		"empty input":   {Inputs: []params.Definition{{Name: ""}}, Steps: []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "x"}}},
		"cycle": {Steps: []Step{
			{ID: "a", Kind: StepQuery, ConnectionID: "c", SQL: "x", DependsOn: []string{"b"}},
			{ID: "b", Kind: StepQuery, ConnectionID: "c", SQL: "x", DependsOn: []string{"a"}},
		}},
	}
	for name, rb := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(rb); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestExecute_UndefinedPlaceholderFails(t *testing.T) {
	rb := Runbook{
		Inputs: []params.Definition{{Name: "a", Type: params.TypeString, Required: true}},
		Steps:  []Step{{ID: "s", Kind: StepQuery, ConnectionID: "c", SQL: "SELECT {{a}}, {{typo}}"}},
	}
	res, err := Execute(context.Background(), rb, map[string]any{"a": "x"}, Deps{Query: newFakeRunner()}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcomes["s"].Status != runner.StatusFailed {
		t.Errorf("expected failure on undefined placeholder, got %s", res.Outcomes["s"].Status)
	}
}
