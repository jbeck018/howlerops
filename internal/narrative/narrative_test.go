package narrative

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func sampleRows() (cols []string, rows []map[string]interface{}) {
	cols = []string{"day", "region", "revenue", "ssn"}
	rows = []map[string]interface{}{
		{"day": "2026-01-01", "region": "west", "revenue": 100.0, "ssn": "111-11-1111"},
		{"day": "2026-01-02", "region": "east", "revenue": 200.0, "ssn": "222-22-2222"},
		{"day": "2026-01-03", "region": "west", "revenue": 150.0, "ssn": "333-33-3333"},
		{"day": "2026-01-04", "region": "west", "revenue": nil, "ssn": "444-44-4444"},
	}
	return cols, rows
}

func TestSummarize_Kinds(t *testing.T) {
	cols, rows := sampleRows()
	s := Summarize(cols, rows)
	if s.RowCount != 4 {
		t.Fatalf("RowCount = %d, want 4", s.RowCount)
	}
	byName := map[string]ColumnSummary{}
	for _, c := range s.Columns {
		byName[c.Name] = c
	}

	if byName["day"].Kind != KindTemporal {
		t.Errorf("day kind = %s, want temporal", byName["day"].Kind)
	}
	if byName["region"].Kind != KindCategorical {
		t.Errorf("region kind = %s, want categorical", byName["region"].Kind)
	}
	rev := byName["revenue"]
	if rev.Kind != KindNumeric {
		t.Fatalf("revenue kind = %s, want numeric", rev.Kind)
	}
	if rev.Min != 100 || rev.Max != 200 || rev.Sum != 450 {
		t.Errorf("revenue stats off: min=%v max=%v sum=%v", rev.Min, rev.Max, rev.Sum)
	}
	if rev.Nulls != 1 {
		t.Errorf("revenue nulls = %d, want 1", rev.Nulls)
	}
	if rev.Mean != 150 {
		t.Errorf("revenue mean = %v, want 150", rev.Mean)
	}
	if byName["region"].Distinct != 2 {
		t.Errorf("region distinct = %d, want 2", byName["region"].Distinct)
	}
	if len(byName["region"].Top) == 0 || byName["region"].Top[0].Value != "west" {
		t.Errorf("region top should lead with west: %+v", byName["region"].Top)
	}
}

func TestBuildPrompt_ContainsAggregatesNotRawRows(t *testing.T) {
	cols, rows := sampleRows()
	in := BriefInput{Title: "Sales", Summary: Summarize(cols, rows)}
	prompt := BuildPrompt(in)

	// Aggregates present.
	for _, want := range []string{"Sales", "mean=150", "min=100", "max=200", "2 distinct"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	// Raw row-level PII must NOT appear — only aggregates leave Summarize.
	// ssn is high-cardinality, so each value appears once and is not in the
	// top-K categorical sample; none of the individual SSNs should leak.
	for _, ssn := range []string{"111-11-1111", "222-22-2222", "333-33-3333"} {
		if strings.Contains(prompt, ssn) {
			t.Errorf("prompt leaked raw value %q:\n%s", ssn, prompt)
		}
	}
}

// TestBuildPrompt_NoPIILeakWhenLowCardinality guards the case the cardinality
// heuristic alone misses: a small result set where a PII value repeats, so it is
// genuinely low-cardinality yet must still never be sampled into the prompt.
func TestBuildPrompt_NoPIILeakWhenLowCardinality(t *testing.T) {
	cols := []string{"email", "ssn", "region"}
	rows := []map[string]interface{}{
		{"email": "alice@example.com", "ssn": "111-11-1111", "region": "west"},
		{"email": "alice@example.com", "ssn": "111-11-1111", "region": "east"},
		{"email": "bob@example.com", "ssn": "222-22-2222", "region": "west"},
	}
	prompt := BuildPrompt(BriefInput{Title: "Customers", Summary: Summarize(cols, rows)})
	for _, leak := range []string{"alice@example.com", "bob@example.com", "111-11-1111", "222-22-2222"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("prompt leaked PII %q despite low cardinality:\n%s", leak, prompt)
		}
	}
	// Genuine categoricals are still summarised.
	if !strings.Contains(prompt, "west") {
		t.Errorf("expected non-PII categorical to still be sampled:\n%s", prompt)
	}
}

// TestBuildPrompt_NoLeakDotlessEmail guards the gap the earlier "contains @ and
// ." email heuristic missed: intranet/local addresses without a dot in the
// domain (e.g. alice@localhost) are still PII and must never be sampled, even
// when they repeat and are thus low-cardinality.
func TestBuildPrompt_NoLeakDotlessEmail(t *testing.T) {
	cols := []string{"contact", "region"}
	rows := []map[string]interface{}{
		{"contact": "alice@localhost", "region": "west"},
		{"contact": "alice@localhost", "region": "east"},
		{"contact": "bob@intranet", "region": "west"},
	}
	prompt := BuildPrompt(BriefInput{Title: "Contacts", Summary: Summarize(cols, rows)})
	for _, leak := range []string{"alice@localhost", "bob@intranet"} {
		if strings.Contains(prompt, leak) {
			t.Errorf("prompt leaked dotless email %q:\n%s", leak, prompt)
		}
	}
	if !strings.Contains(prompt, "west") {
		t.Errorf("expected non-PII categorical to still be sampled:\n%s", prompt)
	}
}

func TestBrief_UsesChatFuncAndSystemPrompt(t *testing.T) {
	var gotSystem, gotPrompt string
	g := New(func(_ context.Context, system, prompt string) (string, error) {
		gotSystem, gotPrompt = system, prompt
		return "  Revenue is up.  ", nil
	})
	cols, rows := sampleRows()
	out, err := g.Brief(context.Background(), BriefInput{Title: "Sales", Summary: Summarize(cols, rows)})
	if err != nil {
		t.Fatal(err)
	}
	if out != "Revenue is up." {
		t.Errorf("output not trimmed/returned: %q", out)
	}
	if !strings.Contains(gotSystem, "insight brief") {
		t.Errorf("system prompt not passed: %q", gotSystem)
	}
	if !strings.Contains(gotPrompt, "Sales") {
		t.Errorf("user prompt not passed: %q", gotPrompt)
	}
}

func TestBrief_NoChatFunc(t *testing.T) {
	g := New(nil)
	if _, err := g.Brief(context.Background(), BriefInput{}); err == nil {
		t.Error("expected error when chat func is nil")
	}
}

func TestBrief_PropagatesError(t *testing.T) {
	g := New(func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("provider down")
	})
	_, err := g.Brief(context.Background(), BriefInput{})
	if err == nil || !strings.Contains(err.Error(), "provider down") {
		t.Errorf("expected wrapped provider error, got %v", err)
	}
}

func TestBuildPrompt_ForecastAndAnomalies(t *testing.T) {
	in := BriefInput{
		Title:   "Revenue",
		Summary: DataSummary{RowCount: 30},
		Forecast: &ForecastNote{
			Method: "holt", Horizon: 7, First: 210, Last: 260,
			LowerLast: 240, UpperLast: 280, MAPEPercent: 4.2,
		},
		Anomalies: []AnomalyNote{{When: "2026-01-15", Observed: 9999, Expected: 210}},
	}
	prompt := BuildPrompt(in)
	for _, want := range []string{"Forecast (holt", "horizon 7", "fit error ~4.2%", "Anomalies (1)", "2026-01-15"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
