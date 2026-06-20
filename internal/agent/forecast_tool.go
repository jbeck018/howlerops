package agent

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/jbeck018/howlerops/internal/forecast"
)

// forecastInput is the argument schema for the forecast tool.
type forecastInput struct {
	SQL         string `json:"sql" jsonschema:"required" jsonschema_description:"A read-only SQL SELECT returning a timestamp column and a numeric value column, ordered by time ascending"`
	TimeColumn  string `json:"time_column,omitempty" jsonschema_description:"Name of the timestamp column (auto-detected when omitted)"`
	ValueColumn string `json:"value_column,omitempty" jsonschema_description:"Name of the numeric value column (auto-detected when omitted)"`
	Horizon     int    `json:"horizon" jsonschema:"required" jsonschema_description:"Number of future periods to forecast"`
	Season      int    `json:"season_length,omitempty" jsonschema_description:"Periods per season for seasonality (e.g. 7 for weekly pattern on daily data); 0 for none"`
}

// buildForecastTool returns the forecast tool. It reuses the host's RunSQL
// capability to fetch the series, then fits a model entirely in-process, so it
// needs no additions to the Toolset interface.
func (e *Engine) buildForecastTool(rs *runState, connectionID string) (tool.BaseTool, error) {
	return utils.InferTool(
		"forecast",
		"Forecast a numeric time series. Runs a read-only SQL query returning a time column and a numeric value column, fits an exponential-smoothing model, and returns future predictions with confidence intervals plus any anomalies detected in the history. Use this for questions about trends, projections, 'what will X be', or unusual spikes/dips.",
		func(ctx context.Context, args forecastInput) (string, error) {
			sql := strings.TrimSpace(args.SQL)
			step := Step{Tool: "forecast", SQL: sql}
			if sql == "" {
				step.Output = "error: empty SQL"
				rs.record(step)
				return "Error: empty SQL statement.", nil
			}
			res, err := e.tools.RunSQL(ctx, connectionID, sql)
			if err != nil {
				step.Output = "error: " + err.Error()
				rs.record(step)
				return fmt.Sprintf("Query failed: %v", err), nil
			}
			rs.mu.Lock()
			rs.executedSQL = sql
			rs.lastResult = res
			rs.mu.Unlock()

			out, err := forecastFromResult(res, args.TimeColumn, args.ValueColumn, args.Horizon, args.Season)
			if err != nil {
				step.Output = "error: " + err.Error()
				rs.record(step)
				return out, nil // out carries a user-facing explanation
			}
			step.Result = res
			step.Output = out
			rs.record(step)
			return out, nil
		},
	)
}

// forecastFromResult is the pure core of the forecast tool: it turns a query
// result into a forecast narrative. Separated from the tool closure so it can
// be unit-tested without an LLM. On a usage error it returns a user-facing
// message AND a non-nil error; on success the message and a nil error.
func forecastFromResult(res *SQLResult, timeCol, valueCol string, horizon, season int) (string, error) {
	if res == nil {
		return "No query result to forecast.", fmt.Errorf("nil result")
	}
	if horizon <= 0 {
		horizon = 7
	}

	tc, vc := strings.TrimSpace(timeCol), strings.TrimSpace(valueCol)
	if tc == "" || vc == "" {
		dtc, dvc, ok := forecast.DetectColumns(res.Columns, res.Rows)
		if tc == "" {
			tc = dtc
		}
		if vc == "" {
			vc = dvc
		}
		if !ok && (tc == "" || vc == "") {
			msg := "Could not determine the time and value columns automatically. Re-run with time_column and value_column set."
			return msg, fmt.Errorf("column detection failed")
		}
	}

	series, skipped, err := forecast.SeriesFromRows(res.Columns, res.Rows, tc, vc)
	if err != nil {
		return fmt.Sprintf("Could not build a time series from the result: %v", err), err
	}

	fc, err := forecast.Forecast(series, forecast.Options{Horizon: horizon, SeasonLength: season})
	if err != nil {
		return fmt.Sprintf("Forecast failed: %v", err), err
	}
	anomalies, _ := forecast.DetectAnomalies(series, forecast.AnomalyOptions{SeasonLength: season})

	return formatForecast(fc, anomalies, len(series), skipped, tc, vc), nil
}

// formatForecast renders a forecast result as concise text for the model to
// summarise to the user.
func formatForecast(fc *forecast.Result, anomalies []forecast.Anomaly, points, skipped int, timeCol, valueCol string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Forecast of %q over %q using %s (%d points", valueCol, timeCol, fc.Method, points)
	if skipped > 0 {
		fmt.Fprintf(&b, ", %d unparseable rows skipped", skipped)
	}
	b.WriteString(").\n")

	fmt.Fprintf(&b, "Fit accuracy: MAE=%s, RMSE=%s", num(fc.MAE), num(fc.RMSE))
	if !math.IsNaN(fc.MAPE) {
		fmt.Fprintf(&b, ", MAPE=%.1f%%", fc.MAPE)
	}
	b.WriteString("\n\nPredictions (value with confidence interval):\n")
	for _, p := range fc.Predictions {
		fmt.Fprintf(&b, "  %s: %s  [%s, %s]\n",
			p.Time.Format("2006-01-02"), num(p.Value), num(p.Lower), num(p.Upper))
	}

	if len(anomalies) > 0 {
		fmt.Fprintf(&b, "\nAnomalies in history (%d):\n", len(anomalies))
		limit := len(anomalies)
		if limit > 10 {
			limit = 10
		}
		for _, a := range anomalies[:limit] {
			fmt.Fprintf(&b, "  %s: observed %s vs expected %s (%.1f deviations)\n",
				a.Time.Format("2006-01-02"), num(a.Value), num(a.Expected), a.Score)
		}
		if len(anomalies) > limit {
			fmt.Fprintf(&b, "  ...and %d more\n", len(anomalies)-limit)
		}
	}
	return b.String()
}

// num formats a float compactly: no trailing zeros, two-decimal cap for
// fractional values.
func num(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "n/a"
	}
	if f == math.Trunc(f) {
		return fmt.Sprintf("%.0f", f)
	}
	return fmt.Sprintf("%.2f", f)
}
