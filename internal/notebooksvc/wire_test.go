package notebooksvc

import (
	"testing"

	"github.com/jbeck018/howlerops/internal/params"
)

// A notebook list input's ElementType (plus Pattern/Min/Max) must survive the
// DTO round-trip, mirroring the runbook DTO; dropping ElementType reloads a
// list-of-integers as list-of-strings.
func TestDefinitionRoundTrip_ListElementTypeAndConstraints(t *testing.T) {
	d := DefinitionDTO{
		Name: "typed",
		Inputs: []InputDTO{
			{Name: "ids", Type: "list", ElementType: "integer"},
			{Name: "code", Type: "string", Pattern: "^[A-Z]+$", Min: params.Float(1), Max: params.Float(9)},
		},
	}
	nb := d.ToNotebook()
	if nb.Inputs[0].ElementType != params.TypeInteger {
		t.Errorf("ElementType lost in ToNotebook: %+v", nb.Inputs[0])
	}
	if nb.Inputs[1].Pattern != "^[A-Z]+$" || nb.Inputs[1].Min == nil || nb.Inputs[1].Max == nil {
		t.Errorf("Pattern/Min/Max lost in ToNotebook: %+v", nb.Inputs[1])
	}

	back := DefinitionFromNotebook(&nb)
	if back.Inputs[0].ElementType != "integer" {
		t.Errorf("ElementType lost in round-trip: %+v", back.Inputs[0])
	}
	if back.Inputs[1].Pattern != "^[A-Z]+$" || back.Inputs[1].Min == nil || back.Inputs[1].Max == nil {
		t.Errorf("Pattern/Min/Max lost in round-trip: %+v", back.Inputs[1])
	}
}
