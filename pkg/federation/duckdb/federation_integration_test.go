//go:build duckdb

package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/sirupsen/logrus"
)

// seedSQLite creates a sqlite file with a single table populated via a raw
// DuckDB connection (read-write attach), independent of the federation Engine.
func seedSQLite(t *testing.T, path, ddl string, inserts ...string) {
	t.Helper()
	raw, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatalf("open raw duckdb: %v", err)
	}
	defer raw.Close()
	ctx := context.Background()
	if _, err := raw.ExecContext(ctx, "INSTALL sqlite_scanner"); err != nil {
		t.Fatalf("install sqlite: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "LOAD sqlite_scanner"); err != nil {
		t.Fatalf("load sqlite: %v", err)
	}
	if _, err := raw.ExecContext(ctx, fmt.Sprintf("ATTACH '%s' AS s (TYPE sqlite)", path)); err != nil {
		t.Fatalf("attach rw: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "USE s"); err != nil {
		t.Fatalf("use s: %v", err)
	}
	if _, err := raw.ExecContext(ctx, ddl); err != nil {
		t.Fatalf("ddl: %v", err)
	}
	for _, ins := range inserts {
		if _, err := raw.ExecContext(ctx, ins); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
}

// TestEngine_FederatedSqliteJoin proves the real federation path: ATTACH two
// separate (sqlite) backends READ_ONLY through the Engine, then run a
// cross-database JOIN+aggregation via the validated ExecuteQuery. postgres/mysql
// use the identical ATTACH ... (TYPE ...) mechanism.
func TestEngine_FederatedSqliteJoin(t *testing.T) {
	dir := t.TempDir()
	dbA := filepath.Join(dir, "a.db")
	dbB := filepath.Join(dir, "b.db")
	seedSQLite(t, dbA, "CREATE TABLE users (id INTEGER, name VARCHAR)",
		"INSERT INTO users VALUES (1,'ada'),(2,'linus')")
	seedSQLite(t, dbB, "CREATE TABLE orders (user_id INTEGER, amount INTEGER)",
		"INSERT INTO orders VALUES (1,100),(1,50),(2,999)")

	logger := logrus.New()
	logger.SetLevel(logrus.PanicLevel)
	e := NewEngine(logger, nil)
	ctx := context.Background()
	if err := e.Initialize(ctx); err != nil {
		t.Fatalf("init engine: %v", err)
	}
	defer e.Close()

	if err := e.Attach(ctx, "sessA", "db_a", dbA, "sqlite", "fpa"); err != nil {
		t.Fatalf("attach A: %v", err)
	}
	if err := e.Attach(ctx, "sessB", "db_b", dbB, "sqlite", "fpb"); err != nil {
		t.Fatalf("attach B: %v", err)
	}

	// Idempotent re-attach with same fingerprint is a no-op (no error).
	if err := e.Attach(ctx, "sessA", "db_a", dbA, "sqlite", "fpa"); err != nil {
		t.Fatalf("re-attach A: %v", err)
	}

	res, err := e.ExecuteQuery(ctx, `
		SELECT u.name, SUM(o.amount) AS total
		FROM "db_a".users u
		JOIN "db_b".orders o ON o.user_id = u.id
		GROUP BY u.name
		ORDER BY u.name`, 0)
	if err != nil {
		t.Fatalf("federated query: %v", err)
	}

	if res.RowCount != 2 {
		t.Fatalf("expected 2 rows, got %d", res.RowCount)
	}
	got := map[string]int64{}
	for _, row := range res.Rows {
		name := fmt.Sprintf("%v", row[0])
		// SUM over integers comes back as DuckDB HUGEINT (int128); parse from its
		// string form so the assertion is value-based, not type-specific.
		n, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprintf("%v", row[1])), 10, 64)
		if err != nil {
			t.Fatalf("unparseable total %v (%T): %v", row[1], row[1], err)
		}
		got[name] = n
	}
	if got["ada"] != 150 || got["linus"] != 999 {
		t.Fatalf("unexpected federated result: %+v", got)
	}

	// Detach removes the attachment.
	if err := e.Detach(ctx, "sessA"); err != nil {
		t.Fatalf("detach: %v", err)
	}
}
