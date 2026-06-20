package reportbind

import (
	"testing"
	"time"

	"github.com/jbeck018/howlerops/pkg/storage"
)

// fixture builds a minimal report exposing the given filter fields, and a
// component whose query references the given top-level filter keys.
func fixture(fields []storage.ReportFilterField, topLevel []string) (*storage.Report, *storage.ReportComponent) {
	report := &storage.Report{Filter: storage.ReportFilterDefinition{Fields: fields}}
	comp := &storage.ReportComponent{
		ID:    "c1",
		Query: storage.ReportQueryConfig{TopLevelFilter: topLevel},
	}
	return report, comp
}

func TestApply_TypedRendering(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{
			{Key: "status", Type: "string"},
			{Key: "active", Type: "boolean"},
			{Key: "limit", Type: "number"},
		},
		[]string{"status", "active", "limit"},
	)
	sql := "SELECT * FROM t WHERE s={{status}} AND a={{active}} AND n={{limit}}"
	got, err := Apply(sql, report, comp, map[string]interface{}{
		"status": "open",
		"active": true,
		"limit":  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT * FROM t WHERE s='open' AND a=TRUE AND n=10"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApply_EscapesInjection(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "name", Type: "string"}},
		[]string{"name"},
	)
	got, err := Apply("WHERE name = {{name}}", report, comp, map[string]interface{}{
		"name": "x'; DROP TABLE users;--",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE name = 'x''; DROP TABLE users;--'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestApply_DefaultFromField(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "status", Type: "string", DefaultValue: "active"}},
		[]string{"status"},
	)
	got, err := Apply("WHERE s={{status}}", report, comp, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "WHERE s='active'" {
		t.Errorf("got %q", got)
	}
}

func TestApply_RequiredMissingErrors(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "owner", Type: "string", Required: true}},
		[]string{"owner"},
	)
	if _, err := Apply("WHERE o={{owner}}", report, comp, map[string]interface{}{}); err == nil {
		t.Error("expected error for missing required filter")
	}
}

func TestApply_OptionalMissingRendersNull(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "note", Type: "string"}},
		[]string{"note"},
	)
	got, err := Apply("WHERE n={{note}}", report, comp, map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "WHERE n=NULL" {
		t.Errorf("got %q, want NULL render", got)
	}
}

func TestApply_ListForInClause(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "ids", Type: "multiselect"}},
		[]string{"ids"},
	)
	got, err := Apply("WHERE id IN ({{ids}})", report, comp, map[string]interface{}{
		"ids": []interface{}{1, 2, 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "WHERE id IN (1, 2, 3)" {
		t.Errorf("got %q", got)
	}
}

func TestApply_LeavesNonFilterTokens(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "a", Type: "string"}},
		[]string{"a"},
	)
	got, err := Apply("{{a}} -- {{context}}", report, comp, map[string]interface{}{"a": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "'x' -- {{context}}" {
		t.Errorf("got %q", got)
	}
}

func TestApply_NoFiltersIsNoop(t *testing.T) {
	report, comp := fixture(nil, nil)
	sql := "SELECT 1"
	got, err := Apply(sql, report, comp, map[string]interface{}{"x": 1})
	if err != nil {
		t.Fatal(err)
	}
	if got != sql {
		t.Errorf("expected no-op, got %q", got)
	}
}

func TestApply_TimestampValue(t *testing.T) {
	report, comp := fixture(
		[]storage.ReportFilterField{{Key: "since", Type: "date"}},
		[]string{"since"},
	)
	got, err := Apply("WHERE ts > {{since}}", report, comp, map[string]interface{}{
		"since": time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "WHERE ts > '2026-01-15 10:30:00'" {
		t.Errorf("got %q", got)
	}
}
