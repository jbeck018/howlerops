//go:build duckdb

// Package duckstage backs notebook cross-cell composition with an embedded,
// in-memory DuckDB instance. Each SQL cell's result is registered as a named
// table (its handle); a downstream cell that references that handle then runs
// its query against this instance, so results from different connections —
// Postgres, MySQL, SQLite, anything — compose in one place. This is the
// DuckDB-UI / marimo model.
//
// One Stager is created per open notebook and kept warm for the session so
// reactive partial re-runs reuse already-staged upstream tables. The package is
// Wails-free and unit-testable under `-tags duckdb`.
package duckstage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/jbeck018/howlerops/internal/notebook"
)

// Stager is a DuckDB-backed implementation of notebook.Stager.
type Stager struct {
	mu     sync.Mutex
	db     *sql.DB
	staged map[string]bool // staged table names, for Reset
}

// New opens a fresh in-memory DuckDB instance for a notebook session.
func New() (*Stager, error) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		return nil, fmt.Errorf("duckstage: open duckdb: %w", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("duckstage: ping duckdb: %w", err)
	}
	return &Stager{db: db, staged: map[string]bool{}}, nil
}

// Available reports that composition is supported.
func (s *Stager) Available() bool { return s != nil && s.db != nil }

// Reset drops every staged table, starting a full run from a clean slate.
func (s *Stager) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for table := range s.staged {
		if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS `+quoteIdent(table)); err != nil {
			return fmt.Errorf("duckstage: drop %q: %w", table, err)
		}
	}
	s.staged = map[string]bool{}
	return nil
}

// Stage registers a cell's result as a replaceable named table.
func (s *Stager) Stage(ctx context.Context, table string, res *notebook.QueryResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if res == nil {
		res = &notebook.QueryResult{}
	}

	create := buildCreate(table, res)
	if _, err := s.db.ExecContext(ctx, create); err != nil {
		return fmt.Errorf("duckstage: create %q: %w", table, err)
	}
	s.staged[table] = true

	if len(res.Rows) == 0 || len(res.Columns) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("duckstage: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back unless committed

	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(res.Columns)), ", ")
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`INSERT INTO %s VALUES (%s)`, quoteIdent(table), placeholders))
	if err != nil {
		return fmt.Errorf("duckstage: prepare insert %q: %w", table, err)
	}
	defer stmt.Close()

	args := make([]any, len(res.Columns))
	for _, row := range res.Rows {
		for i, col := range res.Columns {
			args[i] = toDriverValue(row[col])
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("duckstage: insert into %q: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("duckstage: commit %q: %w", table, err)
	}
	return nil
}

// Query runs SQL that references staged tables and returns the result.
func (s *Stager) Query(ctx context.Context, query string) (*notebook.QueryResult, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("duckstage: query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("duckstage: columns: %w", err)
	}

	out := &notebook.QueryResult{Columns: columns}
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("duckstage: scan: %w", err)
		}
		m := make(map[string]any, len(columns))
		for i, col := range columns {
			m[col] = normalizeValue(values[i])
		}
		out.Rows = append(out.Rows, m)
		out.RowCount++
	}
	return out, rows.Err()
}

// Close releases the DuckDB instance.
func (s *Stager) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// buildCreate builds a CREATE OR REPLACE TABLE statement whose column types are
// inferred from the first non-null value in each column (defaulting to VARCHAR).
func buildCreate(table string, res *notebook.QueryResult) string {
	if len(res.Columns) == 0 {
		// A zero-column result still gets a placeholder table so references don't
		// dangle; DuckDB requires at least one column.
		return fmt.Sprintf(`CREATE OR REPLACE TABLE %s (_empty BOOLEAN)`, quoteIdent(table))
	}
	cols := make([]string, len(res.Columns))
	for i, col := range res.Columns {
		cols[i] = fmt.Sprintf("%s %s", quoteIdent(col), inferType(res, col))
	}
	return fmt.Sprintf(`CREATE OR REPLACE TABLE %s (%s)`, quoteIdent(table), strings.Join(cols, ", "))
}

// inferType picks a DuckDB column type from the first non-null value found.
func inferType(res *notebook.QueryResult, col string) string {
	for _, row := range res.Rows {
		v := row[col]
		if v == nil {
			continue
		}
		switch v.(type) {
		case bool:
			return "BOOLEAN"
		case int, int8, int16, int32, int64, *big.Int:
			return "BIGINT"
		case float32, float64:
			return "DOUBLE"
		case time.Time:
			return "TIMESTAMP"
		case string, []byte:
			return "VARCHAR"
		default:
			return "VARCHAR"
		}
	}
	return "VARCHAR"
}

// toDriverValue converts an engine value into something the DuckDB driver
// accepts, JSON-encoding anything exotic so it lands in a VARCHAR column.
func toDriverValue(v any) any {
	switch t := v.(type) {
	case nil, bool, string, int, int8, int16, int32, int64, float32, float64, time.Time:
		return t
	case []byte:
		return string(t)
	case *big.Int:
		if t == nil {
			return nil
		}
		if t.IsInt64() {
			return t.Int64()
		}
		return t.String()
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// normalizeValue mirrors the federation engine: make DuckDB-specific values
// JSON-friendly so staged results match direct query results.
func normalizeValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case *big.Int:
		if x == nil {
			return nil
		}
		if x.IsInt64() {
			return x.Int64()
		}
		return x.String()
	case big.Int:
		if x.IsInt64() {
			return x.Int64()
		}
		return x.String()
	default:
		return v
	}
}

// quoteIdent double-quotes a SQL identifier, doubling embedded quotes.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
