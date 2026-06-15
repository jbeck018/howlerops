package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// NormalizeValue converts database-specific types to JSON-compatible values
// This ensures consistent serialization across all database connectors
func NormalizeValue(val interface{}) interface{} {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	// Handle SQL null types
	case sql.NullString:
		if v.Valid {
			return v.String
		}
		return nil

	case sql.NullInt64:
		if v.Valid {
			return v.Int64
		}
		return nil

	case sql.NullInt32:
		if v.Valid {
			return v.Int32
		}
		return nil

	case sql.NullFloat64:
		if v.Valid {
			return v.Float64
		}
		return nil

	case sql.NullBool:
		if v.Valid {
			return v.Bool
		}
		return nil

	case sql.NullTime:
		if v.Valid {
			return v.Time.Format(time.RFC3339)
		}
		return nil

	// Handle byte arrays (convert to string for text data)
	case []byte:
		// Only attempt JSON decoding when the bytes can legally begin a JSON
		// value. Plain text/varchar columns are commonly returned as []byte and
		// would always fail json.Unmarshal, so this skips a wasted parse on the
		// hot path (every string cell of every result set).
		if jsonVal, ok := decodeJSONBytes(v); ok {
			return jsonVal
		}
		return string(v)

	// Handle time.Time
	case time.Time:
		return v.Format(time.RFC3339)

	// Handle pointers
	case *string:
		if v != nil {
			return *v
		}
		return nil

	case *int:
		if v != nil {
			return *v
		}
		return nil

	case *int64:
		if v != nil {
			return *v
		}
		return nil

	case *float64:
		if v != nil {
			return *v
		}
		return nil

	case *bool:
		if v != nil {
			return *v
		}
		return nil

	case *time.Time:
		if v != nil {
			return v.Format(time.RFC3339)
		}
		return nil

	// Handle arrays and slices
	case []interface{}:
		normalized := make([]interface{}, len(v))
		for i, item := range v {
			normalized[i] = NormalizeValue(item)
		}
		return normalized

	// Handle maps (for JSON/JSONB columns)
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(v))
		for key, value := range v {
			normalized[key] = NormalizeValue(value)
		}
		return normalized

	// Default: return as-is
	default:
		return val
	}
}

// decodeJSONBytes attempts to decode b as JSON, but only when its first
// non-whitespace byte can legally begin a JSON value (one of { [ " - t f n or a
// digit). Any other leading byte cannot be valid JSON, so json.Unmarshal would
// fail anyway — returning (nil, false) lets the caller fall back to a string
// without paying for the parse. Behaviour is identical to always trying to
// unmarshal; only the wasted attempts on plain text are skipped.
func decodeJSONBytes(b []byte) (interface{}, bool) {
	i := 0
	for i < len(b) {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			i++
			continue
		}
		break
	}
	if i >= len(b) {
		return nil, false
	}

	switch c := b[i]; {
	case c == '{', c == '[', c == '"', c == '-', c == 't', c == 'f', c == 'n', c >= '0' && c <= '9':
		var jsonVal interface{}
		if err := json.Unmarshal(b, &jsonVal); err == nil {
			return jsonVal, true
		}
	}
	return nil, false
}

// NormalizeRow normalizes all values in a row
func NormalizeRow(row []interface{}) []interface{} {
	normalized := make([]interface{}, len(row))
	for i, val := range row {
		normalized[i] = NormalizeValue(val)
	}
	return normalized
}

// ApplyPagination modifies a SQL query to add LIMIT and OFFSET clauses
// Returns the modified query and a boolean indicating if modification was successful
func ApplyPagination(query string, limit, offset int) (string, error) {
	if limit <= 0 {
		return query, nil
	}

	// Simple approach: append LIMIT and OFFSET
	// Note: This may not work for all query types (e.g., queries with existing LIMIT)
	// A production implementation should use a SQL parser
	paginatedQuery := fmt.Sprintf("%s LIMIT %d", query, limit)
	if offset > 0 {
		paginatedQuery = fmt.Sprintf("%s OFFSET %d", paginatedQuery, offset)
	}

	return paginatedQuery, nil
}

// ExtractTotalCount executes a COUNT(*) query to get total row count
// This is used for pagination to know the total number of rows
func ExtractTotalCount(query string) string {
	// Simple approach: wrap the original query in a COUNT(*)
	// This works for most SELECT queries
	return fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS count_query", query)
}
