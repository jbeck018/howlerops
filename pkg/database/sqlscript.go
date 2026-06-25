package database

import (
	"strings"
	"unicode"
)

// splitSQLStatements splits a SQL script into its individual statements on
// top-level semicolons. Semicolons that appear inside single-quoted string
// literals, double-quoted identifiers, dollar-quoted strings ($$ ... $$ or
// $tag$ ... $tag$), line comments (-- ...) and block comments (/* ... */) are
// not treated as statement boundaries.
//
// Each returned statement is trimmed of surrounding whitespace and has its
// terminating semicolon removed. Statements that contain only whitespace and/or
// comments are dropped, so a trailing ";" or a comment-only tail does not yield
// an empty statement. Comments that are attached to a real statement are
// preserved verbatim.
func splitSQLStatements(script string) []string {
	runes := []rune(script)
	n := len(runes)

	statements := make([]string, 0, 4)
	var buf strings.Builder

	flush := func() {
		stmt := strings.TrimSpace(buf.String())
		if !isBlankOrCommentOnly(stmt) {
			statements = append(statements, stmt)
		}
		buf.Reset()
	}

	i := 0
	for i < n {
		c := runes[i]
		switch {
		case c == ';':
			flush()
			i++
		case c == '\'' || c == '"':
			i = copyQuoted(runes, i, c, &buf)
		case c == '$':
			if tagLen := dollarQuoteLen(runes, i); tagLen > 0 {
				i = copyDollarQuoted(runes, i, tagLen, &buf)
			} else {
				buf.WriteRune(c)
				i++
			}
		case c == '-' && i+1 < n && runes[i+1] == '-':
			i = copyLineComment(runes, i, &buf)
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i = copyBlockComment(runes, i, &buf)
		default:
			buf.WriteRune(c)
			i++
		}
	}
	flush()

	return statements
}

// statementReturnsRows reports whether a single SQL statement is expected to
// produce a result set (and therefore should be run via Query rather than Exec).
func statementReturnsRows(stmt string) bool {
	switch leadingKeyword(stmt) {
	case "SELECT", "WITH", "VALUES", "TABLE", "SHOW", "EXPLAIN", "FETCH", "CALL":
		return true
	}
	// INSERT/UPDATE/DELETE/MERGE ... RETURNING also produce a result set.
	return hasReturningClause(stmt)
}

// leadingKeyword returns the first SQL keyword of a statement, upper-cased,
// skipping any leading whitespace and comments. It returns "" when the
// statement does not begin with an alphabetic keyword.
func leadingKeyword(stmt string) string {
	runes := []rune(stmt)
	n := len(runes)
	i := 0
	for i < n {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '-' && i+1 < n && runes[i+1] == '-':
			i = copyLineComment(runes, i, nil)
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i = copyBlockComment(runes, i, nil)
		case unicode.IsLetter(c) || c == '_':
			start := i
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			return strings.ToUpper(string(runes[start:i]))
		default:
			return ""
		}
	}
	return ""
}

// hasReturningClause reports whether a statement contains a RETURNING keyword
// outside of string literals, quoted identifiers, dollar-quoted strings and
// comments.
func hasReturningClause(stmt string) bool {
	runes := []rune(stmt)
	n := len(runes)
	i := 0
	for i < n {
		c := runes[i]
		switch {
		case c == '\'' || c == '"':
			i = copyQuoted(runes, i, c, nil)
		case c == '$':
			if tagLen := dollarQuoteLen(runes, i); tagLen > 0 {
				i = copyDollarQuoted(runes, i, tagLen, nil)
			} else {
				i++
			}
		case c == '-' && i+1 < n && runes[i+1] == '-':
			i = copyLineComment(runes, i, nil)
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i = copyBlockComment(runes, i, nil)
		case unicode.IsLetter(c) || c == '_':
			start := i
			for i < n && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_') {
				i++
			}
			if strings.EqualFold(string(runes[start:i]), "RETURNING") {
				return true
			}
		default:
			i++
		}
	}
	return false
}

// isBlankOrCommentOnly reports whether a statement contains no executable SQL,
// i.e. only whitespace and/or comments.
func isBlankOrCommentOnly(stmt string) bool {
	runes := []rune(stmt)
	n := len(runes)
	i := 0
	for i < n {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '-' && i+1 < n && runes[i+1] == '-':
			i = copyLineComment(runes, i, nil)
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i = copyBlockComment(runes, i, nil)
		default:
			return false
		}
	}
	return true
}

// dollarQuoteLen returns the rune length of a PostgreSQL dollar-quote opening
// tag starting at runes[start] (which must be '$'), e.g. 2 for "$$" or 6 for
// "$body$". It returns 0 when no valid tag begins at start (for example the
// parameter placeholders $1, $2 are not dollar-quote tags).
func dollarQuoteLen(runes []rune, start int) int {
	n := len(runes)
	i := start + 1
	for i < n {
		c := runes[i]
		if c == '$' {
			return i - start + 1
		}
		// The tag (between the dollar signs) follows identifier rules and may
		// not start with a digit.
		if c == '_' || unicode.IsLetter(c) || (i > start+1 && unicode.IsDigit(c)) {
			i++
			continue
		}
		return 0
	}
	return 0
}

// copyQuoted copies a quoted region beginning at runes[start] (the opening quote
// q, either ' or "). A doubled quote (q q) is an escaped quote, not a
// terminator. When buf is nil the region is skipped without being copied.
// Returns the index just past the closing quote (or n if unterminated).
func copyQuoted(runes []rune, start int, q rune, buf *strings.Builder) int {
	n := len(runes)
	writeRune(buf, runes[start])
	i := start + 1
	for i < n {
		writeRune(buf, runes[i])
		if runes[i] == q {
			if i+1 < n && runes[i+1] == q {
				i++
				writeRune(buf, runes[i])
				i++
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// copyDollarQuoted copies a dollar-quoted string beginning at runes[start] whose
// opening tag has the given rune length. When buf is nil the region is skipped.
// Returns the index just past the matching closing tag (or n if unterminated).
func copyDollarQuoted(runes []rune, start, tagLen int, buf *strings.Builder) int {
	n := len(runes)
	for k := 0; k < tagLen; k++ {
		writeRune(buf, runes[start+k])
	}
	i := start + tagLen
	for i < n {
		if runes[i] == '$' && runesRegionEqual(runes, i, start, tagLen) {
			for k := 0; k < tagLen; k++ {
				writeRune(buf, runes[i+k])
			}
			return i + tagLen
		}
		writeRune(buf, runes[i])
		i++
	}
	return i
}

// copyLineComment copies a -- line comment (including its terminating newline)
// beginning at runes[start]. When buf is nil the comment is skipped. Returns the
// index just past the newline (or n at end of input).
func copyLineComment(runes []rune, start int, buf *strings.Builder) int {
	n := len(runes)
	i := start
	for i < n && runes[i] != '\n' {
		writeRune(buf, runes[i])
		i++
	}
	if i < n {
		writeRune(buf, runes[i]) // include the newline
		i++
	}
	return i
}

// copyBlockComment copies a /* ... */ block comment beginning at runes[start],
// honoring PostgreSQL's nesting rules. When buf is nil the comment is skipped.
// Returns the index just past the closing */ (or n if unterminated).
func copyBlockComment(runes []rune, start int, buf *strings.Builder) int {
	n := len(runes)
	i := start
	depth := 0
	for i < n {
		if runes[i] == '/' && i+1 < n && runes[i+1] == '*' {
			writeRune(buf, runes[i])
			writeRune(buf, runes[i+1])
			i += 2
			depth++
			continue
		}
		if runes[i] == '*' && i+1 < n && runes[i+1] == '/' {
			writeRune(buf, runes[i])
			writeRune(buf, runes[i+1])
			i += 2
			depth--
			if depth == 0 {
				return i
			}
			continue
		}
		writeRune(buf, runes[i])
		i++
	}
	return i
}

// runesRegionEqual reports whether runes[a:a+length] equals runes[b:b+length].
func runesRegionEqual(runes []rune, a, b, length int) bool {
	if a+length > len(runes) || b+length > len(runes) {
		return false
	}
	for k := 0; k < length; k++ {
		if runes[a+k] != runes[b+k] {
			return false
		}
	}
	return true
}

// writeRune writes r to buf when buf is non-nil; otherwise it is a no-op, which
// lets the copy* helpers double as skip helpers.
func writeRune(buf *strings.Builder, r rune) {
	if buf != nil {
		buf.WriteRune(r)
	}
}
