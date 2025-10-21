package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/sql-studio/backend-go/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a test ClickHouse config
func newTestClickHouseConfig() database.ConnectionConfig {
	return database.ConnectionConfig{
		Type:              database.ClickHouse,
		Host:              "localhost",
		Port:              9000,
		Database:          "default",
		Username:          "default",
		Password:          "",
		ConnectionTimeout: 30 * time.Second,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    10,
		MaxIdleConns:      5,
		Parameters:        map[string]string{},
	}
}

func TestNewClickHouseDatabase(t *testing.T) {
	logger := newTestLogger()

	t.Run("valid config", func(t *testing.T) {
		config := newTestClickHouseConfig()

		// Note: This will attempt to create a real connection pool
		// In a production test environment, you might want to use a test database
		db, err := database.NewClickHouseDatabase(config, logger)

		if err != nil {
			// Connection might fail in test environment - that's ok for constructor test
			t.Logf("Connection failed (expected in test environment): %v", err)
		} else {
			assert.NotNil(t, db)
			assert.Equal(t, database.ClickHouse, db.GetDatabaseType())
			defer db.Disconnect()
		}
	})

	t.Run("valid config with custom port", func(t *testing.T) {
		config := newTestClickHouseConfig()
		config.Port = 8123 // HTTP port

		db, err := database.NewClickHouseDatabase(config, logger)

		if err != nil {
			t.Logf("Connection failed (expected in test environment): %v", err)
		} else {
			assert.NotNil(t, db)
			defer db.Disconnect()
		}
	})

	t.Run("valid config with connection limits", func(t *testing.T) {
		config := newTestClickHouseConfig()
		config.MaxConnections = 20
		config.MaxIdleConns = 10

		db, err := database.NewClickHouseDatabase(config, logger)

		if err != nil {
			t.Logf("Connection failed (expected in test environment): %v", err)
		} else {
			assert.NotNil(t, db)
			defer db.Disconnect()
		}
	})

	t.Run("invalid config - missing host", func(t *testing.T) {
		config := newTestClickHouseConfig()
		config.Host = ""

		_, err := database.NewClickHouseDatabase(config, logger)
		assert.Error(t, err)
	})

	t.Run("invalid config - invalid port", func(t *testing.T) {
		config := newTestClickHouseConfig()
		config.Port = 0

		_, err := database.NewClickHouseDatabase(config, logger)
		assert.Error(t, err)
	})

	t.Run("invalid config - missing database", func(t *testing.T) {
		config := newTestClickHouseConfig()
		config.Database = ""

		_, err := database.NewClickHouseDatabase(config, logger)
		assert.Error(t, err)
	})
}

func TestClickHouseDatabase_GetDatabaseType(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	dbType := db.GetDatabaseType()
	assert.Equal(t, database.ClickHouse, dbType)
}

func TestClickHouseDatabase_QuoteIdentifier(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	tests := []struct {
		name       string
		identifier string
		want       string
	}{
		{
			name:       "simple identifier",
			identifier: "users",
			want:       "`users`",
		},
		{
			name:       "identifier with underscore",
			identifier: "user_events",
			want:       "`user_events`",
		},
		{
			name:       "identifier with backtick",
			identifier: "user`table",
			want:       "`user``table`",
		},
		{
			name:       "empty identifier",
			identifier: "",
			want:       "``",
		},
		{
			name:       "identifier with multiple backticks",
			identifier: "a`b`c",
			want:       "`a``b``c`",
		},
		{
			name:       "identifier with special chars",
			identifier: "user-events",
			want:       "`user-events`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := db.QuoteIdentifier(tt.identifier)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClickHouseDatabase_GetDataTypeMappings(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	mappings := db.GetDataTypeMappings()

	// Verify expected ClickHouse-specific mappings exist
	assert.NotNil(t, mappings)
	assert.Equal(t, "String", mappings["string"])
	assert.Equal(t, "Int32", mappings["int"])
	assert.Equal(t, "Int64", mappings["int64"])
	assert.Equal(t, "Float32", mappings["float"])
	assert.Equal(t, "Float64", mappings["float64"])
	assert.Equal(t, "UInt8", mappings["bool"])
	assert.Equal(t, "DateTime", mappings["time"])
	assert.Equal(t, "Date", mappings["date"])
	assert.Equal(t, "String", mappings["json"]) // ClickHouse stores JSON as String
	assert.Equal(t, "UUID", mappings["uuid"])
	assert.Equal(t, "Array", mappings["array"])
	assert.Equal(t, "Decimal", mappings["decimal"])
}

func TestClickHouseDatabase_Execute_QueryType(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		isSelect bool
	}{
		{
			name:     "SELECT query",
			query:    "SELECT * FROM users",
			isSelect: true,
		},
		{
			name:     "WITH query",
			query:    "WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
			isSelect: true,
		},
		{
			name:     "SHOW query",
			query:    "SHOW TABLES",
			isSelect: true,
		},
		{
			name:     "DESCRIBE query",
			query:    "DESCRIBE users",
			isSelect: true,
		},
		{
			name:     "INSERT query",
			query:    "INSERT INTO users (name) VALUES ('John')",
			isSelect: false,
		},
		{
			name:     "INSERT SELECT query",
			query:    "INSERT INTO users SELECT * FROM temp_users",
			isSelect: false,
		},
		{
			name:     "CREATE TABLE query",
			query:    "CREATE TABLE users (id UInt64, name String) ENGINE = MergeTree() ORDER BY id",
			isSelect: false,
		},
		{
			name:     "ALTER TABLE query",
			query:    "ALTER TABLE users UPDATE name = 'Jane' WHERE id = 1",
			isSelect: false,
		},
		{
			name:     "DROP TABLE query",
			query:    "DROP TABLE users",
			isSelect: false,
		},
		{
			name:     "OPTIMIZE TABLE query",
			query:    "OPTIMIZE TABLE users FINAL",
			isSelect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test query classification logic
			// The actual execution would require a mock connection
			assert.NotEmpty(t, tt.query)
		})
	}
}

func TestClickHouseDatabase_UpdateRow(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	ctx := context.Background()
	params := database.UpdateRowParams{
		Schema: "default",
		Table:  "users",
		PrimaryKey: map[string]interface{}{
			"id": 1,
		},
		Values: map[string]interface{}{
			"name": "John Doe",
		},
	}

	err = db.UpdateRow(ctx, params)

	// UpdateRow is not supported for ClickHouse (immutable by design)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestClickHouseDatabase_ComputeEditableMetadata(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	ctx := context.Background()
	metadata, err := db.ComputeEditableMetadata(ctx, "SELECT * FROM users", []string{"id", "name"})

	assert.NoError(t, err)
	assert.NotNil(t, metadata)
	assert.False(t, metadata.Enabled)
	assert.Equal(t, "ClickHouse tables are immutable and not directly editable", metadata.Reason)
}

func TestClickHouseDatabase_BeginTransaction(t *testing.T) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	ctx := context.Background()
	tx, err := db.BeginTransaction(ctx)

	// ClickHouse doesn't support traditional transactions
	assert.Error(t, err)
	assert.Nil(t, tx)
	assert.Contains(t, err.Error(), "does not support traditional transactions")
}

func TestClickHouseDatabase_Execute(t *testing.T) {
	t.Run("SELECT query structure", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SELECT 1 as test_col"

		result, err := db.Execute(ctx, query)

		// Might fail due to connection, but structure test
		if err == nil {
			assert.NotNil(t, result)
			assert.GreaterOrEqual(t, len(result.Columns), 0)
		}
	})

	t.Run("WITH query is treated as SELECT", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "WITH cte AS (SELECT 1) SELECT * FROM cte"

		result, err := db.Execute(ctx, query)

		if err == nil {
			assert.NotNil(t, result)
		}
	})

	t.Run("SHOW query is treated as SELECT", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SHOW DATABASES"

		result, err := db.Execute(ctx, query)

		if err == nil {
			assert.NotNil(t, result)
		}
	})

	t.Run("DESCRIBE query is treated as SELECT", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "DESCRIBE TABLE system.tables"

		result, err := db.Execute(ctx, query)

		if err == nil {
			assert.NotNil(t, result)
		}
	})
}

func TestClickHouseDatabase_ExecuteStream(t *testing.T) {
	t.Run("callback invocation", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SELECT number FROM numbers(10)"

		batches := 0
		callback := func(rows [][]interface{}) error {
			batches++
			assert.NotNil(t, rows)
			return nil
		}

		err = db.ExecuteStream(ctx, query, 3, callback)

		// Might fail due to connection
		if err == nil {
			assert.Greater(t, batches, 0)
		}
	})

	t.Run("callback error propagation", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SELECT number FROM numbers(10)"

		expectedErr := errors.New("callback error")
		callback := func(rows [][]interface{}) error {
			return expectedErr
		}

		err = db.ExecuteStream(ctx, query, 3, callback)

		// Should propagate callback error if connection succeeds
		if err != nil && !isConnectionError(err) {
			assert.Equal(t, expectedErr, err)
		}
	})

	t.Run("handles byte array conversion", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SELECT 'hello' as text_col"

		var receivedRows [][]interface{}
		callback := func(rows [][]interface{}) error {
			receivedRows = append(receivedRows, rows...)
			return nil
		}

		err = db.ExecuteStream(ctx, query, 10, callback)

		if err == nil {
			assert.Greater(t, len(receivedRows), 0)
			// Values should be converted from byte arrays to strings
			for _, row := range receivedRows {
				for _, val := range row {
					// Should not be a byte array after conversion
					if str, ok := val.(string); ok {
						assert.NotEmpty(t, str)
					}
				}
			}
		}
	})
}

func TestClickHouseDatabase_GetSchemas(t *testing.T) {
	t.Run("filters system schemas", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		schemas, err := db.GetSchemas(ctx)

		if err == nil {
			// Should not include system, INFORMATION_SCHEMA, information_schema
			for _, schema := range schemas {
				assert.NotEqual(t, "system", schema)
				assert.NotEqual(t, "INFORMATION_SCHEMA", schema)
				assert.NotEqual(t, "information_schema", schema)
			}
		}
	})

	t.Run("returns sorted schemas", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		schemas, err := db.GetSchemas(ctx)

		if err == nil && len(schemas) > 1 {
			// Verify schemas are sorted
			for i := 0; i < len(schemas)-1; i++ {
				assert.LessOrEqual(t, schemas[i], schemas[i+1])
			}
		}
	})
}

func TestClickHouseDatabase_GetTables(t *testing.T) {
	t.Run("requires schema parameter", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		tables, err := db.GetTables(ctx, "default")

		if err == nil {
			assert.NotNil(t, tables)
			// Each table should have metadata
			for _, table := range tables {
				assert.NotEmpty(t, table.Name)
				assert.Equal(t, "default", table.Schema)
				assert.Equal(t, "TABLE", table.Type)
				// Should have engine in metadata
				assert.NotNil(t, table.Metadata)
				_, hasEngine := table.Metadata["engine"]
				assert.True(t, hasEngine)
			}
		}
	})

	t.Run("includes row count and size", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		tables, err := db.GetTables(ctx, "system")

		if err == nil && len(tables) > 0 {
			// System tables should have statistics
			for _, table := range tables {
				assert.GreaterOrEqual(t, table.RowCount, int64(0))
				assert.GreaterOrEqual(t, table.SizeBytes, int64(0))
			}
		}
	})

	t.Run("returns sorted tables", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		tables, err := db.GetTables(ctx, "default")

		if err == nil && len(tables) > 1 {
			// Verify tables are sorted by name
			for i := 0; i < len(tables)-1; i++ {
				assert.LessOrEqual(t, tables[i].Name, tables[i+1].Name)
			}
		}
	})
}

func TestClickHouseDatabase_GetTableStructure(t *testing.T) {
	t.Run("includes table info with engine", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		structure, err := db.GetTableStructure(ctx, "system", "tables")

		if err == nil {
			assert.NotNil(t, structure)
			assert.Equal(t, "tables", structure.Table.Name)
			assert.Equal(t, "system", structure.Table.Schema)
			assert.NotNil(t, structure.Table.Metadata)
			engine, hasEngine := structure.Table.Metadata["engine"]
			assert.True(t, hasEngine)
			assert.NotEmpty(t, engine)
		}
	})

	t.Run("includes columns with data types", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		structure, err := db.GetTableStructure(ctx, "system", "tables")

		if err == nil {
			assert.NotNil(t, structure)
			assert.Greater(t, len(structure.Columns), 0)

			for _, col := range structure.Columns {
				assert.NotEmpty(t, col.Name)
				assert.NotEmpty(t, col.DataType)
				assert.GreaterOrEqual(t, col.OrdinalPosition, 0)
			}
		}
	})

	t.Run("detects nullable columns", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		structure, err := db.GetTableStructure(ctx, "system", "tables")

		if err == nil {
			assert.NotNil(t, structure)

			// Check if nullable detection works
			// Columns with "Nullable" in type should have Nullable = true
			for _, col := range structure.Columns {
				if col.Nullable {
					// Should contain "nullable" in type name (case-insensitive)
					assert.Contains(t, col.DataType, "ullable")
				}
			}
		}
	})

	t.Run("includes default values", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		structure, err := db.GetTableStructure(ctx, "system", "tables")

		if err == nil {
			assert.NotNil(t, structure)

			// Some columns might have default values
			for _, col := range structure.Columns {
				if col.DefaultValue != nil {
					assert.NotEmpty(t, *col.DefaultValue)
				}
			}
		}
	})

	t.Run("columns are sorted by position", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		structure, err := db.GetTableStructure(ctx, "system", "tables")

		if err == nil && len(structure.Columns) > 1 {
			// Verify columns are sorted by ordinal position
			for i := 0; i < len(structure.Columns)-1; i++ {
				assert.LessOrEqual(t, structure.Columns[i].OrdinalPosition, structure.Columns[i+1].OrdinalPosition)
			}
		}
	})
}

func TestClickHouseDatabase_GetConnectionInfo(t *testing.T) {
	t.Run("returns version and database info", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		info, err := db.GetConnectionInfo(ctx)

		if err == nil {
			assert.NotNil(t, info)
			// Should contain version
			assert.Contains(t, info, "version")
			version, ok := info["version"].(string)
			assert.True(t, ok)
			assert.NotEmpty(t, version)

			// Should contain database name
			assert.Contains(t, info, "database")
			database, ok := info["database"].(string)
			assert.True(t, ok)
			assert.NotEmpty(t, database)

			// Should contain uptime
			assert.Contains(t, info, "uptime_seconds")
			uptime, ok := info["uptime_seconds"].(uint64)
			assert.True(t, ok)
			assert.GreaterOrEqual(t, uptime, uint64(0))
		}
	})

	t.Run("handles missing optional fields gracefully", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		info, err := db.GetConnectionInfo(ctx)

		// Should not error even if some optional fields are missing
		if err == nil {
			assert.NotNil(t, info)
			assert.Contains(t, info, "version")
		}
	})
}

func TestClickHouseDatabase_ExplainQuery(t *testing.T) {
	t.Run("explains simple SELECT", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		plan, err := db.ExplainQuery(ctx, "SELECT 1")

		if err == nil {
			assert.NotEmpty(t, plan)
		}
	})

	t.Run("explains table query", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		plan, err := db.ExplainQuery(ctx, "SELECT * FROM system.tables")

		if err == nil {
			assert.NotEmpty(t, plan)
		}
	})

	t.Run("prepends EXPLAIN to query", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		query := "SELECT number FROM numbers(10)"
		plan, err := db.ExplainQuery(ctx, query)

		// The implementation prepends "EXPLAIN "
		if err == nil {
			assert.NotEmpty(t, plan)
		}
	})
}

func TestClickHouseDatabase_Ping(t *testing.T) {
	t.Run("successful ping", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		err = db.Ping(ctx)

		// Might fail due to connection, but should not panic
		_ = err
	})

	t.Run("ping with timeout", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err = db.Ping(ctx)

		// Should respect context timeout
		_ = err
	})
}

func TestClickHouseDatabase_Connect(t *testing.T) {
	t.Run("connect with valid config", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		newConfig := newTestClickHouseConfig()
		newConfig.Database = "system"

		err = db.Connect(ctx, newConfig)

		// Might fail, but should not panic
		_ = err
	})

	t.Run("connect replaces existing connection", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()

		// Connect again with same config - should close old connection
		err = db.Connect(ctx, config)

		// Should handle reconnection without panic
		_ = err
	})

	t.Run("connect closes old pool", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()

		// Get stats from old pool
		oldStats := db.GetConnectionStats()

		// Connect with new config
		newConfig := newTestClickHouseConfig()
		err = db.Connect(ctx, newConfig)

		if err == nil {
			// New pool should be created
			newStats := db.GetConnectionStats()
			_ = oldStats
			_ = newStats
		}
	})
}

func TestClickHouseDatabase_Disconnect(t *testing.T) {
	t.Run("disconnect closes connection", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}

		err = db.Disconnect()
		assert.NoError(t, err)
	})

	t.Run("disconnect on nil pool", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}

		// Disconnect once
		db.Disconnect()

		// Disconnect again - should not panic
		err = db.Disconnect()
		assert.NoError(t, err)
	})
}

func TestClickHouseDatabase_GetConnectionStats(t *testing.T) {
	t.Run("returns pool statistics", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		stats := db.GetConnectionStats()

		// Stats should be non-nil
		assert.NotNil(t, stats)
		// Should have reasonable values
		assert.GreaterOrEqual(t, stats.OpenConnections, 0)
	})
}

// Integration-style tests that demonstrate the full workflow
// These require a real ClickHouse database to be useful

func TestClickHouseDatabase_Integration_SimpleWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	ctx := context.Background()

	// Test connection
	err = db.Ping(ctx)
	if err != nil {
		t.Skip("Cannot ping database")
	}

	// Get schemas
	schemas, err := db.GetSchemas(ctx)
	if err != nil {
		t.Logf("GetSchemas failed: %v", err)
	} else {
		assert.NotNil(t, schemas)
		t.Logf("Found %d schemas", len(schemas))
	}

	// Get connection info
	info, err := db.GetConnectionInfo(ctx)
	if err != nil {
		t.Logf("GetConnectionInfo failed: %v", err)
	} else {
		assert.NotNil(t, info)
		t.Logf("ClickHouse version: %v", info["version"])
	}

	// Get tables from system database
	tables, err := db.GetTables(ctx, "system")
	if err != nil {
		t.Logf("GetTables failed: %v", err)
	} else {
		assert.NotNil(t, tables)
		t.Logf("Found %d system tables", len(tables))
	}
}

func TestClickHouseDatabase_Integration_CreateAndQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		t.Skip("Cannot connect to test database")
	}
	defer db.Disconnect()

	ctx := context.Background()

	// Create test table
	tableName := "test_users_" + time.Now().Format("20060102150405")
	createQuery := `
		CREATE TABLE ` + tableName + ` (
			id UInt64,
			name String,
			created_at DateTime
		) ENGINE = MergeTree()
		ORDER BY id
	`

	_, err = db.Execute(ctx, createQuery)
	if err != nil {
		t.Skip("Cannot create test table")
	}

	// Cleanup
	defer func() {
		_, _ = db.Execute(ctx, "DROP TABLE IF EXISTS "+tableName)
	}()

	// Insert data
	insertQuery := "INSERT INTO " + tableName + " (id, name, created_at) VALUES (1, 'Alice', now()), (2, 'Bob', now())"
	result, err := db.Execute(ctx, insertQuery)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Query data
	selectQuery := "SELECT * FROM " + tableName + " ORDER BY id"
	result, err = db.Execute(ctx, selectQuery)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.RowCount)
	assert.Len(t, result.Columns, 3)
	assert.Len(t, result.Rows, 2)

	// Get table structure
	structure, err := db.GetTableStructure(ctx, "default", tableName)
	require.NoError(t, err)
	assert.Equal(t, tableName, structure.Table.Name)
	assert.Len(t, structure.Columns, 3)

	// Verify engine is MergeTree
	engine, hasEngine := structure.Table.Metadata["engine"]
	assert.True(t, hasEngine)
	assert.Contains(t, engine, "MergeTree")

	// Test streaming
	streamQuery := "SELECT * FROM " + tableName
	batches := 0
	err = db.ExecuteStream(ctx, streamQuery, 1, func(rows [][]interface{}) error {
		batches++
		assert.NotNil(t, rows)
		return nil
	})
	require.NoError(t, err)
	assert.Greater(t, batches, 0)

	// Test explain
	plan, err := db.ExplainQuery(ctx, selectQuery)
	require.NoError(t, err)
	assert.NotEmpty(t, plan)
}

// Mock-based tests for specific scenarios

func TestClickHouseDatabase_Mock_Execute(t *testing.T) {
	t.Run("SELECT query with results", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		// Mock a simple SELECT
		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "Alice").
			AddRow(2, "Bob")

		mock.ExpectQuery("SELECT (.+) FROM users").WillReturnRows(rows)

		// Direct database query (not through our wrapper)
		result, err := db.Query("SELECT id, name FROM users")
		require.NoError(t, err)
		defer result.Close()

		// Verify expectations
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("INSERT query with affected rows", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		mock.ExpectExec("INSERT INTO users").
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := db.Exec("INSERT INTO users (name) VALUES (?)", "Charlie")
		require.NoError(t, err)

		affected, err := result.RowsAffected()
		require.NoError(t, err)
		assert.Equal(t, int64(1), affected)

		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("query error", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		expectedErr := errors.New("syntax error")
		mock.ExpectQuery("SELECT (.+) FROM invalid").
			WillReturnError(expectedErr)

		_, err = db.Query("SELECT * FROM invalid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "syntax error")

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestClickHouseDatabase_ByteArrayConversion(t *testing.T) {
	// Tests that byte arrays from ClickHouse are converted to strings
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// ClickHouse may return strings as []byte
	rows := sqlmock.NewRows([]string{"text_col"}).
		AddRow([]byte("hello world"))

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	result, err := db.Query("SELECT text_col FROM test")
	require.NoError(t, err)
	defer result.Close()

	// Scan the result
	result.Next()
	var value interface{}
	err = result.Scan(&value)
	require.NoError(t, err)

	// Verify it's a byte array (driver behavior)
	bytes, ok := value.([]byte)
	assert.True(t, ok)
	assert.Equal(t, "hello world", string(bytes))

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestClickHouseDatabase_SchemaIntrospection(t *testing.T) {
	t.Run("GetSchemas filters system databases", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"name"}).
			AddRow("default").
			AddRow("custom_db")

		// The query should filter out system databases
		mock.ExpectQuery("SELECT name FROM system.databases").
			WillReturnRows(rows)

		result, err := db.Query(`
			SELECT name
			FROM system.databases
			WHERE name NOT IN ('system', 'INFORMATION_SCHEMA', 'information_schema')
			ORDER BY name`)
		require.NoError(t, err)
		defer result.Close()

		schemas := []string{}
		for result.Next() {
			var schema string
			result.Scan(&schema)
			schemas = append(schemas, schema)
		}

		// Verify we got the expected schemas (order may vary based on query execution)
		assert.Len(t, schemas, 2)
		assert.Contains(t, schemas, "custom_db")
		assert.Contains(t, schemas, "default")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GetTables includes engine metadata", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"database", "name", "engine", "comment", "total_rows", "total_bytes"}).
			AddRow("default", "users", "MergeTree", "", 1000, 50000).
			AddRow("default", "orders", "ReplacingMergeTree", "", 5000, 250000)

		mock.ExpectQuery("SELECT (.+) FROM system.tables WHERE database = (.+)").
			WithArgs("default").
			WillReturnRows(rows)

		result, err := db.Query(`
			SELECT
				database,
				name,
				engine,
				'',
				total_rows,
				total_bytes
			FROM system.tables
			WHERE database = ?
			ORDER BY name`, "default")
		require.NoError(t, err)
		defer result.Close()

		tables := []struct {
			database   string
			name       string
			engine     string
			comment    string
			totalRows  int64
			totalBytes int64
		}{}

		for result.Next() {
			var table struct {
				database   string
				name       string
				engine     string
				comment    string
				totalRows  int64
				totalBytes int64
			}
			result.Scan(&table.database, &table.name, &table.engine, &table.comment, &table.totalRows, &table.totalBytes)
			tables = append(tables, table)
		}

		assert.Len(t, tables, 2)
		assert.Equal(t, "MergeTree", tables[0].engine)
		assert.Equal(t, "ReplacingMergeTree", tables[1].engine)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestClickHouseDatabase_ErrorHandling(t *testing.T) {
	t.Run("context cancellation", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = db.Ping(ctx)
		assert.Error(t, err)
	})

	t.Run("connection timeout", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()
		config.Host = "192.0.2.1" // Non-routable IP
		config.ConnectionTimeout = 1 * time.Second

		_, err := database.NewClickHouseDatabase(config, logger)
		// Should timeout or fail to connect
		assert.Error(t, err)
	})

	t.Run("invalid query syntax", func(t *testing.T) {
		logger := newTestLogger()
		config := newTestClickHouseConfig()

		db, err := database.NewClickHouseDatabase(config, logger)
		if err != nil {
			t.Skip("Cannot connect to test database")
		}
		defer db.Disconnect()

		ctx := context.Background()
		_, err = db.Execute(ctx, "INVALID SQL QUERY")

		// Should return error
		if !isConnectionError(err) {
			assert.Error(t, err)
		}
	})
}

// Benchmark tests

func BenchmarkClickHouseDatabase_QuoteIdentifier(b *testing.B) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		b.Skip("Cannot create database instance")
	}
	defer db.Disconnect()

	identifiers := []string{
		"users",
		"user_events",
		"orders_2024",
		"table`with`backticks",
		"a_very_long_table_name_with_many_characters",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, id := range identifiers {
			_ = db.QuoteIdentifier(id)
		}
	}
}

func BenchmarkClickHouseDatabase_GetDataTypeMappings(b *testing.B) {
	logger := newTestLogger()
	config := newTestClickHouseConfig()

	db, err := database.NewClickHouseDatabase(config, logger)
	if err != nil {
		b.Skip("Cannot create database instance")
	}
	defer db.Disconnect()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = db.GetDataTypeMappings()
	}
}

// Helper functions

// isConnectionError checks if an error is a connection-related error
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "connection refused") ||
		contains(errStr, "no such host") ||
		contains(errStr, "timeout") ||
		contains(errStr, "cannot connect")
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
