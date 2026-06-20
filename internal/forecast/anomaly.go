package forecast

import (
	"math"
	"sort"
	"time"
)

// AnomalyMethod selects how residuals are scored.
type AnomalyMethod string

const (
	// AnomalyZScore flags points whose residual is more than Threshold sample
	// standard deviations from the mean residual. Sensitive to extreme values
	// but simple and fast.
	AnomalyZScore AnomalyMethod = "zscore"
	// AnomalyMAD uses the median absolute deviation (a robust z-score). Less
	// affected by the very outliers it is trying to find; a good default.
	AnomalyMAD AnomalyMethod = "mad"
	// AnomalyIQR flags residuals outside [Q1 - k*IQR, Q3 + k*IQR].
	AnomalyIQR AnomalyMethod = "iqr"
)

// AnomalyOptions configures detection. Detection first removes trend and
// seasonality by fitting a smoothing model, then scores the residuals.
type AnomalyOptions struct {
	// Method selects the residual scoring approach. Defaults to AnomalyMAD.
	Method AnomalyMethod
	// SeasonLength, when >= 2, removes seasonality via Holt-Winters before
	// scoring; otherwise Holt (or SES for very short series) is used.
	SeasonLength int
	// Threshold is the cutoff: number of (robust) standard deviations for
	// zscore/mad, or the IQR multiplier for iqr. Defaults to 3.0 for
	// zscore/mad and 1.5 for iqr.
	Threshold float64
}

// Anomaly marks a single observation flagged as anomalous.
type Anomaly struct {
	Index    int // position in the (time-sorted) series
	Time     time.Time
	Value    float64 // observed value
	Expected float64 // model's fitted value
	Residual float64 // Value - Expected
	Score    float64 // how many (robust) deviations out, signed
}

// DetectAnomalies fits a smoothing model to s and returns the observations whose
// residuals exceed the configured threshold. The series is sorted ascending by
// time defensively. Warm-up positions without a fitted value are skipped.
func DetectAnomalies(s Series, opt AnomalyOptions) ([]Anomaly, error) {
	if len(s) == 0 {
		return nil, ErrNoData
	}
	if opt.Method == "" {
		opt.Method = AnomalyMAD
	}
	if opt.Threshold <= 0 {
		if opt.Method == AnomalyIQR {
			opt.Threshold = 1.5
		} else {
			opt.Threshold = 3.0
		}
	}

	data := append(Series(nil), s...)
	sort.SliceStable(data, func(i, j int) bool { return data[i].Time.Before(data[j].Time) })
	values := make([]float64, len(data))
	for i, p := range data {
		values[i] = p.Value
	}

	// Fit a model purely to obtain residuals; horizon is irrelevant here.
	method := resolveMethod(Options{SeasonLength: opt.SeasonLength}, len(values))
	if !methodViable(method, opt.SeasonLength, len(values)) {
		// Fall back to the mean as the "expected" value for tiny series rather
		// than failing outright.
		return detectAroundMean(data, values, opt), nil
	}
	fitted, _, _ := fitAndForecast(method, values, Options{SeasonLength: opt.SeasonLength, Horizon: 1})

	residuals := make([]float64, 0, len(values))
	idx := make([]int, 0, len(values))
	for i := range values {
		if i < len(fitted) && !math.IsNaN(fitted[i]) {
			residuals = append(residuals, values[i]-fitted[i])
			idx = append(idx, i)
		}
	}
	if len(residuals) == 0 {
		return nil, nil
	}

	return scoreResiduals(data, values, fitted, residuals, idx, opt), nil
}

// scoreResiduals applies the chosen scoring method to the residual series and
// emits anomalies above threshold.
func scoreResiduals(data Series, values, fitted, residuals []float64, idx []int, opt AnomalyOptions) []Anomaly {
	var center, scale float64
	var lowCut, highCut float64
	useCut := false

	switch opt.Method {
	case AnomalyIQR:
		q1, q3 := quartiles(residuals)
		iqr := q3 - q1
		lowCut = q1 - opt.Threshold*iqr
		highCut = q3 + opt.Threshold*iqr
		useCut = true
	case AnomalyZScore:
		center = mean(residuals)
		scale = stddev(residuals)
	default: // AnomalyMAD
		center = median(residuals)
		scale = medianAbsDeviation(residuals, center) * 1.4826 // normal-consistent
	}

	var out []Anomaly
	for k, i := range idx {
		r := residuals[k]
		var score float64
		var flagged bool
		if useCut {
			if r < lowCut || r > highCut {
				flagged = true
			}
			if scaleW := (highCut - lowCut) / 2; scaleW > 0 {
				score = (r - (highCut+lowCut)/2) / scaleW
			}
		} else {
			if scale > 0 {
				score = (r - center) / scale
			} else if r != center {
				score = math.Inf(map1(r - center))
			}
			if math.Abs(score) > opt.Threshold {
				flagged = true
			}
		}
		if flagged {
			out = append(out, Anomaly{
				Index:    i,
				Time:     data[i].Time,
				Value:    values[i],
				Expected: fitted[i],
				Residual: r,
				Score:    score,
			})
		}
	}
	return out
}

// detectAroundMean is the degenerate path for series too short to fit a model:
// score deviations from the mean.
func detectAroundMean(data Series, values []float64, opt AnomalyOptions) []Anomaly {
	m := mean(values)
	sd := stddev(values)
	if sd == 0 {
		return nil
	}
	var out []Anomaly
	for i, v := range values {
		score := (v - m) / sd
		if math.Abs(score) > opt.Threshold {
			out = append(out, Anomaly{
				Index:    i,
				Time:     data[i].Time,
				Value:    v,
				Expected: m,
				Residual: v - m,
				Score:    score,
			})
		}
	}
	return out
}

func map1(x float64) int {
	if x < 0 {
		return -1
	}
	return 1
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func medianAbsDeviation(xs []float64, center float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	dev := make([]float64, len(xs))
	for i, x := range xs {
		dev[i] = math.Abs(x - center)
	}
	return median(dev)
}

// quartiles returns Q1 and Q3 using linear interpolation on the sorted values.
func quartiles(xs []float64) (q1, q3 float64) {
	c := append([]float64(nil), xs...)
	sort.Float64s(c)
	return percentile(c, 0.25), percentile(c, 0.75)
}

// percentile returns the p-quantile (0..1) of an already-sorted slice using
// linear interpolation between closest ranks.
func percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
