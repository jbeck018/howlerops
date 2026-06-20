package forecast

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SeriesFromRows builds a Series from tabular query output (the shape returned
// by the query engine and the agent's SQLResult: a column list plus rows keyed
// by column name). It reads timeCol as the timestamp and valueCol as a numeric
// measure. Rows whose time or value cannot be parsed are skipped; the count of
// skipped rows is returned for diagnostics. It errors only if a named column is
// absent or no usable points remain.
func SeriesFromRows(columns []string, rows []map[string]interface{}, timeCol, valueCol string) (Series, int, error) {
	if !containsCol(columns, timeCol) {
		return nil, 0, fmt.Errorf("forecast: time column %q not found", timeCol)
	}
	if !containsCol(columns, valueCol) {
		return nil, 0, fmt.Errorf("forecast: value column %q not found", valueCol)
	}

	series := make(Series, 0, len(rows))
	skipped := 0
	for _, row := range rows {
		t, okT := coerceTime(row[timeCol])
		v, okV := coerceFloat(row[valueCol])
		if !okT || !okV {
			skipped++
			continue
		}
		series = append(series, Point{Time: t, Value: v})
	}
	if len(series) == 0 {
		return nil, skipped, fmt.Errorf("forecast: no usable (time, value) points in %d rows", len(rows))
	}
	return series, skipped, nil
}

// DetectColumns guesses a time column and a numeric value column from query
// output: the first column whose values parse as timestamps, and the first
// other column whose values parse as numbers. It enables an "auto" mode for the
// agent tool and report builder when the user hasn't named columns explicitly.
func DetectColumns(columns []string, rows []map[string]interface{}) (timeCol, valueCol string, ok bool) {
	if len(rows) == 0 {
		return "", "", false
	}
	sample := rows[0]
	for _, c := range columns {
		if timeCol == "" {
			if _, isT := coerceTime(sample[c]); isT {
				timeCol = c
				continue
			}
		}
		if valueCol == "" && c != timeCol {
			if _, isV := coerceFloat(sample[c]); isV {
				valueCol = c
			}
		}
	}
	return timeCol, valueCol, timeCol != "" && valueCol != ""
}

func containsCol(columns []string, name string) bool {
	for _, c := range columns {
		if c == name {
			return true
		}
	}
	return false
}

// coerceFloat converts a cell value to float64, accepting the numeric and
// stringy forms a SQL driver or JSON decoder might produce.
func coerceFloat(v interface{}) (float64, bool) {
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
	case uint:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case []byte:
		return parseFloat(string(n))
	case string:
		return parseFloat(n)
	default:
		return 0, false
	}
}

func parseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// timeLayouts covers the timestamp shapes commonly seen in SQL output.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999-07",
	"2006-01-02 15:04:05.999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"2006/01/02",
}

// coerceTime converts a cell value to time.Time, accepting time.Time, the usual
// string layouts, and integer/float Unix seconds.
func coerceTime(v interface{}) (time.Time, bool) {
	switch t := v.(type) {
	case time.Time:
		return t, true
	case int64:
		return time.Unix(t, 0).UTC(), true
	case int:
		return time.Unix(int64(t), 0).UTC(), true
	case float64:
		return time.Unix(int64(t), 0).UTC(), true
	case []byte:
		return parseTime(string(t))
	case string:
		return parseTime(t)
	default:
		return time.Time{}, false
	}
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	// Bare integer string -> Unix seconds.
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(n, 0).UTC(), true
	}
	return time.Time{}, false
}
