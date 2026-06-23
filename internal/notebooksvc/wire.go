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
}

// CellDTO is a single notebook cell.
type CellDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title,omitempty"`
	Kind         string `json:"kind"`
	ConnectionID string `json:"connectionId,omitempty"`
	SQL          string `json:"sql,omitempty"`
	Markdown     string `json:"markdown,omitempty"`
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
		})
	}
	for _, c := range d.Cells {
		nb.Cells = append(nb.Cells, notebook.Cell{
			ID:           c.ID,
			Title:        c.Title,
			Kind:         notebook.CellKind(c.Kind),
			ConnectionID: c.ConnectionID,
			SQL:          c.SQL,
			Markdown:     c.Markdown,
		})
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
		})
	}
	for _, c := range nb.Cells {
		d.Cells = append(d.Cells, CellDTO{
			ID:           c.ID,
			Title:        c.Title,
			Kind:         string(c.Kind),
			ConnectionID: c.ConnectionID,
			SQL:          c.SQL,
			Markdown:     c.Markdown,
		})
	}
	return d
}

// CellResultDTO is the wire form of a cell's output.
type CellResultDTO struct {
	CellID   string           `json:"cellId"`
	Title    string           `json:"title,omitempty"`
	Kind     string           `json:"kind"`
	Status   string           `json:"status"`
	Markdown string           `json:"markdown,omitempty"`
	SQL      string           `json:"sql,omitempty"`
	Columns  []string         `json:"columns,omitempty"`
	Rows     []map[string]any `json:"rows,omitempty"`
	RowCount int64            `json:"rowCount,omitempty"`
	Error    string           `json:"error,omitempty"`
}

// RunResultDTO is the wire form of a notebook run.
type RunResultDTO struct {
	Failed bool            `json:"failed"`
	Cells  []CellResultDTO `json:"cells"`
}

// ResultToDTO flattens a RunResult into wire cell outputs.
func ResultToDTO(res *notebook.RunResult) RunResultDTO {
	out := RunResultDTO{Failed: res.Failed}
	for _, c := range res.Cells {
		dto := CellResultDTO{
			CellID:   c.CellID,
			Title:    c.Title,
			Kind:     string(c.Kind),
			Status:   string(c.Status),
			Markdown: c.Markdown,
			SQL:      c.SQL,
			Error:    c.Error,
		}
		if c.Result != nil {
			dto.Columns = c.Result.Columns
			dto.Rows = c.Result.Rows
			dto.RowCount = c.Result.RowCount
		}
		out.Cells = append(out.Cells, dto)
	}
	return out
}
