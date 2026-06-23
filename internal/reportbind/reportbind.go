// Package reportbind binds a report component's top-level {{filter}}
// placeholders to SQL-safe literals using the canonical params engine. It lives
// apart from the services package (which pulls in the Wails GUI runtime) so the
// binding logic stays dependency-light and unit-testable.
package reportbind

import (
	"time"

	"github.com/jbeck018/howlerops/internal/params"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// Apply substitutes the component's top-level {{filter}} placeholders with
// SQL-safe literals. Values come from the runtime filters, falling back to the
// report's filter-field defaults; field definitions also supply required-ness.
// Types are inferred from the resolved value so rendering matches historical
// behavior, while the shared engine adds consistent escaping and list/IN
// support. Placeholders not listed in the component's TopLevelFilter are left
// untouched for later context injection.
func Apply(sql string, report *storage.Report, component *storage.ReportComponent, filters map[string]interface{}) (string, error) {
	keys := component.Query.TopLevelFilter
	if len(keys) == 0 {
		return sql, nil
	}

	fields := make(map[string]storage.ReportFilterField, len(report.Filter.Fields))
	for _, f := range report.Filter.Fields {
		fields[f.Key] = f
	}

	defs := make([]params.Definition, 0, len(keys))
	raw := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		val, ok := filters[key]
		field, hasField := fields[key]
		if (!ok || val == nil) && hasField && field.DefaultValue != nil {
			val, ok = field.DefaultValue, true
		}

		def := params.Definition{Name: key, Type: inferParamType(val)}
		if def.Type == params.TypeList {
			def.ElementType = inferListElementType(val)
		}
		if hasField {
			def.Required = field.Required
		}
		defs = append(defs, def)
		if ok && val != nil {
			raw[key] = val
		}
	}

	// AllowUndefined leaves non-filter placeholders (e.g. context tokens) for
	// later injection; NullForMissing renders optional, unsupplied filters as
	// NULL instead of leaving an invalid {{token}} in the SQL.
	return params.Bind(sql, defs, raw, params.BindOptions{AllowUndefined: true, NullForMissing: true})
}

// inferParamType maps a runtime filter value to a params type, preserving the
// previous value-driven rendering semantics.
func inferParamType(v interface{}) params.Type {
	switch v.(type) {
	case bool:
		return params.TypeBoolean
	case time.Time:
		return params.TypeTimestamp
	case []interface{}, []string:
		return params.TypeList
	case int, int32, int64:
		// Integers render via the integer path so large values (beyond
		// float64's 2^53 exact range) are not silently rounded, which would
		// otherwise produce a WHERE clause matching the wrong row.
		return params.TypeInteger
	case float32, float64:
		return params.TypeNumber
	default:
		return params.TypeString
	}
}

// inferListElementType inspects the first element of a list value to pick an
// element type, so numeric lists render as 1, 2, 3 and string lists as quoted
// literals.
func inferListElementType(v interface{}) params.Type {
	switch list := v.(type) {
	case []interface{}:
		if len(list) > 0 {
			return inferParamType(list[0])
		}
	case []string:
		return params.TypeString
	}
	return params.TypeString
}
