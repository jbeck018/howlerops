// Package notebook models an interactive, reactive, cell-based analysis document
// — the unified surface that merges the old read-only "notebook" and the
// operational "runbook" into one thing. A notebook has typed inputs (widgets)
// and a set of cells that form a dependency DAG. Cells come in five kinds:
//
//   - markdown : narrative text with {{input}} substitution
//   - sql      : a read-only query
//   - action   : a write/mutation, gated by the dry-run + approval guardrail
//   - notify   : a notification message on a channel
//   - chart    : a visualization over another cell's result
//
// Reactivity (marimo / DuckDB-UI style): each SQL cell has a stable handle
// (Name). A cell that references another cell's handle in its SQL depends on it,
// and runs against the staged compute engine (DuckDB) where upstream results are
// registered as named tables — so cells compose. Editing one cell re-runs it and
// its descendants. The dependency edges are the explicit DependsOn plus the ones
// inferred from handle references in SQL.
//
// The package is built on the platform primitives — typed inputs via
// internal/params and DAG execution via internal/runner — and stays Wails-free
// and unit-testable. The host injects the capabilities (Deps): a QueryRunner for
// real connections, an ActionRunner for writes, a Notifier, an approval
// guardrail, and a Stager for cross-cell composition.
package notebook

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runner"
)

// CellKind enumerates the kinds of cell.
type CellKind string

const (
	// CellMarkdown renders narrative text (with {{input}} substitution).
	CellMarkdown CellKind = "markdown"
	// CellSQL runs a read-only query and captures the result.
	CellSQL CellKind = "sql"
	// CellAction runs a write/mutation statement, gated by the approval guardrail.
	CellAction CellKind = "action"
	// CellNotify emits a notification message on a channel.
	CellNotify CellKind = "notify"
	// CellChart renders a visualization over another cell's result.
	CellChart CellKind = "chart"
)

// ChartSpec describes how a chart cell visualizes a source cell's result.
type ChartSpec struct {
	Source  string   // handle (Name) of the cell whose result to chart
	Type    string   // bar | line | area | pie | scatter
	X       string   // column for the category/x axis
	Y       []string // one or more value columns for the y axis
	Stacked bool
}

// Cell is one unit of a notebook.
type Cell struct {
	ID    string
	Name  string // stable handle; used as the staged table name and for references
	Title string
	Kind  CellKind

	// DependsOn lists explicit upstream cell IDs. Reactive references inferred
	// from SQL (a cell mentioning another cell's Name) are added on top.
	DependsOn []string

	// SQL/Action cells.
	ConnectionID string
	SQL          string // template bound from inputs via internal/params

	// Markdown cells.
	Markdown string

	// Notify cells.
	Channel string
	Message string // text template; {{input}} placeholders are substituted plainly

	// Chart cells.
	Chart *ChartSpec

	// Timeout bounds this cell; 0 uses the run-wide default.
	Timeout time.Duration
}

// Notebook is a parameterized, reactive, cell-based document.
type Notebook struct {
	ID          string
	Name        string
	Description string
	Inputs      []params.Definition
	Cells       []Cell
}

// QueryResult is the tabular output of a SQL cell.
type QueryResult struct {
	Columns  []string
	Rows     []map[string]any
	RowCount int64
}

// QueryRunner executes a read-only query against a real connection.
type QueryRunner interface {
	RunSQL(ctx context.Context, connectionID, sql string) (*QueryResult, error)
}

// ActionRunner executes a write/mutation against a real connection.
type ActionRunner interface {
	ExecSQL(ctx context.Context, connectionID, sql string) (rowsAffected int64, err error)
}

// Notifier delivers a notification.
type Notifier interface {
	Notify(ctx context.Context, channel, message string) error
}

// ActionRequest describes a pending write for the approval guardrail.
type ActionRequest struct {
	CellID       string
	Name         string
	ConnectionID string
	SQL          string // the bound statement about to run
}

// ApproveFunc decides whether a write may proceed. Returning false blocks the
// cell (and its dependents); returning an error aborts the run.
type ApproveFunc func(ctx context.Context, req ActionRequest) (bool, error)

// Stager stages each SQL cell's result as a named, queryable table so downstream
// cells can compose against it (the DuckDB-UI / marimo model). A run uses a
// single Stager whose tables persist for the lifetime of the document session,
// enabling reactive partial re-runs (only re-run changed cells + descendants,
// reusing upstream staged tables). The no-op NoStager disables composition: a
// cell that references another cell errors with a clear message.
type Stager interface {
	// Available reports whether cross-cell composition is supported (e.g. the
	// duckdb build is present and the engine initialized).
	Available() bool
	// Reset drops all staged tables; called at the start of a full run.
	Reset(ctx context.Context) error
	// Stage registers (replacing any prior) a cell's result under a table name.
	Stage(ctx context.Context, table string, res *QueryResult) error
	// Query runs SQL that references staged tables and returns the result.
	Query(ctx context.Context, sql string) (*QueryResult, error)
}

// NoStager is the default Stager: composition is unavailable.
type NoStager struct{}

func (NoStager) Available() bool                                   { return false }
func (NoStager) Reset(context.Context) error                       { return nil }
func (NoStager) Stage(context.Context, string, *QueryResult) error { return nil }
func (NoStager) Query(context.Context, string) (*QueryResult, error) {
	return nil, fmt.Errorf("notebook: cross-cell composition requires the DuckDB compute engine, which is not available in this build")
}

// Deps are the host-provided capabilities the executor uses.
type Deps struct {
	Query   QueryRunner
	Action  ActionRunner
	Notify  Notifier
	Approve ApproveFunc
	Stage   Stager // optional; defaults to NoStager
}

// Options tune execution.
type Options struct {
	MaxParallel    int
	DefaultTimeout time.Duration
	// StopOnError aborts not-yet-started cells after the first failure. When
	// false (default), only cells depending on a failed cell are skipped.
	StopOnError bool
	// DryRun plans writes and notifications without performing them (read-only
	// SQL cells still run). The safe default for previewing.
	DryRun bool
	// AutoApprove bypasses the per-action approval prompt (for trusted runs).
	// Ignored in DryRun.
	AutoApprove bool
	// Only, when non-empty, restricts execution to the given cell IDs plus all of
	// their descendants (a reactive re-run). Cells outside that closure are left
	// untouched — marked Preserved — and their previously staged tables are
	// assumed to still be present. When empty, every cell runs (a full run) and
	// the Stager is reset first.
	Only []string
}

// Status is a cell's terminal state.
type Status string

const (
	StatusSuccess   Status = "success"
	StatusError     Status = "error"
	StatusSkipped   Status = "skipped"   // a dependency failed/was skipped, or the run aborted
	StatusPreserved Status = "preserved" // not re-run during a partial run; prior output retained
)

// CellResult captures a cell's rendered output.
type CellResult struct {
	CellID   string
	Name     string
	Title    string
	Kind     CellKind
	Status   Status
	Markdown string       // rendered markdown (markdown cells)
	SQL      string       // the bound SQL that ran (sql/action cells)
	Result   *QueryResult // query output (sql cells)
	Rows     int64        // rows affected (action cells)
	Message  string       // rendered notify message (notify cells)
	Notified bool         // a notification was sent (notify cells)
	Planned  bool         // dry-run: would have executed but did not
	Chart    *ChartSpec   // chart spec (chart cells)
	Error    string
	Skipped  string // reason, when Status == skipped
}

// RunResult is the outcome of executing a notebook.
type RunResult struct {
	Cells  []CellResult // in definition order
	Order  []string     // cell IDs in definition order
	Failed bool
	DryRun bool
}

// handleRe matches a valid cell handle (SQL identifier).
var handleRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks the notebook is structurally sound, including the DAG.
func Validate(nb Notebook) error {
	for _, def := range nb.Inputs {
		if def.Name == "" {
			return fmt.Errorf("notebook: input with empty name")
		}
	}
	ids := make(map[string]bool, len(nb.Cells))
	handles := make(map[string]bool, len(nb.Cells))
	byID := make(map[string]Cell, len(nb.Cells))
	for _, c := range nb.Cells {
		if c.ID == "" {
			return fmt.Errorf("notebook: cell with empty ID")
		}
		if ids[c.ID] {
			return fmt.Errorf("notebook: duplicate cell ID %q", c.ID)
		}
		ids[c.ID] = true
		byID[c.ID] = c
		if c.Name != "" {
			if !handleRe.MatchString(c.Name) {
				return fmt.Errorf("notebook: cell %q has invalid handle %q (must be a SQL identifier)", c.ID, c.Name)
			}
			if handles[c.Name] {
				return fmt.Errorf("notebook: duplicate cell handle %q", c.Name)
			}
			handles[c.Name] = true
		}
	}

	// The dependency graph drives both the connection requirement (a composing
	// cell needs no connection — it runs on the staged compute engine) and the
	// DAG validity check (unknown deps, cycles).
	graph := buildGraph(nb)
	for _, c := range nb.Cells {
		switch c.Kind {
		case CellSQL:
			if strings.TrimSpace(c.SQL) == "" {
				return fmt.Errorf("notebook: sql cell %q missing SQL", c.ID)
			}
			if c.ConnectionID == "" && !composes(c, graph, byID) {
				return fmt.Errorf("notebook: sql cell %q missing connection", c.ID)
			}
		case CellAction:
			if strings.TrimSpace(c.SQL) == "" {
				return fmt.Errorf("notebook: action cell %q missing SQL", c.ID)
			}
			if c.ConnectionID == "" {
				return fmt.Errorf("notebook: action cell %q missing connection", c.ID)
			}
		case CellNotify:
			if strings.TrimSpace(c.Message) == "" {
				return fmt.Errorf("notebook: notify cell %q missing message", c.ID)
			}
		case CellChart:
			if c.Chart == nil || strings.TrimSpace(c.Chart.Source) == "" {
				return fmt.Errorf("notebook: chart cell %q missing source", c.ID)
			}
		case CellMarkdown:
			// markdown may be empty
		default:
			return fmt.Errorf("notebook: cell %q has unsupported kind %q", c.ID, c.Kind)
		}
	}

	steps := make([]runner.Step, len(nb.Cells))
	for i, c := range nb.Cells {
		steps[i] = runner.Step{ID: c.ID, DependsOn: graph[c.ID]}
	}
	return runner.Validate(runner.Plan{Steps: steps})
}

// buildGraph returns each cell's full dependency set: explicit DependsOn plus
// edges inferred from references to other cells' handles in SQL / chart source.
func buildGraph(nb Notebook) map[string][]string {
	// Map handle -> cell ID, and a matcher per handle.
	handleToID := make(map[string]string)
	for _, c := range nb.Cells {
		if c.Name != "" {
			handleToID[c.Name] = c.ID
		}
	}
	graph := make(map[string][]string, len(nb.Cells))
	for _, c := range nb.Cells {
		seen := make(map[string]bool)
		var deps []string
		add := func(id string) {
			if id == "" || id == c.ID || seen[id] {
				return
			}
			seen[id] = true
			deps = append(deps, id)
		}
		for _, d := range c.DependsOn {
			add(d)
		}
		switch c.Kind {
		case CellSQL, CellAction:
			for handle, id := range handleToID {
				if id == c.ID {
					continue
				}
				if referencesHandle(c.SQL, handle) {
					add(id)
				}
			}
		case CellChart:
			if c.Chart != nil {
				add(handleToID[c.Chart.Source])
			}
		}
		graph[c.ID] = deps
	}
	return graph
}

// referencesHandle reports whether sql mentions handle as a whole-word token.
func referencesHandle(sql, handle string) bool {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(handle) + `\b`)
	return re.MatchString(sql)
}

// composes reports whether a SQL/action cell references any other cell's handle
// (and therefore must run against the staged compute engine).
func composes(c Cell, graph map[string][]string, byID map[string]Cell) bool {
	for _, dep := range graph[c.ID] {
		up := byID[dep]
		if up.Name != "" && referencesHandle(c.SQL, up.Name) {
			return true
		}
	}
	return false
}

// Descendants returns the transitive closure of cells reachable from seeds
// following dependency edges downstream (consumers of the seeds), including the
// seeds themselves. Used for reactive partial re-runs.
func Descendants(nb Notebook, seeds []string) []string {
	graph := buildGraph(nb)
	// Invert: dependents[x] = cells that depend on x.
	dependents := make(map[string][]string)
	for id, deps := range graph {
		for _, d := range deps {
			dependents[d] = append(dependents[d], id)
		}
	}
	in := make(map[string]bool)
	var stack []string
	for _, s := range seeds {
		if !in[s] {
			in[s] = true
			stack = append(stack, s)
		}
	}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, dep := range dependents[cur] {
			if !in[dep] {
				in[dep] = true
				stack = append(stack, dep)
			}
		}
	}
	// Return in definition order for stable output.
	var out []string
	for _, c := range nb.Cells {
		if in[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out
}

// Execute validates the notebook, resolves inputs, and runs the cells on the
// shared DAG runner — staging each SQL cell's result so downstream cells compose,
// and honoring the dry-run / approval guardrail for writes and notifications.
func Execute(ctx context.Context, nb Notebook, inputs map[string]any, deps Deps, opts Options) (*RunResult, error) {
	if err := Validate(nb); err != nil {
		return nil, err
	}
	resolved, err := params.Resolve(nb.Inputs, inputs)
	if err != nil {
		return nil, err
	}
	if deps.Stage == nil {
		deps.Stage = NoStager{}
	}

	graph := buildGraph(nb)
	byID := make(map[string]Cell, len(nb.Cells))
	for _, c := range nb.Cells {
		byID[c.ID] = c
	}

	// Determine the active set (reactive partial re-run vs full run).
	active := map[string]bool(nil)
	if len(opts.Only) > 0 {
		active = make(map[string]bool)
		for _, id := range Descendants(nb, opts.Only) {
			active[id] = true
		}
	} else if deps.Stage.Available() {
		// Full run: clear prior staged tables.
		if err := deps.Stage.Reset(ctx); err != nil {
			return nil, fmt.Errorf("notebook: reset staging: %w", err)
		}
	}

	order := make([]string, len(nb.Cells))
	steps := make([]runner.Step, len(nb.Cells))
	for i, c := range nb.Cells {
		c := c
		order[i] = c.ID
		steps[i] = runner.Step{
			ID:        c.ID,
			DependsOn: graph[c.ID],
			Timeout:   c.Timeout,
			Run: func(stepCtx context.Context, _ map[string]runner.Result) (any, error) {
				if active != nil && !active[c.ID] {
					return CellResult{CellID: c.ID, Name: c.Name, Title: c.Title, Kind: c.Kind, Status: StatusPreserved}, nil
				}
				return runCell(stepCtx, nb, c, inputs, resolved, graph, byID, deps, opts)
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

	out := &RunResult{Cells: make([]CellResult, 0, len(nb.Cells)), Order: order, DryRun: opts.DryRun}
	for _, id := range order {
		r := runResults[id]
		var cr CellResult
		if oc, ok := r.Output.(CellResult); ok {
			cr = oc
		} else if se, ok := r.Err.(*cellError); ok {
			cr = se.result
		} else {
			cr = CellResult{CellID: id, Name: byID[id].Name, Title: byID[id].Title, Kind: byID[id].Kind}
		}
		switch r.Status {
		case runner.StatusSkipped:
			cr.Status = StatusSkipped
			cr.Skipped = r.SkipReason
		case runner.StatusFailed:
			cr.Status = StatusError
			out.Failed = true
		}
		out.Cells = append(out.Cells, cr)
	}
	return out, nil
}

// cellError wraps a cell's failure together with its captured result so the
// runner (which only retains the error on failure) can hand back the full
// CellResult — preserving Name/Kind/SQL/Error for the run history and UI.
type cellError struct {
	result CellResult
	err    error
}

func (e *cellError) Error() string { return e.err.Error() }
func (e *cellError) Unwrap() error { return e.err }

func fail(res CellResult, err error) (any, error) {
	res.Status = StatusError
	res.Error = err.Error()
	return res, &cellError{result: res, err: err}
}

// runCell executes a single cell according to its kind, the composition model,
// and the dry-run/approval guardrail.
func runCell(ctx context.Context, nb Notebook, c Cell, inputs map[string]any, resolved map[string]params.Value, graph map[string][]string, byID map[string]Cell, deps Deps, opts Options) (any, error) {
	res := CellResult{CellID: c.ID, Name: c.Name, Title: c.Title, Kind: c.Kind, Status: StatusSuccess}

	switch c.Kind {
	case CellMarkdown:
		res.Markdown = renderText(c.Markdown, resolved)
		return res, nil

	case CellNotify:
		msg := renderText(c.Message, resolved)
		res.Message = msg
		if opts.DryRun {
			res.Planned = true
			return res, nil
		}
		if deps.Notify == nil {
			return fail(res, fmt.Errorf("notebook: cell %q: no notifier configured", c.ID))
		}
		if err := deps.Notify.Notify(ctx, c.Channel, msg); err != nil {
			return fail(res, err)
		}
		res.Notified = true
		return res, nil

	case CellChart:
		// Charts are rendered on the frontend from their source cell's result;
		// the engine validates the dependency and passes the spec through.
		res.Chart = c.Chart
		return res, nil

	case CellAction:
		bound, err := params.Bind(c.SQL, nb.Inputs, inputs, params.BindOptions{NullForMissing: true})
		if err != nil {
			return fail(res, err)
		}
		res.SQL = bound
		if opts.DryRun {
			res.Planned = true
			return res, nil
		}
		approved := opts.AutoApprove
		if !approved && deps.Approve != nil {
			ok, err := deps.Approve(ctx, ActionRequest{CellID: c.ID, Name: c.Name, ConnectionID: c.ConnectionID, SQL: bound})
			if err != nil {
				return fail(res, err)
			}
			approved = ok
		}
		if !approved {
			return fail(res, fmt.Errorf("notebook: cell %q: action not approved", c.ID))
		}
		if deps.Action == nil {
			return fail(res, fmt.Errorf("notebook: cell %q: no action runner configured", c.ID))
		}
		affected, err := deps.Action.ExecSQL(ctx, c.ConnectionID, bound)
		if err != nil {
			return fail(res, err)
		}
		res.Rows = affected
		return res, nil

	default: // CellSQL
		bound, err := params.Bind(c.SQL, nb.Inputs, inputs, params.BindOptions{NullForMissing: true})
		if err != nil {
			return fail(res, err)
		}
		res.SQL = bound

		var result *QueryResult
		if composes(c, graph, byID) {
			// References other cells: run against the staged compute engine.
			if !deps.Stage.Available() {
				return fail(res, fmt.Errorf("notebook: cell %q composes other cells but the DuckDB compute engine is unavailable in this build", c.ID))
			}
			result, err = deps.Stage.Query(ctx, bound)
		} else {
			if deps.Query == nil {
				return fail(res, fmt.Errorf("notebook: cell %q: no query runner configured", c.ID))
			}
			result, err = deps.Query.RunSQL(ctx, c.ConnectionID, bound)
		}
		if err != nil {
			return fail(res, err)
		}
		res.Result = result

		// Stage this cell's result so downstream cells can reference it by handle.
		if c.Name != "" && deps.Stage.Available() {
			if err := deps.Stage.Stage(ctx, c.Name, result); err != nil {
				return fail(res, fmt.Errorf("notebook: stage cell %q: %w", c.ID, err))
			}
		}
		return res, nil
	}
}

// renderText substitutes {{name}} placeholders with the plain (non-SQL-quoted)
// value of each resolved input, for markdown and notify cells.
func renderText(template string, resolved map[string]params.Value) string {
	return params.Substitute(template, func(name string) (string, bool) {
		v, ok := resolved[name]
		if !ok {
			return "", false
		}
		return plainValue(v.Raw()), true
	})
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
