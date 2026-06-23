// Package runbook models a reusable, parameterized operational task: typed
// inputs, an ordered (DAG) set of steps, and captured outputs. It is the Phase 4
// foundation and is built entirely on the platform primitives — typed inputs via
// internal/params and execution via internal/runner — so it stays Wails-free and
// unit-testable.
//
// Steps come in three kinds: read-only queries, writes ("actions"), and
// notifications. Writes are gated by a guardrail: a run can be a dry run (no
// writes/notifications are performed, only planned) and, when executing for
// real, each action must be approved (explicitly via an ApproveFunc, or by
// Options.AutoApprove for trusted automated runs).
package runbook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runner"
)

// StepKind enumerates what a step does.
type StepKind string

const (
	// StepQuery runs a read-only SQL query.
	StepQuery StepKind = "query"
	// StepAction runs a write/mutation statement, gated by the approval guardrail.
	StepAction StepKind = "action"
	// StepNotify emits a notification message on a channel.
	StepNotify StepKind = "notify"
)

// Step is one unit of a runbook.
type Step struct {
	ID        string
	Name      string
	Kind      StepKind
	DependsOn []string

	// Query/Action steps.
	ConnectionID string
	SQL          string // template bound from inputs via internal/params

	// Notify steps.
	Channel string
	Message string // text template; {{input}} placeholders are substituted plainly

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

// QueryResult is the tabular output of a query step.
type QueryResult struct {
	Columns  []string
	Rows     []map[string]any
	RowCount int64
}

// QueryRunner executes a read-only query step.
type QueryRunner interface {
	RunSQL(ctx context.Context, connectionID, sql string) (*QueryResult, error)
}

// ActionRunner executes a write/mutation step and returns rows affected.
type ActionRunner interface {
	ExecSQL(ctx context.Context, connectionID, sql string) (rowsAffected int64, err error)
}

// Notifier delivers a notification.
type Notifier interface {
	Notify(ctx context.Context, channel, message string) error
}

// ActionRequest describes a pending write for the approval guardrail.
type ActionRequest struct {
	StepID       string
	Name         string
	ConnectionID string
	SQL          string // the bound statement about to run
}

// ApproveFunc decides whether a write may proceed. Returning false blocks the
// step (and its dependents); returning an error aborts the run.
type ApproveFunc func(ctx context.Context, req ActionRequest) (bool, error)

// Deps are the host-provided capabilities the executor uses.
type Deps struct {
	Query   QueryRunner
	Action  ActionRunner
	Notify  Notifier
	Approve ApproveFunc
}

// Options tune execution.
type Options struct {
	MaxParallel    int
	DefaultTimeout time.Duration
	// StopOnError aborts not-yet-started steps after the first failure. When
	// false (default), only steps depending on a failed step are skipped.
	StopOnError bool
	// DryRun plans writes and notifications without performing them (read-only
	// query steps still run). The safe default for previewing a runbook.
	DryRun bool
	// AutoApprove bypasses the per-action approval prompt (for trusted,
	// automated runs). Ignored in DryRun.
	AutoApprove bool
}

// StepOutcome captures the result of one executed (or skipped/planned) step.
type StepOutcome struct {
	StepID       string
	Name         string
	Kind         StepKind
	Status       runner.Status
	SQL          string // the bound SQL that ran (or would run)
	Result       *QueryResult
	RowsAffected int64
	Message      string // rendered notify message
	Notified     bool
	Planned      bool // dry-run: would have executed but did not
	Error        string
	Skipped      string // reason, when Status == skipped
}

// RunResult is the outcome of executing a runbook.
type RunResult struct {
	Outcomes map[string]StepOutcome
	Order    []string // step IDs in definition order
	Failed   bool
	DryRun   bool
}

// Validate checks the runbook is structurally sound.
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
		case StepQuery, StepAction, "":
			if st.ConnectionID == "" {
				return fmt.Errorf("runbook: step %q missing connection", st.ID)
			}
			if st.SQL == "" {
				return fmt.Errorf("runbook: step %q missing SQL", st.ID)
			}
		case StepNotify:
			if strings.TrimSpace(st.Message) == "" {
				return fmt.Errorf("runbook: notify step %q missing message", st.ID)
			}
		default:
			return fmt.Errorf("runbook: step %q has unsupported kind %q", st.ID, st.Kind)
		}
	}
	return runner.Validate(runner.Plan{Steps: steps})
}

// Execute validates inputs, binds each step, and runs the steps on the shared
// runner engine, honoring the dry-run/approval guardrail for writes and
// notifications.
func Execute(ctx context.Context, rb Runbook, inputs map[string]any, deps Deps, opts Options) (*RunResult, error) {
	if err := Validate(rb); err != nil {
		return nil, err
	}
	resolved, err := params.Resolve(rb.Inputs, inputs)
	if err != nil {
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
				return runStep(stepCtx, rb, st, inputs, resolved, deps, opts)
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

	out := &RunResult{Outcomes: make(map[string]StepOutcome, len(runResults)), Order: order, DryRun: opts.DryRun}
	for id, r := range runResults {
		outcome := StepOutcome{StepID: id, Status: r.Status}
		// On success the runner carries the outcome as Output; on failure it
		// carries only the error (Output is nil), so recover the rich outcome
		// from the wrapping stepError so Error/Name/Kind/SQL survive.
		if oc, ok := r.Output.(StepOutcome); ok {
			outcome = oc
		} else if se, ok := r.Err.(*stepError); ok {
			outcome = se.outcome
		}
		outcome.Status = r.Status
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

// stepError wraps a step's failure together with its captured outcome so the
// runner (which only retains the error on failure) can hand back the full
// StepOutcome — preserving Name/Kind/SQL/Error for the run history and UI.
type stepError struct {
	outcome StepOutcome
	err     error
}

func (e *stepError) Error() string { return e.err.Error() }
func (e *stepError) Unwrap() error { return e.err }

// fail attaches the outcome to the error returned to the runner.
func fail(outcome StepOutcome, err error) (any, error) {
	return outcome, &stepError{outcome: outcome, err: err}
}

// runStep executes a single step according to its kind and the guardrail.
func runStep(ctx context.Context, rb Runbook, st Step, inputs map[string]any, resolved map[string]params.Value, deps Deps, opts Options) (any, error) {
	outcome := StepOutcome{StepID: st.ID, Name: st.Name, Kind: st.Kind}

	switch st.Kind {
	case StepNotify:
		msg := renderText(st.Message, resolved)
		outcome.Message = msg
		if opts.DryRun {
			outcome.Planned = true
			return outcome, nil
		}
		if deps.Notify == nil {
			outcome.Error = "no notifier configured"
			return fail(outcome, fmt.Errorf("runbook: step %q: no notifier configured", st.ID))
		}
		if err := deps.Notify.Notify(ctx, st.Channel, msg); err != nil {
			outcome.Error = err.Error()
			return fail(outcome, err)
		}
		outcome.Notified = true
		return outcome, nil

	case StepAction:
		bound, err := bindSQL(st.SQL, rb.Inputs, inputs)
		if err != nil {
			outcome.Error = err.Error()
			return fail(outcome, err)
		}
		outcome.SQL = bound
		if opts.DryRun {
			outcome.Planned = true
			return outcome, nil
		}
		// Approval guardrail.
		approved := opts.AutoApprove
		if !approved && deps.Approve != nil {
			ok, err := deps.Approve(ctx, ActionRequest{StepID: st.ID, Name: st.Name, ConnectionID: st.ConnectionID, SQL: bound})
			if err != nil {
				outcome.Error = err.Error()
				return fail(outcome, err)
			}
			approved = ok
		}
		if !approved {
			outcome.Error = "action not approved"
			return fail(outcome, fmt.Errorf("runbook: step %q: action not approved", st.ID))
		}
		if deps.Action == nil {
			outcome.Error = "no action runner configured"
			return fail(outcome, fmt.Errorf("runbook: step %q: no action runner configured", st.ID))
		}
		affected, err := deps.Action.ExecSQL(ctx, st.ConnectionID, bound)
		if err != nil {
			outcome.Error = err.Error()
			return fail(outcome, err)
		}
		outcome.RowsAffected = affected
		return outcome, nil

	default: // StepQuery
		bound, err := bindSQL(st.SQL, rb.Inputs, inputs)
		if err != nil {
			outcome.Error = err.Error()
			return fail(outcome, err)
		}
		outcome.SQL = bound
		if deps.Query == nil {
			outcome.Error = "no query runner configured"
			return fail(outcome, fmt.Errorf("runbook: step %q: no query runner configured", st.ID))
		}
		res, err := deps.Query.RunSQL(ctx, st.ConnectionID, bound)
		if err != nil {
			outcome.Error = err.Error()
			return fail(outcome, err)
		}
		outcome.Result = res
		return outcome, nil
	}
}

func bindSQL(sql string, defs []params.Definition, inputs map[string]any) (string, error) {
	return params.Bind(sql, defs, inputs, params.BindOptions{NullForMissing: true})
}

// renderText substitutes {{name}} placeholders in a plain-text template with the
// human-readable form of each resolved input (no SQL quoting), for notification
// messages.
func renderText(template string, resolved map[string]params.Value) string {
	out := template
	for name, v := range resolved {
		out = strings.ReplaceAll(out, "{{"+name+"}}", plainValue(v.Raw()))
	}
	return out
}

func plainValue(raw any) string {
	switch t := raw.(type) {
	case time.Time:
		return t.UTC().Format(time.RFC3339)
	case []params.Value:
		parts := make([]string, len(t))
		for i, v := range t {
			parts[i] = plainValue(v.Raw())
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", raw)
	}
}
