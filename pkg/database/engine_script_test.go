package database

import (
	"context"
	"io"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// These tests confirm that every SQL engine routes a multi-statement script
// through the shared executeSQLScript path (returning the final statement's
// result set) rather than the single-statement path that discards it. The
// shared logic itself is covered in depth by postgres_script_test.go and
// sqlscript_test.go; here we only assert the per-engine wiring.

func discardLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestMySQLExecuteScriptRouting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	m := &MySQLDatabase{pool: &ConnectionPool{db: db}, logger: discardLogger()}

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("total").
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(int64(5)))
	mock.ExpectExec("COMMIT").WillReturnResult(sqlmock.NewResult(0, 0))

	script := "BEGIN; INSERT INTO t (a) VALUES (1); SELECT count(*) AS total FROM t; COMMIT;"
	res, err := m.ExecuteWithOptions(context.Background(), script, &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, []string{"total"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.Equal(t, int64(5), res.Rows[0][0])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClickHouseExecuteScriptRouting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	c := &ClickHouseDatabase{pool: &ConnectionPool{db: db}, logger: discardLogger()}

	// ClickHouse has no transactions, but a multi-statement batch (INSERT then a
	// final SELECT) must still surface the SELECT's rows.
	mock.ExpectExec("INSERT INTO events").WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectQuery("n").
		WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(int64(2)))

	script := "INSERT INTO events (id) VALUES (1), (2); SELECT count() AS n FROM events;"
	res, err := c.ExecuteWithOptions(context.Background(), script, &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, []string{"n"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.Equal(t, int64(2), res.Rows[0][0])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSQLiteExecuteScriptRouting(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	s := &SQLiteDatabase{pool: &ConnectionPool{db: db}, logger: discardLogger()}

	mock.ExpectExec("BEGIN").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO t").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("id").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec("COMMIT").WillReturnResult(sqlmock.NewResult(0, 0))

	script := "BEGIN; INSERT INTO t (a) VALUES (1); SELECT id FROM t; COMMIT;"
	res, err := s.ExecuteWithOptions(context.Background(), script, &QueryOptions{Limit: 1000})
	require.NoError(t, err)
	require.Equal(t, []string{"id"}, res.Columns)
	require.Len(t, res.Rows, 1)
	require.Equal(t, int64(1), res.Rows[0][0])
	require.NoError(t, mock.ExpectationsWereMet())
}
