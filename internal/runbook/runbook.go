// Package runbook models a reusable, parameterized operational task: typed
// inputs, an ordered (DAG) set of steps, and captured outputs. It is the Phase 4
// foundation and is built entirely on the platform primitives — typed inputs via
// internal/params and execution via internal/runner — so it stays Wails-free and
// unit-testable. The host injects a QueryRunner to actually execute SQL.
package runbook

import (
	"context"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runner"
)

// StepKind enumerates what a step does. Phase 4 starts with read-only queries;
// "action" (writes) and "notify" steps come later behind a dry-run/approval
// guardrail.
type StepKind string

const (
	StepQuery StepKind = "query"
)

// Step is one unit of a runbook.
type Step struct {
	ID           string
	Name         string
	Kind         StepKind
	DependsOn    []string
	ConnectionID string
	// SQL is a template referencing the runbook's inputs as {{name}}
	// placeholders, bound safely via internal/params.
	SQL string
	// Timeout bounds this step; 0 uses the run-wide default.
	Timeout time.Duration
}

// Runbook is a parameterized task definition.
type Runbook struct {
	ID          string
	Name        string
	Description string
	Inputs      []params.Definition
	Steps       []Step
}

// QueryResult is the tabular output of a step query.
type QueryResult struct {
	Columns  []string
	Rows     []map[string]any
	RowCount int64
}

// QueryRunner executes a step's bound SQL against a connection. The host adapts
// its database layer to this interface.
type QueryRunner interface {
	RunSQL(ctx context.Context, connectionID, sql string) (*QueryResult, error)
}

// StepOutcome captures the result of one executed (or skipped) step.
type StepOutcome struct {
	StepID  string
	Name    string
	Status  runner.Status
	SQL     string // the bound SQL that was executed
	Result  *QueryResult
	Error   string
	Skipped string // reason, when Status == skipped
}

// RunResult is the outcome of executing a runbook.
type RunResult struct {
	Outcomes map[string]StepOutcome
	Order    []string // step IDs in definition order
	Failed   bool
}

// Options tune execution.
type Options struct {
	MaxParallel    int
	DefaultTimeout time.Duration
	// StopOnError aborts not-yet-started steps after the first failure. When
	// false (default), only steps that depend on a failed step are skipped;
	// independent branches still run.
	StopOnError bool
}

// Validate checks the runbook is structurally sound: step IDs/dependencies form
// a valid DAG and each query step names a connection and SQL.
func Validate(rb Runbook) error {
	for _, def := range rb.Inputs {
		if def.Name == "" {
			return fmt.Errorf("runbook: input with empty name")
		}
	}
	steps := make([]runner.Step, len(rb.Steps))
	for i, st := range rb.Steps {
		steps[i] = runner.Step{ID: st.ID, DependsOn: st.DependsOn}
		switch st.Kind {
		case StepQuery, "":
			if st.ConnectionID == "" {
				return fmt.Errorf("runbook: step %q missing connection", st.ID)
			}
			if st.SQL == "" {
				return fmt.Errorf("runbook: step %q missing SQL", st.ID)
			}
		default:
			return fmt.Errorf("runbook: step %q has unsupported kind %q", st.ID, st.Kind)
		}
	}
	return runner.Validate(runner.Plan{Steps: steps})
}

// Execute validates inputs, binds each step's SQL, and runs the steps on the
// shared runner engine. A step failure fails its dependents (which are skipped);
// independent steps continue unless Options.StopOnError is set.
func Execute(ctx context.Context, rb Runbook, inputs map[string]any, qr QueryRunner, opts Options) (*RunResult, error) {
	if err := Validate(rb); err != nil {
		return nil, err
	}
	if qr == nil {
		return nil, fmt.Errorf("runbook: no query runner provided")
	}
	// Validate inputs once up front for a clear error before any step runs.
	if _, err := params.Resolve(rb.Inputs, inputs); err != nil {
		return nil, err
	}

	order := make([]string, len(rb.Steps))
	steps := make([]runner.Step, len(rb.Steps))
	for i, st := range rb.Steps {
		st := st
		order[i] = st.ID
		steps[i] = runner.Step{
			ID:        st.ID,
			DependsOn: st.DependsOn,
			Timeout:   st.Timeout,
			Run: func(stepCtx context.Context, _ map[string]runner.Result) (any, error) {
				outcome := StepOutcome{StepID: st.ID, Name: st.Name}
				bound, err := params.Bind(st.SQL, rb.Inputs, inputs, params.BindOptions{NullForMissing: true})
				if err != nil {
					outcome.Error = err.Error()
					return outcome, err
				}
				outcome.SQL = bound

				res, err := qr.RunSQL(stepCtx, st.ConnectionID, bound)
				if err != nil {
					outcome.Error = err.Error()
					return outcome, err
				}
				outcome.Result = res
				return outcome, nil
			},
		}
	}

	runResults, err := runner.Run(ctx, runner.Plan{Steps: steps}, runner.Options{
		MaxParallel:    opts.MaxParallel,
		DefaultTimeout: opts.DefaultTimeout,
		StopOnError:    opts.StopOnError,
	})
	if err != nil {
		return nil, err
	}

	out := &RunResult{Outcomes: make(map[string]StepOutcome, len(runResults)), Order: order}
	for id, r := range runResults {
		outcome := StepOutcome{StepID: id, Status: r.Status}
		if oc, ok := r.Output.(StepOutcome); ok {
			outcome = oc
			outcome.Status = r.Status
		}
		if r.Status == runner.StatusSkipped {
			outcome.Skipped = r.SkipReason
		}
		if r.Status == runner.StatusFailed {
			out.Failed = true
		}
		out.Outcomes[id] = outcome
	}
	return out, nil
}
