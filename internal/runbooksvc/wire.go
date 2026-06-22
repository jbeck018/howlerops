package runbooksvc

import (
	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runbook"
)

// Wire DTOs are plain, JSON/Wails-friendly shapes for crossing the frontend
// boundary. They are defined here (not in package main) so the mapping to/from
// the engine types is unit-testable without the GUI runtime.

// InputDTO is a typed runbook parameter definition.
type InputDTO struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Label       string   `json:"label,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// StepDTO is a single runbook step.
type StepDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Kind         string   `json:"kind"`
	DependsOn    []string `json:"dependsOn,omitempty"`
	ConnectionID string   `json:"connectionId,omitempty"`
	SQL          string   `json:"sql,omitempty"`
	Channel      string   `json:"channel,omitempty"`
	Message      string   `json:"message,omitempty"`
}

// DefinitionDTO is the wire form of a runbook definition.
type DefinitionDTO struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Inputs      []InputDTO `json:"inputs,omitempty"`
	Steps       []StepDTO  `json:"steps,omitempty"`
}

// ToRunbook converts a wire definition into the engine type.
func (d DefinitionDTO) ToRunbook() runbook.Runbook {
	rb := runbook.Runbook{ID: d.ID, Name: d.Name, Description: d.Description}
	for _, in := range d.Inputs {
		rb.Inputs = append(rb.Inputs, params.Definition{
			Name:        in.Name,
			Type:        params.Type(in.Type),
			Label:       in.Label,
			Description: in.Description,
			Required:    in.Required,
			Default:     in.Default,
			Options:     in.Options,
		})
	}
	for _, st := range d.Steps {
		rb.Steps = append(rb.Steps, runbook.Step{
			ID:           st.ID,
			Name:         st.Name,
			Kind:         runbook.StepKind(st.Kind),
			DependsOn:    st.DependsOn,
			ConnectionID: st.ConnectionID,
			SQL:          st.SQL,
			Channel:      st.Channel,
			Message:      st.Message,
		})
	}
	return rb
}

// DefinitionFromRunbook converts an engine runbook into its wire form.
func DefinitionFromRunbook(rb *runbook.Runbook) DefinitionDTO {
	d := DefinitionDTO{ID: rb.ID, Name: rb.Name, Description: rb.Description}
	for _, in := range rb.Inputs {
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
	for _, st := range rb.Steps {
		d.Steps = append(d.Steps, StepDTO{
			ID:           st.ID,
			Name:         st.Name,
			Kind:         string(st.Kind),
			DependsOn:    st.DependsOn,
			ConnectionID: st.ConnectionID,
			SQL:          st.SQL,
			Channel:      st.Channel,
			Message:      st.Message,
		})
	}
	return d
}

// OutcomeDTO is the wire form of a single step outcome.
type OutcomeDTO struct {
	StepID       string `json:"stepId"`
	Name         string `json:"name,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Status       string `json:"status"`
	SQL          string `json:"sql,omitempty"`
	RowsAffected int64  `json:"rowsAffected,omitempty"`
	Message      string `json:"message,omitempty"`
	Notified     bool   `json:"notified,omitempty"`
	Planned      bool   `json:"planned,omitempty"`
	Error        string `json:"error,omitempty"`
	Skipped      string `json:"skipped,omitempty"`
}

// RunResultDTO is the wire form of a run result, ordered by step definition.
type RunResultDTO struct {
	Failed   bool         `json:"failed"`
	DryRun   bool         `json:"dryRun"`
	Outcomes []OutcomeDTO `json:"outcomes"`
}

// ResultToDTO flattens a RunResult into ordered wire outcomes.
func ResultToDTO(res *runbook.RunResult) RunResultDTO {
	out := RunResultDTO{Failed: res.Failed, DryRun: res.DryRun}
	for _, id := range res.Order {
		oc := res.Outcomes[id]
		out.Outcomes = append(out.Outcomes, OutcomeDTO{
			StepID:       oc.StepID,
			Name:         oc.Name,
			Kind:         string(oc.Kind),
			Status:       string(oc.Status),
			SQL:          oc.SQL,
			RowsAffected: oc.RowsAffected,
			Message:      oc.Message,
			Notified:     oc.Notified,
			Planned:      oc.Planned,
			Error:        oc.Error,
			Skipped:      oc.Skipped,
		})
	}
	return out
}
