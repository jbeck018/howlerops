package insight

import (
	"context"
	"strings"
	"testing"
	"time"
)

// trendRows builds a daily series with a linear upward trend and one spike.
func trendRows(days int, spikeAt int) (cols []string, rows []map[string]interface{}) {
	cols = []string{"day", "revenue"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < days; i++ {
		v := float64(100 + 5*i)
		if i == spikeAt {
			v = 99999
		}
		rows = append(rows, map[string]interface{}{
			"day":     base.AddDate(0, 0, i).Format("2006-01-02"),
			"revenue": v,
		})
	}
	return cols, rows
}

// capturingChat records the prompt and returns a canned brief.
func capturingChat(captured *string) func(context.Context, string, string) (string, error) {
	return func(_ context.Context, _, prompt string) (string, error) {
		if captured != nil {
			*captured = prompt
		}
		return "Revenue is trending up.", nil
	}
}

func TestGenerate_WithForecast(t *testing.T) {
	cols, rows := trendRows(30, -1)
	var prompt string
	res, err := Generate(context.Background(), capturingChat(&prompt), cols, rows, Options{
		Title:    "Sales",
		Forecast: true,
		Horizon:  5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Brief != "Revenue is trending up." {
		t.Errorf("brief = %q", res.Brief)
	}
	if res.ForecastErr != "" {
		t.Errorf("unexpected forecast error: %s", res.ForecastErr)
	}
	if res.Forecast == nil || len(res.Forecast.Predictions) != 5 {
		t.Fatalf("expected 5 predictions, got %+v", res.Forecast)
	}
	// The forecast context must be reflected in the prompt sent to the model.
	if !strings.Contains(prompt, "Forecast (") {
		t.Errorf("prompt missing forecast context:\n%s", prompt)
	}
}

func TestGenerate_ForecastAnomalySurfaced(t *testing.T) {
	cols, rows := trendRows(40, 20) // spike at index 20
	res, err := Generate(context.Background(), capturingChat(nil), cols, rows, Options{
		Title:    "Sales",
		Forecast: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Anomalies) == 0 {
		t.Error("expected the injected spike to be flagged as an anomaly")
	}
}

func TestGenerate_NoForecastRequested(t *testing.T) {
	cols, rows := trendRows(10, -1)
	res, err := Generate(context.Background(), capturingChat(nil), cols, rows, Options{Title: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Forecast != nil {
		t.Error("forecast should be nil when not requested")
	}
	if res.Summary.RowCount != 10 {
		t.Errorf("summary row count = %d, want 10", res.Summary.RowCount)
	}
}

func TestGenerate_ForecastNotApplicableIsNonFatal(t *testing.T) {
	// No detectable time/value columns -> forecast skipped, brief still made.
	cols := []string{"label", "note"}
	rows := []map[string]interface{}{{"label": "a", "note": "x"}, {"label": "b", "note": "y"}}
	res, err := Generate(context.Background(), capturingChat(nil), cols, rows, Options{Forecast: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ForecastErr == "" {
		t.Error("expected a non-fatal forecast error explanation")
	}
	if res.Brief == "" {
		t.Error("brief should still be generated despite forecast being skipped")
	}
}

func TestGenerate_PropagatesChatError(t *testing.T) {
	cols, rows := trendRows(10, -1)
	_, err := Generate(context.Background(), func(_ context.Context, _, _ string) (string, error) {
		return "", context.DeadlineExceeded
	}, cols, rows, Options{})
	if err == nil {
		t.Error("expected chat error to propagate")
	}
}
