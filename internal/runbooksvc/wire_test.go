package runbooksvc

import (
	"testing"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/internal/runner"
)

func TestDefinitionRoundTrip(t *testing.T) {
	d := DefinitionDTO{
		ID:          "rb1",
		Name:        "Cleanup",
		Description: "desc",
		Inputs: []InputDTO{
			{Name: "status", Type: "string", Required: true, Default: "active", Options: []string{"active", "stale"}},
			{Name: "limit", Type: "integer", Default: float64(10)},
		},
		Steps: []StepDTO{
			{ID: "q", Kind: "query", ConnectionID: "c", SQL: "SELECT {{status}}"},
			{ID: "n", Kind: "notify", Channel: "ops", Message: "done {{status}}", DependsOn: []string{"q"}},
		},
	}

	rb := d.ToRunbook()
	if rb.Name != "Cleanup" || len(rb.Inputs) != 2 || len(rb.Steps) != 2 {
		t.Fatalf("ToRunbook lost data: %+v", rb)
	}
	if rb.Inputs[0].Type != params.TypeString || !rb.Inputs[0].Required {
		t.Errorf("input 0 mismatch: %+v", rb.Inputs[0])
	}
	if rb.Steps[1].Kind != runbook.StepNotify || rb.Steps[1].DependsOn[0] != "q" {
		t.Errorf("step 1 mismatch: %+v", rb.Steps[1])
	}

	back := DefinitionFromRunbook(&rb)
	if back.Name != d.Name || len(back.Inputs) != 2 || len(back.Steps) != 2 {
		t.Fatalf("round trip lost data: %+v", back)
	}
	if back.Inputs[0].Type != "string" || back.Steps[0].Kind != "query" {
		t.Errorf("round-trip type/kind mismatch: %+v", back)
	}
}

func TestResultToDTO_PreservesOrder(t *testing.T) {
	res := &runbook.RunResult{
		Order:  []string{"a", "b", "c"},
		Failed: true,
		DryRun: false,
		Outcomes: map[string]runbook.StepOutcome{
			"a": {StepID: "a", Status: runner.StatusSuccess, Kind: runbook.StepQuery, SQL: "SELECT 1"},
			"b": {StepID: "b", Status: runner.StatusFailed, Error: "boom"},
			"c": {StepID: "c", Status: runner.StatusSkipped, Skipped: "dep failed"},
		},
	}
	dto := ResultToDTO(res)
	if !dto.Failed {
		t.Error("expected Failed")
	}
	if len(dto.Outcomes) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(dto.Outcomes))
	}
	if dto.Outcomes[0].StepID != "a" || dto.Outcomes[1].StepID != "b" || dto.Outcomes[2].StepID != "c" {
		t.Errorf("order not preserved: %+v", dto.Outcomes)
	}
	if dto.Outcomes[1].Error != "boom" || dto.Outcomes[2].Skipped != "dep failed" {
		t.Errorf("outcome details lost: %+v", dto.Outcomes)
	}
}
