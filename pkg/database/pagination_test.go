package database_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/jbeck018/howlerops/pkg/database"
)

// TestSkipCountPaginationHasMore guards the pagination fix: when the COUNT(*) is
// skipped (SkipCount, used for pages past the first), TotalRows is unknown so
// HasMore must be inferred from page fullness rather than the total — otherwise
// next-page navigation breaks after page 1.
func TestSkipCountPaginationHasMore(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	cfg := database.ConnectionConfig{
		Type:           database.SQLite,
		Database:       fmt.Sprintf("file:pagetest_%d?mode=memory", time.Now().UnixNano()),
		MaxConnections: 25,
		MaxIdleConns:   5,
		Parameters:     map[string]string{"cache": "shared"},
	}

	db, err := database.NewSQLiteDatabase(cfg, logger)
	require.NoError(t, err)
	defer func() { _ = db.Disconnect() }()

	ctx := context.Background()
	_, err = db.Execute(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY)")
	require.NoError(t, err)
	for i := 0; i < 25; i++ {
		_, err = db.Execute(ctx, fmt.Sprintf("INSERT INTO t (id) VALUES (%d)", i))
		require.NoError(t, err)
	}

	const limit = 10
	q := "SELECT * FROM t ORDER BY id"

	// Page 1: count not skipped — exact total, more pages available.
	r1, err := db.ExecuteWithOptions(ctx, q, &database.QueryOptions{Limit: limit, Offset: 0})
	require.NoError(t, err)
	require.Equal(t, int64(25), r1.TotalRows)
	require.True(t, r1.HasMore, "page 1 of 25 rows should have more")
	require.Len(t, r1.Rows, limit)

	// Page 2: count skipped, full page — HasMore must still be true.
	r2, err := db.ExecuteWithOptions(ctx, q, &database.QueryOptions{Limit: limit, Offset: limit, SkipCount: true})
	require.NoError(t, err)
	require.True(t, r2.HasMore, "a full page with skipped count must report HasMore (the regression)")
	require.Len(t, r2.Rows, limit)

	// Page 3: count skipped, partial page (5 rows) — HasMore false.
	r3, err := db.ExecuteWithOptions(ctx, q, &database.QueryOptions{Limit: limit, Offset: 2 * limit, SkipCount: true})
	require.NoError(t, err)
	require.False(t, r3.HasMore, "a partial last page should report no more")
	require.Len(t, r3.Rows, 5)
}
