package alerting

import (
	"strings"
	"testing"
	"time"

	"github.com/jbeck018/howlerops/internal/forecast"
)

func series(values ...float64) forecast.Series {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := make(forecast.Series, len(values))
	for i, v := range values {
		s[i] = forecast.Point{Time: start.AddDate(0, 0, i), Value: v}
	}
	return s
}

func TestEvaluate_EmptySeries(t *testing.T) {
	if _, err := Evaluate(nil, Rule{Threshold: &ThresholdRule{}}); err != forecast.ErrNoData {
		t.Errorf("expected ErrNoData, got %v", err)
	}
}

func TestEvaluate_NoConditionSet(t *testing.T) {
	if _, err := Evaluate(series(1, 2, 3), Rule{Name: "x"}); err == nil {
		t.Error("expected error when no condition is set")
	}
}

func TestEvaluate_Threshold(t *testing.T) {
	s := series(10, 20, 30, 95)
	fired, err := Evaluate(s, Rule{Name: "high", Threshold: &ThresholdRule{Comparator: GT, Value: 90}})
	if err != nil {
		t.Fatal(err)
	}
	if !fired.Fired || fired.Kind != KindThreshold || fired.Value != 95 {
		t.Errorf("expected threshold fire at 95: %+v", fired)
	}
	if !strings.Contains(fired.Message, "above") {
		t.Errorf("message should describe the comparator: %q", fired.Message)
	}

	notFired, _ := Evaluate(s, Rule{Name: "high", Threshold: &ThresholdRule{Comparator: GT, Value: 100}})
	if notFired.Fired {
		t.Errorf("should not fire below threshold: %+v", notFired)
	}
}

func TestEvaluate_ThresholdComparators(t *testing.T) {
	s := series(50)
	cases := []struct {
		c    Comparator
		v    float64
		want bool
	}{
		{GT, 40, true}, {GT, 50, false},
		{GTE, 50, true}, {GTE, 51, false},
		{LT, 60, true}, {LT, 50, false},
		{LTE, 50, true}, {LTE, 49, false},
	}
	for _, c := range cases {
		ev, _ := Evaluate(s, Rule{Name: "r", Threshold: &ThresholdRule{Comparator: c.c, Value: c.v}})
		if ev.Fired != c.want {
			t.Errorf("%s %v: fired=%v want %v", c.c, c.v, ev.Fired, c.want)
		}
	}
}

func TestEvaluate_Anomaly(t *testing.T) {
	// Flat-ish series with a spike at the most recent point.
	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = 50
	}
	vals[39] = 500 // recent spike
	ev, err := Evaluate(series(vals...), Rule{Name: "spike", Anomaly: &AnomalyRule{Lookback: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Fired || ev.Kind != KindAnomaly {
		t.Fatalf("expected anomaly fire: %+v", ev)
	}
	if ev.Value != 500 {
		t.Errorf("anomaly value = %v, want 500", ev.Value)
	}
}

func TestEvaluate_AnomalyOutsideLookbackDoesNotFire(t *testing.T) {
	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = 50
	}
	vals[5] = 500 // old spike, outside a small lookback
	ev, err := Evaluate(series(vals...), Rule{Name: "spike", Anomaly: &AnomalyRule{Lookback: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Fired {
		t.Errorf("anomaly outside lookback should not fire: %+v", ev)
	}
}

func TestEvaluate_CleanSeriesNoAnomaly(t *testing.T) {
	vals := make([]float64, 30)
	for i := range vals {
		vals[i] = 10 + float64(i)
	}
	ev, err := Evaluate(series(vals...), Rule{Name: "x", Anomaly: &AnomalyRule{}})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Fired {
		t.Errorf("clean trend should not fire anomaly: %+v", ev)
	}
}

func TestEvaluate_ThresholdSortsUnorderedSeries(t *testing.T) {
	// The genuine latest point (by time) is 95; it is supplied out of order.
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := forecast.Series{
		{Time: start.AddDate(0, 0, 3), Value: 95}, // latest in time
		{Time: start.AddDate(0, 0, 0), Value: 10},
		{Time: start.AddDate(0, 0, 1), Value: 20},
		{Time: start.AddDate(0, 0, 2), Value: 30},
	}
	ev, err := Evaluate(s, Rule{Name: "high", Threshold: &ThresholdRule{Comparator: GT, Value: 90}})
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Fired || ev.Value != 95 {
		t.Fatalf("expected fire on the time-latest value 95, got %+v", ev)
	}
	if !ev.At.Equal(start.AddDate(0, 0, 3)) {
		t.Errorf("triggering time = %v, want the latest timestamp", ev.At)
	}
	// The caller's slice must not be mutated by the defensive sort.
	if s[0].Value != 95 {
		t.Errorf("Evaluate mutated caller's series order: %+v", s)
	}
}

func TestEvaluate_ForecastThresholdCrossing(t *testing.T) {
	// Rising trend: forecast should exceed a threshold above the current max.
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = 100 + 5*float64(i) // ends at 195
	}
	ev, err := Evaluate(series(vals...), Rule{
		Name:     "growth",
		Forecast: &ForecastRule{Horizon: 10, Comparator: GT, Value: 220},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Fired || ev.Kind != KindForecast {
		t.Fatalf("expected forecast threshold fire: %+v", ev)
	}
	if ev.Value <= 220 {
		t.Errorf("firing value should exceed threshold, got %v", ev.Value)
	}
	if ev.At.IsZero() {
		t.Error("expected a crossing time")
	}
}

func TestEvaluate_ForecastNoCrossing(t *testing.T) {
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = 100 // flat
	}
	ev, err := Evaluate(series(vals...), Rule{
		Name:     "growth",
		Forecast: &ForecastRule{Horizon: 5, Comparator: GT, Value: 500},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Fired {
		t.Errorf("flat series should not cross a high threshold: %+v", ev)
	}
}
