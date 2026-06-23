package forecast

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSeriesFromRows_Basic(t *testing.T) {
	columns := []string{"day", "revenue"}
	rows := []map[string]interface{}{
		{"day": "2026-01-01", "revenue": 100.0},
		{"day": "2026-01-02", "revenue": 110.0},
		{"day": "2026-01-03", "revenue": json.Number("120")},
	}
	s, skipped, err := SeriesFromRows(columns, rows, "day", "revenue")
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0", skipped)
	}
	if len(s) != 3 {
		t.Fatalf("len = %d, want 3", len(s))
	}
	if s[0].Value != 100 || s[2].Value != 120 {
		t.Errorf("values = %v", s)
	}
	if !s[1].Time.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("time[1] = %v", s[1].Time)
	}
}

func TestSeriesFromRows_SkipsUnparseable(t *testing.T) {
	columns := []string{"ts", "v"}
	rows := []map[string]interface{}{
		{"ts": "2026-01-01", "v": 1.0},
		{"ts": "not-a-date", "v": 2.0},
		{"ts": "2026-01-02", "v": "bad"},
		{"ts": "2026-01-03", "v": "3.5"},
	}
	s, skipped, err := SeriesFromRows(columns, rows, "ts", "v")
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 2 {
		t.Errorf("skipped = %d, want 2", skipped)
	}
	if len(s) != 2 {
		t.Fatalf("len = %d, want 2", len(s))
	}
	if s[1].Value != 3.5 {
		t.Errorf("last value = %v, want 3.5", s[1].Value)
	}
}

func TestSeriesFromRows_MissingColumn(t *testing.T) {
	columns := []string{"day", "revenue"}
	rows := []map[string]interface{}{{"day": "2026-01-01", "revenue": 1.0}}
	if _, _, err := SeriesFromRows(columns, rows, "missing", "revenue"); err == nil {
		t.Error("expected error for missing time column")
	}
	if _, _, err := SeriesFromRows(columns, rows, "day", "missing"); err == nil {
		t.Error("expected error for missing value column")
	}
}

func TestSeriesFromRows_NoUsablePoints(t *testing.T) {
	columns := []string{"ts", "v"}
	rows := []map[string]interface{}{
		{"ts": "bad", "v": "bad"},
	}
	if _, _, err := SeriesFromRows(columns, rows, "ts", "v"); err == nil {
		t.Error("expected error when no points are usable")
	}
}

func TestSeriesFromRows_UnixAndRFC3339(t *testing.T) {
	columns := []string{"ts", "v"}
	rows := []map[string]interface{}{
		{"ts": int64(1767225600), "v": 5.0},           // 2026-01-01T00:00:00Z
		{"ts": "2026-01-02T12:00:00Z", "v": int64(6)}, // RFC3339 + int value
	}
	s, _, err := SeriesFromRows(columns, rows, "ts", "v")
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 2 {
		t.Fatalf("len = %d, want 2", len(s))
	}
	if s[0].Value != 5 || s[1].Value != 6 {
		t.Errorf("values = %v", s)
	}
}

func TestDetectColumns(t *testing.T) {
	columns := []string{"label", "day", "amount"}
	rows := []map[string]interface{}{
		{"label": "x", "day": "2026-01-01", "amount": 10.0},
	}
	tc, vc, ok := DetectColumns(columns, rows)
	if !ok {
		t.Fatal("expected detection to succeed")
	}
	if tc != "day" {
		t.Errorf("timeCol = %q, want day", tc)
	}
	if vc != "amount" {
		t.Errorf("valueCol = %q, want amount", vc)
	}
}

func TestDetectColumns_NoTime(t *testing.T) {
	columns := []string{"a", "b"}
	rows := []map[string]interface{}{{"a": "foo", "b": "bar"}}
	if _, _, ok := DetectColumns(columns, rows); ok {
		t.Error("expected detection to fail with no time/value columns")
	}
}

// End-to-end: rows -> series -> forecast.
func TestSeriesFromRows_FeedsForecast(t *testing.T) {
	columns := []string{"day", "v"}
	var rows []map[string]interface{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 14; i++ {
		rows = append(rows, map[string]interface{}{
			"day": base.AddDate(0, 0, i).Format("2006-01-02"),
			"v":   float64(10 + i),
		})
	}
	s, _, err := SeriesFromRows(columns, rows, "day", "v")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Forecast(s, Options{Method: MethodHolt, Horizon: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Predictions) != 3 {
		t.Fatalf("expected 3 predictions, got %d", len(res.Predictions))
	}
	// Linear ramp continuing: next value ~24.
	if got := res.Predictions[0].Value; got < 22 || got > 26 {
		t.Errorf("first prediction = %.2f, want ~24", got)
	}
}
