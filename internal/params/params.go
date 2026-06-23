// Package params is HowlerOps' canonical typed-parameter system. It defines
// reusable, validated inputs (filters for reports, inputs for runbooks, widgets
// for notebooks) and binds them safely into SQL templates that use
// {{name}} placeholders.
//
// It is the "build once" primitive from the platform plan: today the report
// engine and the template engine each carry their own ad-hoc placeholder logic.
// New surfaces build on this package, and the existing ones migrate onto it.
//
// Safety model: values are made SQL-safe by correct per-type quoting and
// escaping (string literals are single-quoted with doubled quotes; numbers are
// validated as numeric; identifiers are whitelist-validated). This is the
// dependable approach — unlike keyword blocklists, it has no false negatives
// (an attacker can't smuggle a payload past correct escaping) and no false
// positives (a legitimate value like "Robert'); DROP" is escaped, not
// rejected). Callers that want defense-in-depth can still run their own
// validation Pattern on top.
package params

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Type is the declared type of a parameter.
type Type string

const (
	TypeString     Type = "string"     // arbitrary text -> quoted string literal
	TypeNumber     Type = "number"     // float -> bare numeric literal
	TypeInteger    Type = "integer"    // integer -> bare integer literal
	TypeBoolean    Type = "boolean"    // bool -> TRUE/FALSE
	TypeDate       Type = "date"       // calendar date -> 'YYYY-MM-DD'
	TypeTimestamp  Type = "timestamp"  // instant -> 'YYYY-MM-DD HH:MM:SS' (UTC)
	TypeEnum       Type = "enum"       // string constrained to Options
	TypeList       Type = "list"       // multi-value -> comma-joined literals for IN (...)
	TypeIdentifier Type = "identifier" // table/column name -> whitelist-validated bare token
)

// Definition declares one parameter: its type, presentation, and constraints.
// The zero value is not valid; Name and Type are required.
type Definition struct {
	Name        string      // placeholder key, e.g. "status" for {{status}}
	Type        Type        // value type
	Label       string      // human label for UI
	Description string      // help text
	Required    bool        // if true, a value (or Default) must be present
	Default     interface{} // used when no value is supplied
	Options     []string    // allowed values for enum; allowed element values for list
	Pattern     string      // optional regex a string/enum value must match
	Min         *float64    // optional inclusive lower bound for number/integer
	Max         *float64    // optional inclusive upper bound for number/integer
	ElementType Type        // element type for list (defaults to TypeString)
}

// Float is a convenience for setting Min/Max.
func Float(v float64) *float64 { return &v }

// identRe matches a safe SQL identifier: a leading letter/underscore followed by
// letters, digits, or underscores, with optional dotted qualifiers
// (schema.table.column). No quoting characters are permitted.
var identRe = mustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// Value is a validated, typed parameter value that knows how to render itself
// as a SQL literal.
type Value struct {
	Def Definition
	raw interface{}
}

// Raw returns the coerced Go value (string, int64, float64, bool, time.Time, or
// []Value for lists).
func (v Value) Raw() interface{} { return v.raw }

// SQL renders the value as a safe SQL literal for substitution into a template.
func (v Value) SQL() (string, error) {
	return renderSQL(v.Def, v.raw)
}

func renderSQL(def Definition, raw interface{}) (string, error) {
	switch def.Type {
	case TypeString, TypeEnum:
		return quoteString(raw.(string))
	case TypeNumber:
		return strconv.FormatFloat(raw.(float64), 'f', -1, 64), nil
	case TypeInteger:
		return strconv.FormatInt(raw.(int64), 10), nil
	case TypeBoolean:
		if raw.(bool) {
			return "TRUE", nil
		}
		return "FALSE", nil
	case TypeDate:
		return "'" + raw.(time.Time).UTC().Format("2006-01-02") + "'", nil
	case TypeTimestamp:
		return "'" + raw.(time.Time).UTC().Format("2006-01-02 15:04:05") + "'", nil
	case TypeIdentifier:
		return raw.(string), nil // already whitelist-validated
	case TypeList:
		elems := raw.([]Value)
		parts := make([]string, len(elems))
		for i, e := range elems {
			s, err := e.SQL()
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return strings.Join(parts, ", "), nil
	default:
		return "", fmt.Errorf("params: unknown type %q", def.Type)
	}
}

// quoteString returns a single-quoted SQL string literal with embedded quotes
// doubled.
//
// Safety across dialects: doubling single quotes alone is sufficient only when
// backslash is an ordinary character (PostgreSQL with standard_conforming_strings
// on, SQLite). On MySQL (with NO_BACKSLASH_ESCAPES off, the default) and
// ClickHouse, backslash is an escape character inside string literals, so a
// trailing/odd backslash could consume the closing quote and break out of the
// literal (e.g. a lone backslash would render, under quote-doubling alone, with
// the closing quote escaped, leaving the string unterminated on MySQL). Because
// this renderer is
// dialect-agnostic and its output is concatenated directly into queries, we also
// double backslashes. That is safe on every supported dialect: where backslash
// is ordinary it merely renders a literal pair of backslashes (a cosmetic data
// difference for backslash-bearing strings), and where backslash escapes are
// active it correctly neutralises the escape. NUL bytes are rejected outright:
// no dialect accepts them in a literal meaningfully, and they enable
// string-truncation attacks against C-based drivers.
func quoteString(s string) (string, error) {
	if strings.IndexByte(s, 0) >= 0 {
		return "", fmt.Errorf("string value contains a NUL byte, which cannot be safely escaped")
	}
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'", nil
}

// Coerce validates and converts a single raw input against a definition,
// returning a typed Value. It applies Options/Pattern/Min/Max constraints.
func (def Definition) Coerce(raw interface{}) (Value, error) {
	switch def.Type {
	case TypeString, TypeEnum:
		s, err := toString(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		if def.Type == TypeEnum || len(def.Options) > 0 {
			if !contains(def.Options, s) {
				return Value{}, def.errf("value %q is not one of the allowed options", s)
			}
		}
		if err := matchPattern(def.Pattern, s); err != nil {
			return Value{}, def.err(err)
		}
		return Value{Def: def, raw: s}, nil
	case TypeNumber:
		f, err := toFloat(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		if err := checkRange(def, f); err != nil {
			return Value{}, def.err(err)
		}
		return Value{Def: def, raw: f}, nil
	case TypeInteger:
		n, err := toInt(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		if err := checkRange(def, float64(n)); err != nil {
			return Value{}, def.err(err)
		}
		return Value{Def: def, raw: n}, nil
	case TypeBoolean:
		b, err := toBool(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		return Value{Def: def, raw: b}, nil
	case TypeDate, TypeTimestamp:
		t, err := toTime(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		return Value{Def: def, raw: t}, nil
	case TypeIdentifier:
		s, err := toString(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		if !identRe.MatchString(s) {
			return Value{}, def.errf("value %q is not a valid SQL identifier", s)
		}
		if len(def.Options) > 0 && !contains(def.Options, s) {
			return Value{}, def.errf("identifier %q is not in the allow-list", s)
		}
		return Value{Def: def, raw: s}, nil
	case TypeList:
		elemType := def.ElementType
		if elemType == "" {
			elemType = TypeString
		}
		items, err := toSlice(raw)
		if err != nil {
			return Value{}, def.err(err)
		}
		elemDef := Definition{Name: def.Name, Type: elemType, Options: def.Options, Pattern: def.Pattern, Min: def.Min, Max: def.Max}
		values := make([]Value, 0, len(items))
		for _, item := range items {
			ev, err := elemDef.Coerce(item)
			if err != nil {
				return Value{}, err
			}
			values = append(values, ev)
		}
		return Value{Def: def, raw: values}, nil
	default:
		return Value{}, def.errf("unknown parameter type %q", def.Type)
	}
}

func (def Definition) err(err error) error {
	return fmt.Errorf("params: %q: %w", def.Name, err)
}

func (def Definition) errf(format string, a ...interface{}) error {
	return fmt.Errorf("params: %q: "+format, append([]interface{}{def.Name}, a...)...)
}

// --- coercion helpers ---------------------------------------------------------

func toString(raw interface{}) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case nil:
		return "", fmt.Errorf("expected a string, got nil")
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func toFloat(raw interface{}) (float64, error) {
	switch v := raw.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, fmt.Errorf("expected a number, got %q", v)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", raw)
	}
}

func toInt(raw interface{}) (int64, error) {
	switch v := raw.(type) {
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("expected an integer, got %v", v)
		}
		return int64(v), nil
	case float32:
		return toInt(float64(v))
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func toBool(raw interface{}) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case int:
		return v != 0, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y", "on":
			return true, nil
		case "false", "0", "no", "n", "off":
			return false, nil
		}
		return false, fmt.Errorf("expected a boolean, got %q", v)
	default:
		return false, fmt.Errorf("expected a boolean, got %T", raw)
	}
}

var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
	"01/02/2006",
}

func toTime(raw interface{}) (time.Time, error) {
	switch v := raw.(type) {
	case time.Time:
		return v, nil
	case int64:
		return time.Unix(v, 0).UTC(), nil
	case int:
		return time.Unix(int64(v), 0).UTC(), nil
	case float64:
		return time.Unix(int64(v), 0).UTC(), nil
	case string:
		s := strings.TrimSpace(v)
		for _, f := range timeFormats {
			if t, err := time.Parse(f, s); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("expected a date/time, got %q", v)
	default:
		return time.Time{}, fmt.Errorf("expected a date/time, got %T", raw)
	}
}

func toSlice(raw interface{}) ([]interface{}, error) {
	switch v := raw.(type) {
	case []interface{}:
		return v, nil
	case []string:
		out := make([]interface{}, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, nil
	case nil:
		return nil, fmt.Errorf("expected a list, got nil")
	default:
		// A single scalar is treated as a one-element list for convenience.
		return []interface{}{v}, nil
	}
}

func checkRange(def Definition, f float64) error {
	if def.Min != nil && f < *def.Min {
		return fmt.Errorf("value %v is below the minimum %v", f, *def.Min)
	}
	if def.Max != nil && f > *def.Max {
		return fmt.Errorf("value %v is above the maximum %v", f, *def.Max)
	}
	return nil
}

// matchPattern checks s against an optional regex constraint. An empty pattern
// is a no-op.
func matchPattern(pattern, s string) error {
	if pattern == "" {
		return nil
	}
	ok, err := regexp.MatchString(pattern, s)
	if err != nil {
		return fmt.Errorf("invalid validation pattern: %w", err)
	}
	if !ok {
		return fmt.Errorf("value %q does not match the required pattern", s)
	}
	return nil
}

func contains(opts []string, s string) bool {
	for _, o := range opts {
		if o == s {
			return true
		}
	}
	return false
}
