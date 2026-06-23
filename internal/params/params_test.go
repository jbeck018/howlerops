package params

import (
	"strings"
	"testing"
	"time"
)

func TestBind_BasicTypes(t *testing.T) {
	defs := []Definition{
		{Name: "status", Type: TypeString, Required: true},
		{Name: "limit", Type: TypeInteger, Required: true},
		{Name: "ratio", Type: TypeNumber, Required: true},
		{Name: "active", Type: TypeBoolean, Required: true},
	}
	raw := map[string]interface{}{
		"status": "active",
		"limit":  10,
		"ratio":  1.5,
		"active": true,
	}
	got, err := Bind("WHERE s={{status}} AND n={{limit}} AND r={{ratio}} AND a={{active}}", defs, raw, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "WHERE s='active' AND n=10 AND r=1.5 AND a=TRUE"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBind_EscapesStringSafely(t *testing.T) {
	// The canonical injection payload must be SAFELY ESCAPED, not rejected:
	// correct quoting neutralises it without false-positive blocking.
	defs := []Definition{{Name: "name", Type: TypeString, Required: true}}
	raw := map[string]interface{}{"name": "Robert'); DROP TABLE students;--"}
	got, err := Bind("WHERE name = {{name}}", defs, raw, BindOptions{})
	if err != nil {
		t.Fatalf("escaping should not error: %v", err)
	}
	want := "WHERE name = 'Robert''); DROP TABLE students;--'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// The dangerous quote is doubled, so it cannot break out of the literal.
	if strings.Count(got, "'")%2 != 0 {
		t.Errorf("unbalanced quotes in %q", got)
	}
}

func TestBind_EscapesBackslashForMySQLDialect(t *testing.T) {
	// On MySQL (NO_BACKSLASH_ESCAPES off) and ClickHouse, backslash is an escape
	// character inside string literals. A value ending in a lone backslash, with
	// only quote-doubling, would render as '\'' — the backslash escapes the
	// closing quote and the string runs on, allowing break-out. The renderer must
	// double backslashes so the literal stays terminated on every dialect.
	defs := []Definition{
		{Name: "a", Type: TypeString, Required: true},
		{Name: "b", Type: TypeString, Required: true},
	}
	// Classic break-out attempt: trailing backslash then a second value that, on a
	// vulnerable escaper, would land outside the first literal.
	raw := map[string]interface{}{"a": `x\`, "b": " OR 1=1 -- "}
	got, err := Bind("WHERE a={{a}} AND b={{b}}", defs, raw, BindOptions{})
	if err != nil {
		t.Fatalf("escaping should not error: %v", err)
	}
	want := `WHERE a='x\\' AND b=' OR 1=1 -- '`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Every literal must be quote-balanced; counting backslash-escaped quotes is
	// what break-out exploits, so also assert no odd run of trailing backslashes
	// precedes a closing quote.
	if strings.Contains(got, `\'`) && !strings.Contains(got, `\\'`) {
		t.Errorf("an unescaped backslash precedes a quote in %q", got)
	}
}

func TestBind_RejectsNulByte(t *testing.T) {
	defs := []Definition{{Name: "a", Type: TypeString, Required: true}}
	if _, err := Bind("{{a}}", defs, map[string]interface{}{"a": "ab\x00cd"}, BindOptions{}); err == nil {
		t.Error("expected rejection of string containing a NUL byte")
	}
	// Also rejected inside a list element.
	listDefs := []Definition{{Name: "xs", Type: TypeList, ElementType: TypeString, Required: true}}
	if _, err := Bind("IN ({{xs}})", listDefs, map[string]interface{}{"xs": []string{"ok", "bad\x00"}}, BindOptions{}); err == nil {
		t.Error("expected rejection of NUL byte in list element")
	}
}

func TestBind_RepeatedPlaceholder(t *testing.T) {
	defs := []Definition{{Name: "id", Type: TypeInteger, Required: true}}
	got, err := Bind("a={{id}} OR b={{id}}", defs, map[string]interface{}{"id": 7}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "a=7 OR b=7" {
		t.Errorf("got %q", got)
	}
}

func TestBind_DefaultAndRequired(t *testing.T) {
	defs := []Definition{
		{Name: "status", Type: TypeString, Default: "open"},
		{Name: "owner", Type: TypeString, Required: true},
	}
	// Default applied; required missing -> error.
	_, err := Bind("{{status}} {{owner}}", defs, map[string]interface{}{}, BindOptions{})
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected missing-required error mentioning owner, got %v", err)
	}
	got, err := Bind("{{status}}", []Definition{defs[0]}, map[string]interface{}{}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "'open'" {
		t.Errorf("default not applied: got %q", got)
	}
}

func TestBind_EnumConstraint(t *testing.T) {
	defs := []Definition{{Name: "period", Type: TypeEnum, Options: []string{"day", "week", "month"}, Required: true}}
	if _, err := Bind("{{period}}", defs, map[string]interface{}{"period": "year"}, BindOptions{}); err == nil {
		t.Error("expected enum rejection for 'year'")
	}
	got, err := Bind("{{period}}", defs, map[string]interface{}{"period": "week"}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "'week'" {
		t.Errorf("got %q", got)
	}
}

func TestBind_ListForInClause(t *testing.T) {
	defs := []Definition{{Name: "ids", Type: TypeList, ElementType: TypeInteger, Required: true}}
	got, err := Bind("WHERE id IN ({{ids}})", defs, map[string]interface{}{"ids": []interface{}{1, 2, 3}}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "WHERE id IN (1, 2, 3)" {
		t.Errorf("got %q", got)
	}

	// String list elements are individually quoted/escaped.
	defs2 := []Definition{{Name: "tags", Type: TypeList, ElementType: TypeString, Required: true}}
	got2, err := Bind("IN ({{tags}})", defs2, map[string]interface{}{"tags": []string{"a", "b'c"}}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "IN ('a', 'b''c')" {
		t.Errorf("got %q", got2)
	}
}

func TestBind_Identifier(t *testing.T) {
	defs := []Definition{{Name: "col", Type: TypeIdentifier, Required: true}}
	got, err := Bind("ORDER BY {{col}}", defs, map[string]interface{}{"col": "created_at"}, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "ORDER BY created_at" {
		t.Errorf("got %q", got)
	}
	// Dotted qualifier allowed.
	if _, err := Bind("{{col}}", defs, map[string]interface{}{"col": "public.users.id"}, BindOptions{}); err != nil {
		t.Errorf("dotted identifier should be valid: %v", err)
	}
	// Injection in an identifier position is rejected (cannot be safely quoted).
	for _, bad := range []string{"id; DROP TABLE x", "id--", "1bad", "a'b", "a b"} {
		if _, err := Bind("{{col}}", defs, map[string]interface{}{"col": bad}, BindOptions{}); err == nil {
			t.Errorf("expected rejection of identifier %q", bad)
		}
	}
}

func TestBind_DateAndTimestamp(t *testing.T) {
	defs := []Definition{
		{Name: "d", Type: TypeDate, Required: true},
		{Name: "ts", Type: TypeTimestamp, Required: true},
	}
	raw := map[string]interface{}{
		"d":  "2026-01-15",
		"ts": time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	got, err := Bind("d={{d}} ts={{ts}}", defs, raw, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "d='2026-01-15' ts='2026-01-15 10:30:00'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBind_NumericRange(t *testing.T) {
	defs := []Definition{{Name: "n", Type: TypeInteger, Min: Float(0), Max: Float(100), Required: true}}
	if _, err := Bind("{{n}}", defs, map[string]interface{}{"n": 150}, BindOptions{}); err == nil {
		t.Error("expected range rejection for 150")
	}
	if _, err := Bind("{{n}}", defs, map[string]interface{}{"n": -1}, BindOptions{}); err == nil {
		t.Error("expected range rejection for -1")
	}
	if _, err := Bind("{{n}}", defs, map[string]interface{}{"n": 50}, BindOptions{}); err != nil {
		t.Errorf("50 should be in range: %v", err)
	}
}

func TestBind_UndefinedPlaceholder(t *testing.T) {
	defs := []Definition{{Name: "a", Type: TypeString, Required: true}}
	raw := map[string]interface{}{"a": "x"}
	// Fail closed by default.
	if _, err := Bind("{{a}} {{b}}", defs, raw, BindOptions{}); err == nil {
		t.Error("expected error for undefined placeholder b")
	}
	// Allowed when opted in.
	got, err := Bind("{{a}} {{b}}", defs, raw, BindOptions{AllowUndefined: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "'x' {{b}}" {
		t.Errorf("got %q", got)
	}
}

func TestBind_OptionalMissing(t *testing.T) {
	defs := []Definition{{Name: "a", Type: TypeString}} // optional, no default
	// Default behavior: optional+unsupplied placeholder is an error.
	if _, err := Bind("{{a}}", defs, map[string]interface{}{}, BindOptions{}); err == nil {
		t.Error("expected error for unsupplied optional without NullForMissing")
	}
	got, err := Bind("x={{a}}", defs, map[string]interface{}{}, BindOptions{NullForMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "x=NULL" {
		t.Errorf("got %q", got)
	}
}

func TestPlaceholders(t *testing.T) {
	got := Placeholders("SELECT {{a}}, {{ b }} FROM t WHERE x={{a}} AND y={{c}}")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBind_IntegerRejectsNonInteger(t *testing.T) {
	defs := []Definition{{Name: "n", Type: TypeInteger, Required: true}}
	if _, err := Bind("{{n}}", defs, map[string]interface{}{"n": 1.5}, BindOptions{}); err == nil {
		t.Error("expected rejection of non-integer float for integer param")
	}
	if _, err := Bind("{{n}}", defs, map[string]interface{}{"n": "notnum"}, BindOptions{}); err == nil {
		t.Error("expected rejection of non-numeric string for integer param")
	}
}
