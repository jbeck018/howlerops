// Package notebook models an exploratory, cell-based analysis document: typed
// inputs (widgets) and an ordered list of cells (SQL or markdown) that execute
// top to bottom, capturing each cell's output for display. It is built on the
// platform primitives — typed inputs via internal/params — and stays Wails-free
// and unit-testable; the host injects a QueryRunner to execute SQL.
//
// Notebooks are the exploratory counterpart to runbooks: read-only by design
// (no write actions), with markdown cells interleaved for narrative.
package notebook

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jbeck018/howlerops/internal/params"
)

// CellKind enumerates the kinds of cell.
type CellKind string

const (
	// CellSQL runs a read-only query and captures the result.
	CellSQL CellKind = "sql"
	// CellMarkdown renders narrative text (with {{input}} substitution).
	CellMarkdown CellKind = "markdown"
)

// Cell is one unit of a notebook.
type Cell struct {
	ID           string
	Title        string
	Kind         CellKind
	ConnectionID string // for SQL cells
	SQL          string // SQL template for SQL cells
	Markdown     string // text for markdown cells
}

// Notebook is a parameterized, cell-based analysis document.
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

// QueryRunner executes a SQL cell's bound query.
type QueryRunner interface {
	RunSQL(ctx context.Context, connectionID, sql string) (*QueryResult, error)
}

// Status is a cell's terminal state.
type Status string

const (
	StatusSuccess Status = "success"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
)

// CellResult captures a cell's rendered output.
type CellResult struct {
	CellID   string
	Title    string
	Kind     CellKind
	Status   Status
	Markdown string       // rendered markdown (markdown cells)
	SQL      string       // the bound SQL that ran (SQL cells)
	Result   *QueryResult // query output (SQL cells)
	Error    string
}

// RunResult is the outcome of executing a notebook.
type RunResult struct {
	Cells  []CellResult // in definition order
	Failed bool
}

// Options tune execution.
type Options struct {
	// StopOnError stops at the first failing cell, marking the rest skipped.
	// When false (default), execution continues so later independent cells
	// still render.
	StopOnError bool
}

// Validate checks the notebook is structurally sound.
func Validate(nb Notebook) error {
	for _, def := range nb.Inputs {
		if def.Name == "" {
			return fmt.Errorf("notebook: input with empty name")
		}
	}
	ids := make(map[string]bool, len(nb.Cells))
	for _, c := range nb.Cells {
		if c.ID == "" {
			return fmt.Errorf("notebook: cell with empty ID")
		}
		if ids[c.ID] {
			return fmt.Errorf("notebook: duplicate cell ID %q", c.ID)
		}
		ids[c.ID] = true
		switch c.Kind {
		case CellSQL:
			if c.ConnectionID == "" {
				return fmt.Errorf("notebook: SQL cell %q missing connection", c.ID)
			}
			if strings.TrimSpace(c.SQL) == "" {
				return fmt.Errorf("notebook: SQL cell %q missing SQL", c.ID)
			}
		case CellMarkdown:
			// markdown may be empty
		default:
			return fmt.Errorf("notebook: cell %q has unsupported kind %q", c.ID, c.Kind)
		}
	}
	return nil
}

// Execute validates inputs and runs the cells top to bottom, capturing outputs.
func Execute(ctx context.Context, nb Notebook, inputs map[string]any, qr QueryRunner, opts Options) (*RunResult, error) {
	if err := Validate(nb); err != nil {
		return nil, err
	}
	resolved, err := params.Resolve(nb.Inputs, inputs)
	if err != nil {
		return nil, err
	}

	out := &RunResult{Cells: make([]CellResult, 0, len(nb.Cells))}
	aborted := false
	for _, cell := range nb.Cells {
		res := CellResult{CellID: cell.ID, Title: cell.Title, Kind: cell.Kind}

		if aborted {
			res.Status = StatusSkipped
			out.Cells = append(out.Cells, res)
			continue
		}

		switch cell.Kind {
		case CellMarkdown:
			res.Markdown = renderText(cell.Markdown, resolved)
			res.Status = StatusSuccess

		default: // CellSQL
			bound, err := params.Bind(cell.SQL, nb.Inputs, inputs, params.BindOptions{NullForMissing: true})
			if err != nil {
				res.Status = StatusError
				res.Error = err.Error()
				out.Failed = true
				if opts.StopOnError {
					aborted = true
				}
				out.Cells = append(out.Cells, res)
				continue
			}
			res.SQL = bound
			if qr == nil {
				res.Status = StatusError
				res.Error = "no query runner configured"
				out.Failed = true
				if opts.StopOnError {
					aborted = true
				}
				out.Cells = append(out.Cells, res)
				continue
			}
			result, err := qr.RunSQL(ctx, cell.ConnectionID, bound)
			if err != nil {
				res.Status = StatusError
				res.Error = err.Error()
				out.Failed = true
				if opts.StopOnError {
					aborted = true
				}
			} else {
				res.Result = result
				res.Status = StatusSuccess
			}
		}
		out.Cells = append(out.Cells, res)
	}
	return out, nil
}

// renderText substitutes {{name}} placeholders with the plain (non-SQL-quoted)
// value of each resolved input, for markdown cells.
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
