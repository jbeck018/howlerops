package forecast

import "math"

// params carries the resolved smoothing factors used by a fit.
type params struct {
	alpha float64
	beta  float64
	gamma float64
}

// fitAndForecast dispatches to the chosen method, returning the in-sample
// fitted series (aligned to values, NaN during warm-up), the future point
// forecasts (length Options.Horizon), and the resolved parameters. When a
// smoothing factor is left at 0 in opt, a coarse grid search selects the value
// minimising in-sample RMSE.
func fitAndForecast(m Method, values []float64, opt Options) ([]float64, []float64, params) {
	switch m {
	case MethodSeasonalNaive:
		fitted, future := seasonalNaive(values, opt.SeasonLength, opt.Horizon)
		return fitted, future, params{}
	case MethodSES:
		alpha := chooseAlpha(opt.Alpha, func(a float64) []float64 {
			f, _ := ses(values, a, opt.Horizon)
			return f
		}, values)
		fitted, future := ses(values, alpha, opt.Horizon)
		return fitted, future, params{alpha: alpha}
	case MethodHolt:
		p := chooseHolt(values, opt)
		fitted, future := holt(values, p.alpha, p.beta, opt.Horizon)
		return fitted, future, p
	case MethodHoltWinters:
		p := chooseHoltWinters(values, opt)
		fitted, future := holtWinters(values, opt.SeasonLength, p.alpha, p.beta, p.gamma, opt.Horizon)
		return fitted, future, p
	default:
		// Should be unreachable; validateForMethod guards callers.
		fitted, future := ses(values, 0.3, opt.Horizon)
		return fitted, future, params{alpha: 0.3}
	}
}

// ses is simple exponential smoothing: level only.
func ses(values []float64, alpha float64, horizon int) (fitted, future []float64) {
	fitted = make([]float64, len(values))
	if len(values) == 0 {
		return fitted, make([]float64, horizon)
	}
	level := values[0]
	fitted[0] = math.NaN() // nothing to predict the first point from
	for t := 1; t < len(values); t++ {
		fitted[t] = level
		level = alpha*values[t] + (1-alpha)*level
	}
	future = make([]float64, horizon)
	for h := range future {
		future[h] = level
	}
	return fitted, future
}

// holt is double exponential smoothing: level + linear trend.
func holt(values []float64, alpha, beta float64, horizon int) (fitted, future []float64) {
	fitted = make([]float64, len(values))
	if len(values) < 2 {
		for i := range fitted {
			fitted[i] = math.NaN()
		}
		future = make([]float64, horizon)
		v := 0.0
		if len(values) > 0 {
			v = values[0]
		}
		for h := range future {
			future[h] = v
		}
		return fitted, future
	}
	level := values[0]
	trend := values[1] - values[0]
	fitted[0] = math.NaN()
	for t := 1; t < len(values); t++ {
		fitted[t] = level + trend
		prevLevel := level
		level = alpha*values[t] + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
	}
	future = make([]float64, horizon)
	for h := range future {
		future[h] = level + float64(h+1)*trend
	}
	return fitted, future
}

// holtWinters is triple exponential smoothing with additive seasonality.
func holtWinters(values []float64, season int, alpha, beta, gamma float64, horizon int) (fitted, future []float64) {
	n := len(values)
	fitted = make([]float64, n)
	for i := range fitted {
		fitted[i] = math.NaN()
	}
	if season < 2 || n < 2*season {
		// Caller guards this, but degrade gracefully.
		return holt(values, alpha, beta, horizon)
	}

	// Initial level: mean of the first season.
	level := mean(values[:season])
	// Initial trend: average per-period change across the first two seasons.
	var trend float64
	for i := 0; i < season; i++ {
		trend += (values[season+i] - values[i]) / float64(season)
	}
	trend /= float64(season)
	// Initial seasonal components (additive): deviation from the first-season mean.
	seasonal := make([]float64, season)
	for i := 0; i < season; i++ {
		seasonal[i] = values[i] - level
	}

	for t := season; t < n; t++ {
		si := t % season
		fitted[t] = level + trend + seasonal[si]
		prevLevel := level
		level = alpha*(values[t]-seasonal[si]) + (1-alpha)*(level+trend)
		trend = beta*(level-prevLevel) + (1-beta)*trend
		seasonal[si] = gamma*(values[t]-level) + (1-gamma)*seasonal[si]
	}

	future = make([]float64, horizon)
	for h := 0; h < horizon; h++ {
		si := (n + h) % season
		future[h] = level + float64(h+1)*trend + seasonal[si]
	}
	return fitted, future
}

// seasonalNaive forecasts each future period as the observation one season
// earlier, and fits each in-sample point the same way.
func seasonalNaive(values []float64, season, horizon int) (fitted, future []float64) {
	n := len(values)
	fitted = make([]float64, n)
	for t := 0; t < n; t++ {
		if t-season < 0 {
			fitted[t] = math.NaN()
		} else {
			fitted[t] = values[t-season]
		}
	}
	future = make([]float64, horizon)
	for h := 0; h < horizon; h++ {
		// Walk back to the matching period within the last observed season,
		// extending forward by re-using already-forecast values when needed.
		idx := n + h - season
		switch {
		case idx >= 0 && idx < n:
			future[h] = values[idx]
		case idx >= n:
			future[h] = future[idx-n]
		default:
			future[h] = values[n-1]
		}
	}
	return fitted, future
}

// --- parameter search ---------------------------------------------------------

// grid is the coarse search space for smoothing factors. It keeps the search
// cheap (so this runs comfortably in the UI thread) while still beating a fixed
// default in practice.
var grid = []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}

// rmseOf computes in-sample RMSE for a fitted series against actuals.
func rmseOf(actual, fitted []float64) float64 {
	var n int
	var sumSq float64
	for i := range actual {
		if i >= len(fitted) || math.IsNaN(fitted[i]) {
			continue
		}
		d := actual[i] - fitted[i]
		sumSq += d * d
		n++
	}
	if n == 0 {
		return math.Inf(1)
	}
	return math.Sqrt(sumSq / float64(n))
}

// chooseAlpha returns the fixed alpha if set, else grid-searches it.
func chooseAlpha(fixed float64, fit func(a float64) []float64, values []float64) float64 {
	if fixed > 0 {
		return fixed
	}
	best, bestErr := grid[0], math.Inf(1)
	for _, a := range grid {
		if e := rmseOf(values, fit(a)); e < bestErr {
			best, bestErr = a, e
		}
	}
	return best
}

func chooseHolt(values []float64, opt Options) params {
	if opt.Alpha > 0 && opt.Beta > 0 {
		return params{alpha: opt.Alpha, beta: opt.Beta}
	}
	best := params{alpha: grid[0], beta: grid[0]}
	bestErr := math.Inf(1)
	for _, a := range grid {
		if opt.Alpha > 0 {
			a = opt.Alpha
		}
		for _, b := range grid {
			if opt.Beta > 0 {
				b = opt.Beta
			}
			f, _ := holt(values, a, b, 1)
			if e := rmseOf(values, f); e < bestErr {
				best, bestErr = params{alpha: a, beta: b}, e
			}
			if opt.Beta > 0 {
				break
			}
		}
		if opt.Alpha > 0 {
			break
		}
	}
	return best
}

func chooseHoltWinters(values []float64, opt Options) params {
	fixed := opt.Alpha > 0 && opt.Beta > 0 && opt.Gamma > 0
	if fixed {
		return params{alpha: opt.Alpha, beta: opt.Beta, gamma: opt.Gamma}
	}
	best := params{alpha: grid[0], beta: grid[0], gamma: grid[0]}
	bestErr := math.Inf(1)
	for _, a := range grid {
		if opt.Alpha > 0 {
			a = opt.Alpha
		}
		for _, b := range grid {
			if opt.Beta > 0 {
				b = opt.Beta
			}
			for _, g := range grid {
				if opt.Gamma > 0 {
					g = opt.Gamma
				}
				f, _ := holtWinters(values, opt.SeasonLength, a, b, g, 1)
				if e := rmseOf(values, f); e < bestErr {
					best, bestErr = params{alpha: a, beta: b, gamma: g}, e
				}
				if opt.Gamma > 0 {
					break
				}
			}
			if opt.Beta > 0 {
				break
			}
		}
		if opt.Alpha > 0 {
			break
		}
	}
	return best
}
