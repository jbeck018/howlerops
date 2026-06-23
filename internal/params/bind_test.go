package params

import "testing"

// Substitute must honor the same {{ name }} grammar Placeholders/Bind accept,
// including optional surrounding whitespace — the renderText paths (notify
// messages, markdown cells) regressed here, leaving spaced placeholders verbatim.
func TestSubstitute_HonorsWhitespaceAndUnknowns(t *testing.T) {
	render := func(name string) (string, bool) {
		switch name {
		case "status":
			return "open", true
		case "count":
			return "3", true
		default:
			return "", false
		}
	}

	cases := map[string]string{
		"value: {{status}}":          "value: open",              // no whitespace
		"value: {{ status }}":        "value: open",              // surrounding whitespace
		"value: {{  status  }}":      "value: open",              // extra whitespace
		"{{status}} and {{ count }}": "open and 3",               // mixed forms
		"unknown {{missing}} kept":   "unknown {{missing}} kept", // unknown left intact
	}
	for in, want := range cases {
		if got := Substitute(in, render); got != want {
			t.Errorf("Substitute(%q) = %q, want %q", in, got, want)
		}
	}
}

// A rendered value that itself contains a placeholder must not be re-expanded
// (single-pass), so a value can't smuggle another parameter's expansion.
func TestSubstitute_SinglePass(t *testing.T) {
	render := func(name string) (string, bool) {
		switch name {
		case "a":
			return "{{b}}", true
		case "b":
			return "SHOULD-NOT-APPEAR", true
		default:
			return "", false
		}
	}
	if got := Substitute("{{a}}", render); got != "{{b}}" {
		t.Errorf("Substitute single-pass = %q, want %q", got, "{{b}}")
	}
}
