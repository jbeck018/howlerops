package notebooksvc

import (
	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/params"
)

// Wire DTOs are JSON/Wails-friendly shapes for the frontend boundary, defined
// here (not in package main) so the engine<->wire mapping is unit-testable.

// InputDTO is a typed notebook input (widget) definition.
type InputDTO struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	// ElementType types the elements of a list input (e.g. "integer" so an IN
	// clause renders bare numbers, not quoted strings). Empty defaults to string.
	ElementType string `json:"elementType,omitempty"`
}

// ChartSpecDTO describes a chart cell's visualization.
type ChartSpecDTO struct {
	Source  string   `json:"source"`
	Type    string   `json:"type,omitempty"`
	X       string   `json:"x,omitempty"`
	Y       []string `json:"y,omitempty"`
	Stacked bool     `json:"stacked,omitempty"`
}

// CellDTO is a single notebook cell.
type CellDTO struct {
	ID           string        `json:"id"`
	Name         string        `json:"name,omitempty"` // stable handle for references
	Title        string        `json:"title,omitempty"`
	Kind         string        `json:"kind"`
	DependsOn    []string      `json:"dependsOn,omitempty"`
	ConnectionID string        `json:"connectionId,omitempty"`
	SQL          string        `json:"sql,omitempty"`
	Markdown     string        `json:"markdown,omitempty"`
	Channel      string        `json:"channel,omitempty"`
	Message      string        `json:"message,omitempty"`
	Chart        *ChartSpecDTO `json:"chart,omitempty"`
}

// DefinitionDTO is the wire form of a notebook definition.
type DefinitionDTO struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Inputs      []InputDTO `json:"inputs,omitempty"`
	Cells       []CellDTO  `json:"cells,omitempty"`
}

// ToNotebook converts a wire definition into the engine type.
func (d DefinitionDTO) ToNotebook() notebook.Notebook {
	nb := notebook.Notebook{ID: d.ID, Name: d.Name, Description: d.Description}
	for _, in := range d.Inputs {
		nb.Inputs = append(nb.Inputs, params.Definition{
			Name:        in.Name,
			Type:        params.Type(in.Type),
			Label:       in.Label,
			Description: in.Description,
			Required:    in.Required,
			Default:     in.Default,
			Options:     in.Options,
			Pattern:     in.Pattern,
			Min:         in.Min,
			Max:         in.Max,
			ElementType: params.Type(in.ElementType),
		})
	}
	for _, c := range d.Cells {
		cell := notebook.Cell{
			ID:           c.ID,
			Name:         c.Name,
			Title:        c.Title,
			Kind:         notebook.CellKind(c.Kind),
			DependsOn:    c.DependsOn,
			ConnectionID: c.ConnectionID,
			SQL:          c.SQL,
			Markdown:     c.Markdown,
			Channel:      c.Channel,
			Message:      c.Message,
		}
		if c.Chart != nil {
			cell.Chart = &notebook.ChartSpec{
				Source:  c.Chart.Source,
				Type:    c.Chart.Type,
				X:       c.Chart.X,
				Y:       c.Chart.Y,
				Stacked: c.Chart.Stacked,
			}
		}
		nb.Cells = append(nb.Cells, cell)
	}
	return nb
}

// DefinitionFromNotebook converts an engine notebook into its wire form.
func DefinitionFromNotebook(nb *notebook.Notebook) DefinitionDTO {
	d := DefinitionDTO{ID: nb.ID, Name: nb.Name, Description: nb.Description}
	for _, in := range nb.Inputs {
		d.Inputs = append(d.Inputs, InputDTO{
			Name:        in.Name,
			Type:        string(in.Type),
			Label:       in.Label,
			Description: in.Description,
			Required:    in.Required,
			Default:     in.Default,
			Options:     in.Options,
			Pattern:     in.Pattern,
			Min:         in.Min,
			Max:         in.Max,
			ElementType: string(in.ElementType),
		})
	}
	for _, c := range nb.Cells {
		cell := CellDTO{
			ID:           c.ID,
			Name:         c.Name,
			Title:        c.Title,
			Kind:         string(c.Kind),
			DependsOn:    c.DependsOn,
			ConnectionID: c.ConnectionID,
			SQL:          c.SQL,
			Markdown:     c.Markdown,
			Channel:      c.Channel,
			Message:      c.Message,
		}
		if c.Chart != nil {
			cell.Chart = &ChartSpecDTO{
				Source:  c.Chart.Source,
				Type:    c.Chart.Type,
				X:       c.Chart.X,
				Y:       c.Chart.Y,
				Stacked: c.Chart.Stacked,
			}
		}
		d.Cells = append(d.Cells, cell)
	}
	return d
}

// CellResultDTO is the wire form of a cell's output.
type CellResultDTO struct {
	CellID   string           `json:"cellId"`
	Name     string           `json:"name,omitempty"`
	Title    string           `json:"title,omitempty"`
	Kind     string           `json:"kind"`
	Status   string           `json:"status"`
	Markdown string           `json:"markdown,omitempty"`
	SQL      string           `json:"sql,omitempty"`
	Columns  []string         `json:"columns,omitempty"`
	Rows     []map[string]any `json:"rows,omitempty"`
	RowCount int64            `json:"rowCount,omitempty"`
	Affected int64            `json:"affected,omitempty"`
	Message  string           `json:"message,omitempty"`
	Notified bool             `json:"notified,omitempty"`
	Planned  bool             `json:"planned,omitempty"`
	Chart    *ChartSpecDTO    `json:"chart,omitempty"`
	Error    string           `json:"error,omitempty"`
	Skipped  string           `json:"skipped,omitempty"`
}

// RunResultDTO is the wire form of a notebook run.
type RunResultDTO struct {
	Failed bool            `json:"failed"`
	DryRun bool            `json:"dryRun"`
	Cells  []CellResultDTO `json:"cells"`
}

// ResultToDTO flattens a RunResult into wire cell outputs.
func ResultToDTO(res *notebook.RunResult) RunResultDTO {
	out := RunResultDTO{Failed: res.Failed, DryRun: res.DryRun}
	for _, c := range res.Cells {
		dto := CellResultDTO{
			CellID:   c.CellID,
			Name:     c.Name,
			Title:    c.Title,
			Kind:     string(c.Kind),
			Status:   string(c.Status),
			Markdown: c.Markdown,
			SQL:      c.SQL,
			Affected: c.Rows,
			Message:  c.Message,
			Notified: c.Notified,
			Planned:  c.Planned,
			Error:    c.Error,
			Skipped:  c.Skipped,
		}
		if c.Result != nil {
			dto.Columns = c.Result.Columns
			dto.Rows = c.Result.Rows
			dto.RowCount = c.Result.RowCount
		}
		if c.Chart != nil {
			dto.Chart = &ChartSpecDTO{
				Source:  c.Chart.Source,
				Type:    c.Chart.Type,
				X:       c.Chart.X,
				Y:       c.Chart.Y,
				Stacked: c.Chart.Stacked,
			}
		}
		out.Cells = append(out.Cells, dto)
	}
	return out
}
