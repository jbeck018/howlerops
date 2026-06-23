// Package narrative turns query/report results into executive prose ("Insight
// Briefs") using the user's configured AI provider. It is deliberately
// dependency-light and Wails-free so it is unit-testable and reusable from the
// report engine, notebooks, and agent tools.
//
// Privacy: the generator computes aggregates from the rows itself and sends
// ONLY schema + aggregates to the model — never raw rows. This holds the
// platform's local-first privacy posture for any provider (and a local Ollama
// model gives a fully-offline path).
package narrative

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Kind classifies a column for summarisation and prompt rendering.
type Kind string

const (
	KindNumeric     Kind = "numeric"
	KindCategorical Kind = "categorical"
	KindTemporal    Kind = "temporal"
	KindOther       Kind = "other"
)

// ValueCount is one value and how often it appears (an aggregate, not a row).
type ValueCount struct {
	Value string
	Count int
}

// ColumnSummary is the aggregated profile of a single column.
type ColumnSummary struct {
	Name  string
	Kind  Kind
	Count int // non-null values seen
	Nulls int

	// Numeric aggregates (valid when Kind == KindNumeric).
	Min  float64
	Max  float64
	Mean float64
	Sum  float64

	// Categorical aggregates (valid when Kind == KindCategorical).
	Distinct int
	Top      []ValueCount

	// Temporal range (valid when Kind == KindTemporal).
	Earliest string
	Latest   string
}

// DataSummary is the aggregate profile of a result set. It contains no raw rows.
type DataSummary struct {
	RowCount int
	Columns  []ColumnSummary
}

const defaultTopK = 5

// maxCategoryValues caps the cardinality at which a column is still treated as a
// genuine category. Above it (or when every value is unique), the column is
// identifier-like (ids, emails, SSNs) and its values are NOT sampled into the
// prompt, preserving the no-raw-rows privacy contract.
const maxCategoryValues = 50

// Summarize profiles a result set (column list + rows keyed by column name) into
// aggregates suitable for an LLM prompt. Raw values never leave this function
// except as bounded top-K frequency counts for low-cardinality categoricals.
func Summarize(columns []string, rows []map[string]interface{}) DataSummary {
	summary := DataSummary{RowCount: len(rows), Columns: make([]ColumnSummary, 0, len(columns))}
	for _, col := range columns {
		summary.Columns = append(summary.Columns, summarizeColumn(col, rows))
	}
	return summary
}

func summarizeColumn(name string, rows []map[string]interface{}) ColumnSummary {
	cs := ColumnSummary{Name: name, Kind: KindOther}

	var (
		numeric    []float64
		temporal   []time.Time
		categories = map[string]int{}
		nonNull    int
		isNumeric  = true
		isTemporal = true
	)

	for _, row := range rows {
		v, ok := row[name]
		if !ok || v == nil {
			cs.Nulls++
			continue
		}
		nonNull++

		if f, ok := toFloat(v); ok {
			numeric = append(numeric, f)
		} else {
			isNumeric = false
		}
		if t, ok := toTime(v); ok {
			temporal = append(temporal, t)
		} else {
			isTemporal = false
		}
		categories[toString(v)]++
	}

	cs.Count = nonNull
	if nonNull == 0 {
		return cs
	}

	switch {
	case isNumeric:
		cs.Kind = KindNumeric
		cs.Min, cs.Max = numeric[0], numeric[0]
		for _, f := range numeric {
			cs.Sum += f
			cs.Min = math.Min(cs.Min, f)
			cs.Max = math.Max(cs.Max, f)
		}
		cs.Mean = cs.Sum / float64(len(numeric))
	case isTemporal:
		cs.Kind = KindTemporal
		earliest, latest := temporal[0], temporal[0]
		for _, t := range temporal {
			if t.Before(earliest) {
				earliest = t
			}
			if t.After(latest) {
				latest = t
			}
		}
		cs.Earliest = earliest.UTC().Format(time.RFC3339)
		cs.Latest = latest.UTC().Format(time.RFC3339)
	default:
		cs.Kind = KindCategorical
		cs.Distinct = len(categories)
		// Only sample values for low-cardinality categories that actually
		// repeat. Columns where values are mostly unique are identifiers, and
		// emitting their values would leak raw data into the prompt. As a second
		// guard, never sample values that look like PII (emails, SSNs) even when
		// they happen to repeat in a small result set — the cardinality
		// heuristic alone cannot catch those.
		if cs.Distinct < nonNull && cs.Distinct <= maxCategoryValues && !looksLikePII(categories) {
			cs.Top = topValues(categories, defaultTopK)
		}
	}
	return cs
}

// ssnPattern matches a US Social Security Number (NNN-NN-NNNN), optionally
// surrounded by whitespace.
var ssnPattern = regexp.MustCompile(`^\s*\d{3}-\d{2}-\d{4}\s*$`)

// emailPattern matches an email-shaped token: a non-empty local part, an "@",
// and a non-empty domain part, none of which contain whitespace or a second "@".
// A dot in the domain is NOT required, so intranet/local addresses like
// "alice@localhost" are still suppressed — the previous "contains @ and ."
// heuristic missed those and would have leaked them.
var emailPattern = regexp.MustCompile(`(^|\s)[^\s@]+@[^\s@]+(\s|$)`)

// looksLikePII reports whether any sampled category value looks like personally
// identifiable information that must never reach the prompt — currently emails
// and US SSNs. A single matching value suppresses sampling for the whole column,
// because mixed identifier columns are still identifier columns.
func looksLikePII(categories map[string]int) bool {
	for v := range categories {
		if emailPattern.MatchString(v) {
			return true
		}
		if ssnPattern.MatchString(v) {
			return true
		}
	}
	return false
}

// topValues returns the k most frequent values, ties broken by value for
// deterministic output.
func topValues(counts map[string]int, k int) []ValueCount {
	out := make([]ValueCount, 0, len(counts))
	for v, c := range counts {
		out = append(out, ValueCount{Value: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// --- coercion helpers (kept local so the package has no heavy deps) ---

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

var temporalLayouts = []string{
	time.RFC3339Nano, time.RFC3339,
	"2006-01-02 15:04:05", "2006-01-02T15:04:05", "2006-01-02",
}

func toTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case string:
		s := strings.TrimSpace(t)
		// Reject bare integers so numeric ids aren't misread as timestamps.
		if _, err := strconv.ParseFloat(s, 64); err == nil {
			return time.Time{}, false
		}
		for _, layout := range temporalLayouts {
			if parsed, err := time.Parse(layout, s); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}

func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}
