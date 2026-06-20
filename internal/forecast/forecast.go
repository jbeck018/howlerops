// Package forecast provides a pure-Go time-series forecasting and anomaly
// detection engine for HowlerOps. It runs entirely in-process (no Python or
// external runtime), so it works in the desktop app, the server, and inside
// agent tools without extra dependencies.
//
// The engine implements the exponential-smoothing family (simple, double /
// Holt, and triple / Holt-Winters) plus a seasonal-naive baseline, and derives
// confidence intervals from in-sample residuals. Anomaly detection reuses the
// same smoothing fit and flags observations whose residuals fall outside a
// configurable band.
//
// Design notes:
//   - Inputs are plain time/value points; callers shape DuckDB/SQL aggregates
//     into a Series before calling in.
//   - Algorithms are deliberately simple and dependency-free; this is the
//     Phase 0 skeleton that later phases (report components, the Forecast agent
//     tool) build on. Heavier models can slot in behind the same Options/Result
//     contract.
package forecast

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Point is a single observation in a time series.
type Point struct {
	Time  time.Time
	Value float64
}

// Series is an ordered collection of observations. Most entry points sort the
// series ascending by Time defensively, so callers need not pre-sort.
type Series []Point

// Method selects the forecasting algorithm.
type Method string

const (
	// MethodAuto picks a method from the data: Holt-Winters when a usable
	// season length and enough history are present, otherwise Holt, otherwise
	// simple exponential smoothing.
	MethodAuto Method = "auto"
	// MethodSES is simple exponential smoothing (level only; no trend/season).
	MethodSES Method = "ses"
	// MethodHolt is double exponential smoothing (level + trend).
	MethodHolt Method = "holt"
	// MethodHoltWinters is triple exponential smoothing (level + trend +
	// additive seasonality). Requires Options.SeasonLength >= 2.
	MethodHoltWinters Method = "holt_winters"
	// MethodSeasonalNaive forecasts each future period as the value one season
	// earlier. A robust baseline; requires Options.SeasonLength >= 1.
	MethodSeasonalNaive Method = "seasonal_naive"
)

// Options configures a forecast. The zero value is invalid; at minimum set
// Horizon. Unset smoothing parameters (0) trigger a coarse grid search that
// minimises in-sample RMSE.
type Options struct {
	// Method selects the algorithm. Empty or MethodAuto auto-selects.
	Method Method
	// Horizon is the number of future periods to predict. Must be >= 1.
	Horizon int
	// SeasonLength is the number of periods in one season (e.g. 7 for daily
	// data with weekly seasonality, 12 for monthly-with-yearly). 0 disables
	// seasonality.
	SeasonLength int
	// ConfidenceLevel for the prediction interval, in (0,1). Defaults to 0.95.
	ConfidenceLevel float64
	// Alpha/Beta/Gamma are the level/trend/season smoothing factors in [0,1].
	// Leave any at 0 to have the engine search for it.
	Alpha float64
	Beta  float64
	Gamma float64
}

// Prediction is one forecasted future period with a confidence interval.
type Prediction struct {
	Time  time.Time
	Value float64
	Lower float64
	Upper float64
}

// Result holds the fit diagnostics and forward predictions.
type Result struct {
	// Method is the algorithm actually used (resolved from MethodAuto).
	Method Method
	// Fitted are the one-step-ahead in-sample predictions, aligned to the
	// input series (Fitted[i] predicts input[i]). Leading warm-up positions
	// that cannot be predicted are NaN.
	Fitted []float64
	// Residuals are input[i].Value - Fitted[i] where Fitted[i] is finite.
	Residuals []float64
	// Predictions are the Horizon future periods with confidence intervals.
	Predictions []Prediction
	// Resolved smoothing parameters and season length.
	Alpha        float64
	Beta         float64
	Gamma        float64
	SeasonLength int
	// Accuracy metrics computed over the finite-fitted positions.
	MAE  float64
	RMSE float64
	MAPE float64 // percentage (e.g. 12.5 == 12.5%); NaN if any actual is 0
}

// Errors returned by the engine.
var (
	ErrNoData         = errors.New("forecast: empty series")
	ErrInsufficient   = errors.New("forecast: not enough data points for the chosen method")
	ErrBadHorizon     = errors.New("forecast: horizon must be >= 1")
	ErrBadSeason      = errors.New("forecast: invalid season length for the chosen method")
	ErrIrregularTimes = errors.New("forecast: cannot infer a period from the series timestamps")
)

// Forecast fits the requested model to s and predicts Options.Horizon future
// periods. The series is sorted ascending by time defensively.
func Forecast(s Series, opt Options) (*Result, error) {
	if len(s) == 0 {
		return nil, ErrNoData
	}
	if opt.Horizon < 1 {
		return nil, ErrBadHorizon
	}
	if opt.ConfidenceLevel <= 0 || opt.ConfidenceLevel >= 1 {
		opt.ConfidenceLevel = 0.95
	}

	data := append(Series(nil), s...)
	sort.SliceStable(data, func(i, j int) bool { return data[i].Time.Before(data[j].Time) })

	values := make([]float64, len(data))
	for i, p := range data {
		values[i] = p.Value
	}

	period, err := inferPeriod(data)
	if err != nil {
		return nil, err
	}

	method := resolveMethod(opt, len(values))
	if err := validateForMethod(method, opt.SeasonLength, len(values)); err != nil {
		return nil, err
	}

	fitted, future, params := fitAndForecast(method, values, opt)

	res := &Result{
		Method:       method,
		Fitted:       fitted,
		Alpha:        params.alpha,
		Beta:         params.beta,
		Gamma:        params.gamma,
		SeasonLength: opt.SeasonLength,
	}
	res.Residuals, res.MAE, res.RMSE, res.MAPE = residualStats(values, fitted)

	// Confidence intervals widen with the square root of the horizon, a common
	// approximation when the per-step error is assumed roughly independent.
	sigma := stddev(res.Residuals)
	z := normalQuantile(0.5 + opt.ConfidenceLevel/2)
	last := data[len(data)-1].Time
	res.Predictions = make([]Prediction, len(future))
	for h, v := range future {
		width := z * sigma * math.Sqrt(float64(h+1))
		res.Predictions[h] = Prediction{
			Time:  last.Add(time.Duration(h+1) * period),
			Value: v,
			Lower: v - width,
			Upper: v + width,
		}
	}
	return res, nil
}

// resolveMethod turns MethodAuto / empty into a concrete method based on the
// data available.
func resolveMethod(opt Options, n int) Method {
	if opt.Method != "" && opt.Method != MethodAuto {
		return opt.Method
	}
	if opt.SeasonLength >= 2 && n >= 2*opt.SeasonLength {
		return MethodHoltWinters
	}
	if n >= 3 {
		return MethodHolt
	}
	return MethodSES
}

func validateForMethod(m Method, season, n int) error {
	switch m {
	case MethodSES:
		if n < 1 {
			return ErrInsufficient
		}
	case MethodHolt:
		if n < 2 {
			return ErrInsufficient
		}
	case MethodHoltWinters:
		if season < 2 {
			return ErrBadSeason
		}
		if n < 2*season {
			return ErrInsufficient
		}
	case MethodSeasonalNaive:
		if season < 1 {
			return ErrBadSeason
		}
		if n < season {
			return ErrInsufficient
		}
	default:
		return errors.New("forecast: unknown method " + string(m))
	}
	return nil
}

// inferPeriod returns the spacing between consecutive observations using the
// median delta, which is robust to occasional gaps. A single point has no
// inferable period, so a 1-day default is used (predictions still carry values,
// just with nominal timestamps).
func inferPeriod(s Series) (time.Duration, error) {
	if len(s) < 2 {
		return 24 * time.Hour, nil
	}
	deltas := make([]time.Duration, 0, len(s)-1)
	for i := 1; i < len(s); i++ {
		d := s[i].Time.Sub(s[i-1].Time)
		if d <= 0 {
			continue
		}
		deltas = append(deltas, d)
	}
	if len(deltas) == 0 {
		return 0, ErrIrregularTimes
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })
	return deltas[len(deltas)/2], nil
}

// residualStats computes residuals and accuracy metrics over positions where
// the fitted value is finite.
func residualStats(actual, fitted []float64) (residuals []float64, mae, rmse, mape float64) {
	var n int
	var sumAbs, sumSq, sumPct float64
	mapeValid := true
	for i := range actual {
		if i >= len(fitted) || math.IsNaN(fitted[i]) {
			continue
		}
		r := actual[i] - fitted[i]
		residuals = append(residuals, r)
		sumAbs += math.Abs(r)
		sumSq += r * r
		if actual[i] == 0 {
			mapeValid = false
		} else {
			sumPct += math.Abs(r / actual[i])
		}
		n++
	}
	if n == 0 {
		return residuals, 0, 0, math.NaN()
	}
	mae = sumAbs / float64(n)
	rmse = math.Sqrt(sumSq / float64(n))
	if mapeValid {
		mape = (sumPct / float64(n)) * 100
	} else {
		mape = math.NaN()
	}
	return residuals, mae, rmse, mape
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

// stddev returns the sample standard deviation; with fewer than two points it
// returns 0 so intervals degrade to point predictions rather than NaN.
func stddev(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	m := mean(xs)
	var s float64
	for _, x := range xs {
		d := x - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(xs)-1))
}

// normalQuantile is the inverse CDF of the standard normal distribution
// (Acklam's rational approximation), accurate to ~1e-9 over (0,1). Used to turn
// a confidence level into an interval multiplier without a stats dependency.
func normalQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	// Coefficients.
	a := []float64{-3.969683028665376e+01, 2.209460984245205e+02, -2.759285104469687e+02, 1.383577518672690e+02, -3.066479806614716e+01, 2.506628277459239e+00}
	b := []float64{-5.447609879822406e+01, 1.615858368580409e+02, -1.556989798598866e+02, 6.680131188771972e+01, -1.328068155288572e+01}
	c := []float64{-7.784894002430293e-03, -3.223964580411365e-01, -2.400758277161838e+00, -2.549732539343734e+00, 4.374664141464968e+00, 2.938163982698783e+00}
	d := []float64{7.784695709041462e-03, 3.224671290700398e-01, 2.445134137142996e+00, 3.754408661907416e+00}
	const pLow = 0.02425
	const pHigh = 1 - pLow
	var x float64
	switch {
	case p < pLow:
		q := math.Sqrt(-2 * math.Log(p))
		x = (((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	case p <= pHigh:
		q := p - 0.5
		r := q * q
		x = (((((a[0]*r+a[1])*r+a[2])*r+a[3])*r+a[4])*r + a[5]) * q /
			(((((b[0]*r+b[1])*r+b[2])*r+b[3])*r+b[4])*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		x = -(((((c[0]*q+c[1])*q+c[2])*q+c[3])*q+c[4])*q + c[5]) /
			((((d[0]*q+d[1])*q+d[2])*q+d[3])*q + 1)
	}
	return x
}
