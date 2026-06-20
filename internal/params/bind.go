package params

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// placeholderRe matches {{ name }} with optional surrounding whitespace.
var placeholderRe = mustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// mustCompile is a tiny indirection so the package has a single regexp import
// site and the compile happens at package init.
func mustCompile(expr string) *regexp.Regexp { return regexp.MustCompile(expr) }

// Placeholders returns the distinct parameter names referenced in a template,
// in first-seen order.
func Placeholders(template string) []string {
	matches := placeholderRe.FindAllStringSubmatch(template, -1)
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// Resolve validates raw inputs against the definitions, applying defaults and
// required checks, and returns the typed values keyed by name. Unknown keys in
// raw are ignored (a definition is the source of truth); missing required
// values are errors.
func Resolve(defs []Definition, raw map[string]interface{}) (map[string]Value, error) {
	out := make(map[string]Value, len(defs))
	var missing []string
	for _, def := range defs {
		if def.Name == "" {
			return nil, fmt.Errorf("params: definition with empty name")
		}
		val, ok := raw[def.Name]
		if !ok || val == nil {
			if def.Default != nil {
				val = def.Default
			} else if def.Required {
				missing = append(missing, def.Name)
				continue
			} else {
				continue // optional, no value -> not bound (renders as NULL on bind)
			}
		}
		v, err := def.Coerce(val)
		if err != nil {
			return nil, err
		}
		out[def.Name] = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("params: missing required parameter(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// BindOptions tunes how Bind handles edge cases.
type BindOptions struct {
	// AllowUndefined permits {{placeholders}} that have no matching definition;
	// they are left untouched instead of erroring. Defaults to false (fail
	// closed) so a typo can't silently ship an un-substituted template.
	AllowUndefined bool
	// NullForMissing renders optional, unsupplied parameters as SQL NULL. When
	// false (default) an optional placeholder with no value is an error, which
	// keeps generated SQL explicit.
	NullForMissing bool
}

// Bind validates raw against defs and substitutes every {{name}} placeholder in
// the template with a SQL-safe literal. It is the primary entry point.
func Bind(template string, defs []Definition, raw map[string]interface{}, opt BindOptions) (string, error) {
	values, err := Resolve(defs, raw)
	if err != nil {
		return "", err
	}
	defByName := make(map[string]Definition, len(defs))
	for _, d := range defs {
		defByName[d.Name] = d
	}

	var bindErr error
	result := placeholderRe.ReplaceAllStringFunc(template, func(match string) string {
		name := placeholderRe.FindStringSubmatch(match)[1]
		if v, ok := values[name]; ok {
			lit, err := v.SQL()
			if err != nil {
				bindErr = err
				return match
			}
			return lit
		}
		// No resolved value. Distinguish "defined but optional/unsupplied" from
		// "undefined placeholder".
		if _, defined := defByName[name]; defined {
			if opt.NullForMissing {
				return "NULL"
			}
			bindErr = fmt.Errorf("params: no value for optional parameter %q (set NullForMissing to render NULL)", name)
			return match
		}
		if opt.AllowUndefined {
			return match
		}
		bindErr = fmt.Errorf("params: template references undefined parameter %q", name)
		return match
	})
	if bindErr != nil {
		return "", bindErr
	}
	return result, nil
}
