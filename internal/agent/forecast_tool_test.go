package agent

import (
	"strings"
	"testing"
	"time"
)

// linearResult builds an SQLResult with a daily date column and a linearly
// increasing value column.
func linearResult(days int) *SQLResult {
	rows := make([]map[string]interface{}, 0, days)
	for i := 0; i < days; i++ {
		// 2026-01-01 + i days
		day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		rows = append(rows, map[string]interface{}{
			"day":     day.Format("2006-01-02"),
			"revenue": float64(100 + 5*i),
		})
	}
	return &SQLResult{
		Columns:  []string{"day", "revenue"},
		Rows:     rows,
		RowCount: int64(days),
	}
}

func TestForecastFromResult_Explicit(t *testing.T) {
	res := linearResult(20)
	out := forecastFromResult(res, "day", "revenue", 3, 0)
	if !strings.Contains(out, "Predictions") {
		t.Errorf("missing predictions section: %q", out)
	}
	// 3 prediction lines should be present (dates in 2026).
	if n := strings.Count(out, "2026-"); n < 3 {
		t.Errorf("expected at least 3 dated lines, got %d in:\n%s", n, out)
	}
}

func TestForecastFromResult_AutoDetectColumns(t *testing.T) {
	res := linearResult(15)
	out := forecastFromResult(res, "", "", 2, 0)
	if !strings.Contains(out, "revenue") || !strings.Contains(out, "day") {
		t.Errorf("expected detected column names in output: %q", out)
	}
}

func TestForecastFromResult_DefaultsHorizon(t *testing.T) {
	res := linearResult(15)
	out := forecastFromResult(res, "day", "revenue", 0, 0) // horizon 0 -> default 7
	if n := strings.Count(out, "2026-"); n < 7 {
		t.Errorf("expected default horizon of 7 predictions, got %d", n)
	}
}

func TestForecastFromResult_DetectFailure(t *testing.T) {
	res := &SQLResult{
		Columns: []string{"label", "note"},
		Rows: []map[string]interface{}{
			{"label": "a", "note": "x"},
		},
	}
	out := forecastFromResult(res, "", "", 3, 0)
	if !strings.Contains(out, "time_column") {
		t.Errorf("expected guidance to set columns, got %q", out)
	}
}

func TestForecastFromResult_NilResult(t *testing.T) {
	out := forecastFromResult(nil, "", "", 3, 0)
	if !strings.Contains(out, "No query result") {
		t.Errorf("expected nil-result message, got %q", out)
	}
}

func TestForecastFromResult_AnomalySection(t *testing.T) {
	res := linearResult(30)
	// Inject a spike.
	res.Rows[15]["revenue"] = float64(99999)
	out := forecastFromResult(res, "day", "revenue", 3, 0)
	if !strings.Contains(out, "Anomalies") {
		t.Errorf("expected anomalies section for spiked data:\n%s", out)
	}
}

func TestNumFormat(t *testing.T) {
	cases := map[float64]string{
		100:      "100",
		100.5:    "100.50",
		-3.14159: "-3.14",
	}
	for in, want := range cases {
		if got := num(in); got != want {
			t.Errorf("num(%v) = %q, want %q", in, got, want)
		}
	}
}
