package forecast

import (
	"math"
	"testing"
	"time"
)

// day builds a daily series starting at a fixed epoch.
func day(values ...float64) Series {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := make(Series, len(values))
	for i, v := range values {
		s[i] = Point{Time: start.AddDate(0, 0, i), Value: v}
	}
	return s
}

func TestForecast_EmptyAndBadInput(t *testing.T) {
	if _, err := Forecast(nil, Options{Horizon: 1}); err != ErrNoData {
		t.Fatalf("empty series: got %v, want ErrNoData", err)
	}
	if _, err := Forecast(day(1, 2, 3), Options{Horizon: 0}); err != ErrBadHorizon {
		t.Fatalf("zero horizon: got %v, want ErrBadHorizon", err)
	}
}

func TestForecast_LinearTrendHolt(t *testing.T) {
	// A clean linear ramp; Holt should extrapolate close to the line.
	vals := make([]float64, 20)
	for i := range vals {
		vals[i] = 10 + 2*float64(i)
	}
	res, err := Forecast(day(vals...), Options{Method: MethodHolt, Horizon: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Predictions) != 3 {
		t.Fatalf("expected 3 predictions, got %d", len(res.Predictions))
	}
	// Next true values are 50, 52, 54.
	wants := []float64{50, 52, 54}
	for i, w := range wants {
		got := res.Predictions[i].Value
		if math.Abs(got-w) > 1.0 {
			t.Errorf("prediction %d: got %.3f, want ~%.1f", i, got, w)
		}
	}
	if res.RMSE > 1.0 {
		t.Errorf("expected tight fit on a line, RMSE=%.3f", res.RMSE)
	}
}

func TestForecast_ConfidenceIntervalsOrderedAndWidening(t *testing.T) {
	vals := make([]float64, 24)
	for i := range vals {
		vals[i] = 5 + float64(i) + math.Sin(float64(i)) // trend + wiggle for residuals
	}
	res, err := Forecast(day(vals...), Options{Method: MethodHolt, Horizon: 5, ConfidenceLevel: 0.95})
	if err != nil {
		t.Fatal(err)
	}
	prevWidth := -1.0
	for i, p := range res.Predictions {
		if p.Lower > p.Value || p.Upper < p.Value {
			t.Errorf("prediction %d interval not bracketing value: [%.2f, %.2f] v=%.2f", i, p.Lower, p.Upper, p.Value)
		}
		w := p.Upper - p.Lower
		if w < prevWidth-1e-9 {
			t.Errorf("interval width should be non-decreasing with horizon: step %d width %.3f < prev %.3f", i, w, prevWidth)
		}
		prevWidth = w
	}
}

func TestForecast_HoltWintersSeasonality(t *testing.T) {
	// 4 weeks of daily data with a clear weekly pattern + slight upward trend.
	pattern := []float64{10, 12, 14, 13, 16, 22, 20}
	var vals []float64
	for w := 0; w < 4; w++ {
		for _, p := range pattern {
			vals = append(vals, p+float64(w)) // +1 each week
		}
	}
	res, err := Forecast(day(vals...), Options{
		Method:       MethodHoltWinters,
		SeasonLength: 7,
		Horizon:      7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != MethodHoltWinters {
		t.Fatalf("method = %s, want holt_winters", res.Method)
	}
	// The weekend peak (index 5 in the pattern) should remain the max of the
	// next forecast week.
	maxIdx, maxVal := 0, math.Inf(-1)
	for i, p := range res.Predictions {
		if p.Value > maxVal {
			maxIdx, maxVal = i, p.Value
		}
	}
	if maxIdx != 5 {
		t.Errorf("expected weekend (idx 5) to be the forecast peak, got idx %d", maxIdx)
	}
}

func TestForecast_AutoSelectsByData(t *testing.T) {
	// Enough seasonal data => Holt-Winters.
	var seasonal []float64
	for w := 0; w < 3; w++ {
		seasonal = append(seasonal, 1, 5, 2, 8, 3, 9, 4)
	}
	res, err := Forecast(day(seasonal...), Options{SeasonLength: 7, Horizon: 7})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != MethodHoltWinters {
		t.Errorf("auto with season => got %s, want holt_winters", res.Method)
	}

	// Short series, no season => Holt or SES.
	res2, err := Forecast(day(1, 2, 3, 4), Options{Horizon: 2})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Method != MethodHolt {
		t.Errorf("auto short series => got %s, want holt", res2.Method)
	}
}

func TestForecast_GridSearchPicksParams(t *testing.T) {
	vals := make([]float64, 30)
	for i := range vals {
		vals[i] = 100 + 3*float64(i)
	}
	res, err := Forecast(day(vals...), Options{Method: MethodHolt, Horizon: 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Alpha < 0.1 || res.Alpha > 0.9 || res.Beta < 0.1 || res.Beta > 0.9 {
		t.Errorf("grid-searched params out of range: alpha=%.2f beta=%.2f", res.Alpha, res.Beta)
	}
}

func TestForecast_SeasonalNaiveBaseline(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 1, 2, 3, 4, 5, 6, 7}
	res, err := Forecast(day(vals...), Options{Method: MethodSeasonalNaive, SeasonLength: 7, Horizon: 7})
	if err != nil {
		t.Fatal(err)
	}
	want := []float64{1, 2, 3, 4, 5, 6, 7}
	for i, w := range want {
		if math.Abs(res.Predictions[i].Value-w) > 1e-9 {
			t.Errorf("seasonal naive pred %d: got %.3f want %.3f", i, res.Predictions[i].Value, w)
		}
	}
}

func TestForecast_InsufficientForHoltWinters(t *testing.T) {
	// Only one season of data; needs two.
	_, err := Forecast(day(1, 2, 3, 4, 5, 6, 7), Options{Method: MethodHoltWinters, SeasonLength: 7, Horizon: 1})
	if err != ErrInsufficient {
		t.Fatalf("got %v, want ErrInsufficient", err)
	}
}

func TestNormalQuantile_KnownValues(t *testing.T) {
	cases := []struct {
		p, want float64
	}{
		{0.5, 0.0},
		{0.975, 1.959964},
		{0.025, -1.959964},
		{0.95, 1.644854},
	}
	for _, c := range cases {
		got := normalQuantile(c.p)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("normalQuantile(%.4f) = %.6f, want %.6f", c.p, got, c.want)
		}
	}
}

func TestDetectAnomalies_Spike(t *testing.T) {
	// Flat series with one obvious spike.
	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = 50 + math.Sin(float64(i)/3) // small noise
	}
	vals[25] = 500 // spike
	anoms, err := DetectAnomalies(day(vals...), AnomalyOptions{Method: AnomalyMAD, Threshold: 3})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range anoms {
		if a.Index == 25 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected anomaly at index 25, got %+v", anoms)
	}
}

func TestDetectAnomalies_NoFalsePositivesOnClean(t *testing.T) {
	vals := make([]float64, 30)
	for i := range vals {
		vals[i] = 10 + 0.5*float64(i)
	}
	anoms, err := DetectAnomalies(day(vals...), AnomalyOptions{Method: AnomalyZScore, Threshold: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(anoms) != 0 {
		t.Errorf("clean linear series flagged %d anomalies: %+v", len(anoms), anoms)
	}
}

func TestDetectAnomalies_IQR(t *testing.T) {
	vals := make([]float64, 50)
	for i := range vals {
		vals[i] = 20
	}
	vals[10] = 200
	vals[40] = -150
	anoms, err := DetectAnomalies(day(vals...), AnomalyOptions{Method: AnomalyIQR, Threshold: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(anoms) < 2 {
		t.Errorf("expected at least 2 anomalies, got %d", len(anoms))
	}
}

func TestInferPeriod(t *testing.T) {
	p, err := inferPeriod(day(1, 2, 3, 4))
	if err != nil {
		t.Fatal(err)
	}
	if p != 24*time.Hour {
		t.Errorf("got %v, want 24h", p)
	}
}

func TestForecast_PredictionTimestamps(t *testing.T) {
	res, err := Forecast(day(1, 2, 3, 4, 5), Options{Method: MethodHolt, Horizon: 2})
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	if !res.Predictions[0].Time.Equal(wantFirst) {
		t.Errorf("first prediction time = %v, want %v", res.Predictions[0].Time, wantFirst)
	}
}
