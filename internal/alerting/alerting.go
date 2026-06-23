// Package alerting decides whether a time-series alert should fire. It is the
// forecasting-aware complement to pkg/alerts (which evaluates scalar threshold
// conditions on report components): here a rule can fire on a detected anomaly,
// a threshold crossing on the latest value, or a forecasted threshold crossing
// within a horizon. It composes internal/forecast and stays Wails-free and
// dependency-light so it is unit-testable and reusable by the report alert
// engine, runbooks, and scheduled checks.
package alerting

import (
	"fmt"
	"sort"
	"time"

	"github.com/jbeck018/howlerops/internal/forecast"
)

// Comparator compares an observed value against a rule's threshold.
type Comparator string

const (
	GT  Comparator = "gt"
	GTE Comparator = "gte"
	LT  Comparator = "lt"
	LTE Comparator = "lte"
)

// AnomalyRule fires when an anomaly is present among the most recent points.
type AnomalyRule struct {
	SeasonLength int     `json:"seasonLength,omitempty"`
	Lookback     int     `json:"lookback,omitempty"`
	MinScore     float64 `json:"minScore,omitempty"`
}

// ThresholdRule fires when the latest observed value crosses a threshold.
type ThresholdRule struct {
	Comparator Comparator `json:"comparator"`
	Value      float64    `json:"value"`
}

// ForecastRule fires when any forecasted point within the horizon crosses a
// threshold.
type ForecastRule struct {
	Horizon      int        `json:"horizon,omitempty"`
	SeasonLength int        `json:"seasonLength,omitempty"`
	Comparator   Comparator `json:"comparator"`
	Value        float64    `json:"value"`
}

// Rule is one alert definition. Exactly one of the rule kinds must be set.
type Rule struct {
	Name      string         `json:"name,omitempty"`
	Anomaly   *AnomalyRule   `json:"anomaly,omitempty"`
	Threshold *ThresholdRule `json:"threshold,omitempty"`
	Forecast  *ForecastRule  `json:"forecast,omitempty"`
}

// Kind identifies which rule fired.
type Kind string

const (
	KindAnomaly   Kind = "anomaly"
	KindThreshold Kind = "threshold"
	KindForecast  Kind = "forecast"
)

// Event is the result of evaluating a rule.
type Event struct {
	Fired   bool
	Rule    string
	Kind    Kind
	Message string
	Value   float64   // the value that triggered (observed or forecasted)
	At      time.Time // when the triggering point occurs
}

// Evaluate runs a rule against a series and reports whether it fires.
func Evaluate(series forecast.Series, rule Rule) (Event, error) {
	if len(series) == 0 {
		return Event{}, forecast.ErrNoData
	}
	// Sort defensively by time so "the latest value" (threshold) and lookback
	// windowing (anomaly) refer to genuinely recent points even when the caller
	// passes an unsorted series. forecast.Forecast/DetectAnomalies sort their own
	// copies internally, but evalThreshold reads series[len-1] directly, so the
	// order matters here. Copy first to avoid mutating the caller's slice.
	series = append(forecast.Series(nil), series...)
	sort.SliceStable(series, func(i, j int) bool { return series[i].Time.Before(series[j].Time) })
	switch {
	case rule.Anomaly != nil:
		return evalAnomaly(series, rule)
	case rule.Threshold != nil:
		return evalThreshold(series, rule)
	case rule.Forecast != nil:
		return evalForecast(series, rule)
	default:
		return Event{}, fmt.Errorf("alerting: rule %q has no condition set", rule.Name)
	}
}

func evalAnomaly(series forecast.Series, rule Rule) (Event, error) {
	r := rule.Anomaly
	anoms, err := forecast.DetectAnomalies(series, forecast.AnomalyOptions{
		SeasonLength: r.SeasonLength,
		Threshold:    r.MinScore,
	})
	if err != nil {
		return Event{}, err
	}
	cutoff := 0
	if r.Lookback > 0 && len(series) > r.Lookback {
		cutoff = len(series) - r.Lookback
	}
	// Report the most recent qualifying anomaly.
	for i := len(anoms) - 1; i >= 0; i-- {
		a := anoms[i]
		if a.Index < cutoff {
			continue
		}
		return Event{
			Fired: true, Rule: rule.Name, Kind: KindAnomaly, Value: a.Value, At: a.Time,
			Message: fmt.Sprintf("%s: anomaly on %s — observed %s vs expected %s (%.1f deviations)",
				rule.Name, a.Time.Format("2006-01-02"), num(a.Value), num(a.Expected), a.Score),
		}, nil
	}
	return Event{Fired: false, Rule: rule.Name, Kind: KindAnomaly}, nil
}

func evalThreshold(series forecast.Series, rule Rule) (Event, error) {
	last := series[len(series)-1]
	if !compare(last.Value, rule.Threshold.Comparator, rule.Threshold.Value) {
		return Event{Fired: false, Rule: rule.Name, Kind: KindThreshold}, nil
	}
	return Event{
		Fired: true, Rule: rule.Name, Kind: KindThreshold, Value: last.Value, At: last.Time,
		Message: fmt.Sprintf("%s: latest value %s is %s %s on %s",
			rule.Name, num(last.Value), describe(rule.Threshold.Comparator), num(rule.Threshold.Value), last.Time.Format("2006-01-02")),
	}, nil
}

func evalForecast(series forecast.Series, rule Rule) (Event, error) {
	r := rule.Forecast
	horizon := r.Horizon
	if horizon <= 0 {
		horizon = 7
	}
	fc, err := forecast.Forecast(series, forecast.Options{Horizon: horizon, SeasonLength: r.SeasonLength})
	if err != nil {
		return Event{}, err
	}
	for _, p := range fc.Predictions {
		if compare(p.Value, r.Comparator, r.Value) {
			return Event{
				Fired: true, Rule: rule.Name, Kind: KindForecast, Value: p.Value, At: p.Time,
				Message: fmt.Sprintf("%s: forecast projects %s %s %s by %s",
					rule.Name, num(p.Value), describe(r.Comparator), num(r.Value), p.Time.Format("2006-01-02")),
			}, nil
		}
	}
	return Event{Fired: false, Rule: rule.Name, Kind: KindForecast}, nil
}

func compare(v float64, c Comparator, threshold float64) bool {
	switch c {
	case GT:
		return v > threshold
	case GTE:
		return v >= threshold
	case LT:
		return v < threshold
	case LTE:
		return v <= threshold
	default:
		return false
	}
}

func describe(c Comparator) string {
	switch c {
	case GT:
		return "above"
	case GTE:
		return "at or above"
	case LT:
		return "below"
	case LTE:
		return "at or below"
	default:
		return string(c)
	}
}

func num(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%.2f", f)
}
