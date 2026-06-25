package database

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode"

	"github.com/sirupsen/logrus"
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
// The keyword set spans every SQL engine we support (PostgreSQL, MySQL/TiDB,
// ClickHouse, SQLite); a row-returning keyword that is meaningless for a given
// engine simply never appears in that engine's scripts.
func statementReturnsRows(stmt string) bool {
	switch leadingKeyword(stmt) {
	case "SELECT", "WITH", "VALUES", "TABLE", "SHOW", "EXPLAIN",
		"FETCH", "CALL", "DESCRIBE", "DESC", "PRAGMA":
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

// scriptStatements decides whether a query should be executed as a
// multi-statement script and, if so, returns its individual statements. A query
// qualifies when it carries no bound parameters — args cannot be distributed
// across split statements, and multi-statement execution relies on the simple
// query protocol — and it splits into more than one statement (e.g.
// "BEGIN; ...; COMMIT;" or several DML statements separated by semicolons).
//
// This is engine-agnostic: every SQL driver we support (PostgreSQL, MySQL/TiDB,
// ClickHouse, SQLite) routes through it before its single-statement path.
func scriptStatements(query string, args []interface{}) ([]string, bool) {
	if len(args) > 0 {
		return nil, false
	}
	stmts := splitSQLStatements(query)
	if len(stmts) <= 1 {
		return nil, false
	}
	return stmts, true
}

// executeSQLScript runs a multi-statement SQL script on a single pinned backend
// connection so that client-side transaction control (BEGIN/COMMIT, SAVEPOINT,
// SET LOCAL) and any data-modifying CTE side effects all share one session. The
// result set of the LAST row-returning statement is returned to the caller,
// mirroring how psql/mysql surface a script's final result; affected-row counts
// from the non-row-returning statements are summed into Affected.
//
// Pagination/limit rewriting is deliberately skipped: wrapping a script — or a
// WITH that contains a data-modifying statement — in a COUNT(*) subquery is not
// valid SQL, which is exactly why these queries failed on the single-statement
// path. The configured limit is still honored as a cap on the rows materialized
// from the final result set.
//
// It works across every database/sql driver we use; per-engine value handling is
// uniform (raw []byte is decoded to string, then NormalizeValue is applied),
// matching each driver's own executeSelect.
func executeSQLScript(ctx context.Context, db *sql.DB, logger *logrus.Logger, statements []string, opts *QueryOptions) (*QueryResult, error) {
	start := time.Now()

	// Pin one connection for the whole script. database/sql would otherwise be
	// free to hand each statement to a different pooled backend, which breaks
	// BEGIN/COMMIT and any cross-statement session state.
	conn, err := db.Conn(ctx)
	if err != nil {
		return &QueryResult{Error: err, Duration: time.Since(start)}, err
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil && logger != nil {
			logger.WithError(cerr).Error("Failed to close pinned connection for script execution")
		}
	}()

	limit := 0
	if opts != nil {
		limit = opts.Limit
	}

	var (
		result        *QueryResult
		totalAffected int64
	)

	for _, stmt := range statements {
		if statementReturnsRows(stmt) {
			res, qerr := runScriptStatementQuery(ctx, conn, logger, stmt, limit)
			if qerr != nil {
				return &QueryResult{Error: qerr, Duration: time.Since(start)}, qerr
			}
			result = res
		} else {
			res, eerr := conn.ExecContext(ctx, stmt)
			if eerr != nil {
				return &QueryResult{Error: eerr, Duration: time.Since(start)}, eerr
			}
			if affected, aerr := res.RowsAffected(); aerr == nil {
				totalAffected += affected
			}
		}
	}

	if result == nil {
		// The script produced no result set (pure DDL/DML); report affected rows.
		result = &QueryResult{Columns: []string{}, Rows: make([][]interface{}, 0)}
	}
	if result.Affected == 0 {
		result.Affected = totalAffected
	}
	result.Duration = time.Since(start)

	// Multi-statement scripts are never inline-editable: the final result set is
	// not a straightforward single-table projection.
	metadata := newEditableMetadata(result.Columns)
	metadata.Reason = "Editing is not available for multi-statement scripts"
	result.Editable = metadata

	return result, nil
}

// runScriptStatementQuery executes one row-returning statement of a script on
// the pinned connection and reads its result set, capping the materialized rows
// at limit (0 means no cap) and reporting HasMore when more rows were available.
func runScriptStatementQuery(ctx context.Context, conn *sql.Conn, logger *logrus.Logger, query string, limit int) (*QueryResult, error) {
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && logger != nil {
			logger.WithError(cerr).Error("Failed to close rows")
		}
	}()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := &QueryResult{
		Columns: columns,
		Rows:    make([][]interface{}, 0),
	}

	for rows.Next() {
		if limit > 0 && len(result.Rows) >= limit {
			// We already have a full page; one more row means there is more.
			result.HasMore = true
			break
		}

		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}
		for i := range values {
			values[i] = NormalizeValue(values[i])
		}

		result.Rows = append(result.Rows, values)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	result.RowCount = int64(len(result.Rows))
	result.PagedRows = result.RowCount
	if !result.HasMore {
		result.TotalRows = result.RowCount
	}

	return result, nil
}
