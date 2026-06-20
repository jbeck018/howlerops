// Package insight composes the forecasting, anomaly, and narrative services
// into a single "Auto Insight Brief" over a query/report result set. It is the
// orchestration the host calls (via a thin Wails binding) to produce the Phase
// 1 hero feature; keeping it Wails-free makes the whole pipeline unit-testable.
package insight

import (
	"context"

	"github.com/jbeck018/howlerops/internal/forecast"
	"github.com/jbeck018/howlerops/internal/narrative"
)

// Options configures brief generation.
type Options struct {
	Title    string // report/component title
	Question string // optional user focus passed to the narrative

	// Forecasting. When Forecast is true, the brief includes a projection. The
	// time/value columns are auto-detected when left empty.
	Forecast    bool
	TimeColumn  string
	ValueColumn string
	Horizon     int // future periods; defaults to 7 when forecasting
	Season      int // periods per season; 0 for none

	// MaxAnomalies caps anomalies surfaced in the brief (default 5).
	MaxAnomalies int
}

// Result is the structured output: the prose plus the structured artifacts the
// UI can render (forecast points, anomalies, and the aggregate summary).
type Result struct {
	Brief       string
	Summary     narrative.DataSummary
	Forecast    *forecast.Result   // nil when not requested or not applicable
	Anomalies   []forecast.Anomaly // nil when none/!applicable
	ForecastErr string             // why a requested forecast was skipped (non-fatal)
}

// Generate builds an Insight Brief for a result set. chat is the user's
// configured provider (see narrative.ChatFunc). A requested-but-impossible
// forecast (e.g. no detectable time column) is reported in Result.ForecastErr
// rather than failing the whole brief.
func Generate(ctx context.Context, chat narrative.ChatFunc, columns []string, rows []map[string]interface{}, opt Options) (*Result, error) {
	res := &Result{Summary: narrative.Summarize(columns, rows)}

	in := narrative.BriefInput{
		Title:    opt.Title,
		Question: opt.Question,
		Summary:  res.Summary,
	}

	if opt.Forecast {
		fc, anomalies, note, ferr := runForecast(columns, rows, opt)
		if ferr != "" {
			res.ForecastErr = ferr
		} else {
			res.Forecast = fc
			res.Anomalies = anomalies
			in.Forecast = note
			in.Anomalies = anomalyNotes(anomalies, maxAnomalies(opt))
		}
	}

	brief, err := narrative.New(chat).Brief(ctx, in)
	if err != nil {
		return res, err
	}
	res.Brief = brief
	return res, nil
}

// runForecast fits a forecast + anomalies and prepares the narrative note. It
// returns a non-empty reason string (instead of an error) when forecasting is
// not applicable, so the brief can proceed without it.
func runForecast(columns []string, rows []map[string]interface{}, opt Options) (*forecast.Result, []forecast.Anomaly, *narrative.ForecastNote, string) {
	timeCol, valueCol := opt.TimeColumn, opt.ValueColumn
	if timeCol == "" || valueCol == "" {
		dtc, dvc, ok := forecast.DetectColumns(columns, rows)
		if timeCol == "" {
			timeCol = dtc
		}
		if valueCol == "" {
			valueCol = dvc
		}
		if !ok && (timeCol == "" || valueCol == "") {
			return nil, nil, nil, "no time/value columns detected for forecasting"
		}
	}

	series, _, err := forecast.SeriesFromRows(columns, rows, timeCol, valueCol)
	if err != nil {
		return nil, nil, nil, "could not build a series: " + err.Error()
	}

	horizon := opt.Horizon
	if horizon <= 0 {
		horizon = 7
	}
	fc, err := forecast.Forecast(series, forecast.Options{Horizon: horizon, SeasonLength: opt.Season})
	if err != nil {
		return nil, nil, nil, "forecast failed: " + err.Error()
	}
	anomalies, _ := forecast.DetectAnomalies(series, forecast.AnomalyOptions{SeasonLength: opt.Season})

	return fc, anomalies, forecastNote(fc, horizon), ""
}

// forecastNote maps a forecast.Result onto the narrative's lightweight note.
func forecastNote(fc *forecast.Result, horizon int) *narrative.ForecastNote {
	if fc == nil || len(fc.Predictions) == 0 {
		return nil
	}
	last := fc.Predictions[len(fc.Predictions)-1]
	return &narrative.ForecastNote{
		Method:      string(fc.Method),
		Horizon:     horizon,
		First:       fc.Predictions[0].Value,
		Last:        last.Value,
		LowerLast:   last.Lower,
		UpperLast:   last.Upper,
		MAPEPercent: fc.MAPE,
	}
}

func anomalyNotes(anomalies []forecast.Anomaly, limit int) []narrative.AnomalyNote {
	if len(anomalies) == 0 {
		return nil
	}
	if len(anomalies) > limit {
		anomalies = anomalies[:limit]
	}
	notes := make([]narrative.AnomalyNote, len(anomalies))
	for i, a := range anomalies {
		notes[i] = narrative.AnomalyNote{
			When:     a.Time.UTC().Format("2006-01-02"),
			Observed: a.Value,
			Expected: a.Expected,
		}
	}
	return notes
}

func maxAnomalies(opt Options) int {
	if opt.MaxAnomalies > 0 {
		return opt.MaxAnomalies
	}
	return 5
}
