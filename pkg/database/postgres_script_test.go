package database

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// newScriptTestPostgres wires a PostgresDatabase around a mock *sql.DB so the
// multi-statement script path can be exercised without a live server.
func newScriptTestPostgres(t *testing.T) (*PostgresDatabase, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	p := &PostgresDatabase{
		pool:           &ConnectionPool{db: db},
		logger:         logger,
		structureCache: newTableStructureCache(time.Minute),
	}
	return p, mock
}

// TestExecuteScript_ReturnsFinalSelect covers the reported case: a BEGIN/COMMIT
// transaction whose body is a WITH that performs INSERTs and ends in a SELECT.
// The single-statement path discarded the SELECT's rows; the script path must
// surface them.
func TestExecuteScript_ReturnsFinalSelect(t *testing.T) {
	p, mock := newScriptTestPostgres(t)

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("definitions_created").
		WillReturnRows(sqlmock.NewRows([]string{"definitions_created"}).AddRow(int64(6)))
	mock.ExpectExec("COMMIT").WillReturnResult(sqlmock.NewResult(0, 0))

	script := `BEGIN;
WITH ins_def AS (
  INSERT INTO orchestration_definitions (id) VALUES (gen_random_uuid()) RETURNING id
)
SELECT count(*) AS definitions_created FROM ins_def;
COMMIT;`

	res, err := p.ExecuteWithOptions(context.Background(), script, &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, []string{"definitions_created"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.Equal(t, int64(6), res.Rows[0][0])
	require.Equal(t, int64(1), res.RowCount)
	require.NotNil(t, res.Editable)
	require.False(t, res.Editable.Enabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestExecuteScript_PureDMLReportsAffected verifies a script with no
// row-returning statement reports the summed affected count and an empty result.
func TestExecuteScript_PureDMLReportsAffected(t *testing.T) {
	p, mock := newScriptTestPostgres(t)

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE t").WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec("COMMIT").WillReturnResult(sqlmock.NewResult(0, 0))

	res, err := p.ExecuteWithOptions(context.Background(), "BEGIN; UPDATE t SET a = 1; COMMIT;", &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, int64(3), res.Affected)
	require.Empty(t, res.Columns)
	require.Len(t, res.Rows, 0)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestExecuteScript_HonorsLimit verifies the configured limit caps the final
// result set and sets HasMore when more rows are available.
func TestExecuteScript_HonorsLimit(t *testing.T) {
	p, mock := newScriptTestPostgres(t)

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM t").WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)).AddRow(int64(3)),
	)
	mock.ExpectExec("COMMIT").WillReturnResult(sqlmock.NewResult(0, 0))

	res, err := p.ExecuteWithOptions(context.Background(), "BEGIN; SELECT id FROM t; COMMIT;", &QueryOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, res.Rows, 2)
	require.True(t, res.HasMore)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestExecuteScript_PropagatesError ensures a mid-script failure is surfaced and
// later statements are not executed.
func TestExecuteScript_PropagatesError(t *testing.T) {
	p, mock := newScriptTestPostgres(t)

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("FROM missing").WillReturnError(io.ErrUnexpectedEOF)
	// COMMIT must NOT be expected: execution stops at the failing statement.

	_, err := p.ExecuteWithOptions(context.Background(), "BEGIN; SELECT * FROM missing; COMMIT;", &QueryOptions{Limit: 1000})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestExecuteWithOptions_SingleStatementUnaffected confirms a normal single
// statement still flows through the standard (paginated) SELECT path and is not
// treated as a script.
func TestExecuteWithOptions_SingleStatementUnaffected(t *testing.T) {
	p, mock := newScriptTestPostgres(t)

	// Single SELECT: the standard path issues a COUNT(*) for pagination and then
	// the limited query. No pinned-connection script behavior.
	mock.ExpectQuery("count_subquery").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT id FROM t").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(42)))

	res, err := p.ExecuteWithOptions(context.Background(), "SELECT id FROM t;", &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, []string{"id"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.Equal(t, int64(42), res.Rows[0][0])
	require.NoError(t, mock.ExpectationsWereMet())
}
