package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sirupsen/logrus"
)

// MySQLDatabase implements the Database interface for MySQL/MariaDB
type MySQLDatabase struct {
	pool           *ConnectionPool
	config         ConnectionConfig
	logger         *logrus.Logger
	structureCache *tableStructureCache
}

// NewMySQLDatabase creates a new MySQL database instance
func NewMySQLDatabase(config ConnectionConfig, logger *logrus.Logger) (*MySQLDatabase, error) {
	pool, err := NewConnectionPool(config, nil, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create MySQL connection pool: %w", err)
	}

	return &MySQLDatabase{
		pool:           pool,
		config:         config,
		logger:         logger,
		structureCache: newTableStructureCache(10 * time.Minute),
	}, nil
}

// Connect establishes a connection to MySQL
func (m *MySQLDatabase) Connect(ctx context.Context, config ConnectionConfig) error {
	m.config = config
	pool, err := NewConnectionPool(config, nil, m.logger)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %w", err)
	}

	if m.pool != nil {
		m.pool.Close()
	}
	m.pool = pool
	m.structureCache = newTableStructureCache(10 * time.Minute)

	return nil
}

// Disconnect closes the MySQL connection
func (m *MySQLDatabase) Disconnect() error {
	if m.pool != nil {
		return m.pool.Close()
	}
	return nil
}

// Ping tests the MySQL connection
func (m *MySQLDatabase) Ping(ctx context.Context) error {
	return m.pool.Ping(ctx)
}

// GetConnectionInfo returns MySQL connection information
func (m *MySQLDatabase) GetConnectionInfo(ctx context.Context) (map[string]interface{}, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	info := make(map[string]interface{})

	// Get version
	var version string
	err = db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	if err != nil {
		return nil, fmt.Errorf("failed to get MySQL version: %w", err)
	}
	info["version"] = version

	// Get current database
	var database string
	err = db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&database)
	if err == nil {
		info["database"] = database
	}

	// Get current user
	var user string
	err = db.QueryRowContext(ctx, "SELECT USER()").Scan(&user)
	if err == nil {
		info["user"] = user
	}

	// Get connection stats
	var totalConns, runningConns int
	err = db.QueryRowContext(ctx, "SHOW STATUS LIKE 'Threads_connected'").Scan(nil, &totalConns)
	if err == nil {
		info["total_connections"] = totalConns
	}

	err = db.QueryRowContext(ctx, "SHOW STATUS LIKE 'Threads_running'").Scan(nil, &runningConns)
	if err == nil {
		info["running_connections"] = runningConns
	}

	return info, nil
}

// Execute runs a SQL query and returns the results
func (m *MySQLDatabase) Execute(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	isSelect := strings.HasPrefix(strings.ToUpper(query), "SELECT") ||
		strings.HasPrefix(strings.ToUpper(query), "WITH") ||
		strings.HasPrefix(strings.ToUpper(query), "SHOW") ||
		strings.HasPrefix(strings.ToUpper(query), "DESCRIBE") ||
		strings.HasPrefix(strings.ToUpper(query), "EXPLAIN")

	if isSelect {
		return m.executeSelect(ctx, db, query, args...)
	} else {
		return m.executeNonSelect(ctx, db, query, args...)
	}
}

// executeSelect handles SELECT queries
func (m *MySQLDatabase) executeSelect(ctx context.Context, db *sql.DB, query string, args ...interface{}) (*QueryResult, error) {
	start := time.Now()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}
	defer rows.Close()

	// Get column information
	columns, err := rows.Columns()
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}

	result := &QueryResult{
		Columns:  columns,
		Rows:     make([][]interface{}, 0),
		Duration: time.Since(start),
	}

	// Read all rows
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return &QueryResult{
				Error:    err,
				Duration: time.Since(start),
			}, err
		}

		// Convert byte arrays to strings for MySQL
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		result.Rows = append(result.Rows, values)
	}

	if err := rows.Err(); err != nil {
		result.Error = err
		return result, err
	}

	result.RowCount = int64(len(result.Rows))
	result.Duration = time.Since(start)

	if metadata, ready, err := m.computeEditableMetadata(ctx, query, columns, false); err == nil {
		if metadata != nil {
			if !ready {
				metadata.Pending = true
				if metadata.Reason == "" {
					metadata.Reason = "Loading editable metadata"
				}
			}
			result.Editable = metadata
		}
	} else {
		meta := newEditableMetadata(columns)
		meta.Reason = "Failed to prepare editable metadata"
		result.Editable = meta
	}

	return result, nil
}

func (m *MySQLDatabase) computeEditableMetadata(ctx context.Context, query string, columns []string, allowFetch bool) (*EditableQueryMetadata, bool, error) {
	metadata := newEditableMetadata(columns)

	query = strings.TrimSpace(query)
	if query == "" {
		metadata.Reason = "Empty query"
		return metadata, true, nil
	}

	schema, table, reason, ok := parseSimpleSelect(query)
	if !ok {
		metadata.Reason = reason
		return metadata, true, nil
	}

	if table == "" {
		metadata.Reason = "Unable to identify target table"
		return metadata, true, nil
	}

	if schema == "" {
		schema = strings.TrimSpace(m.config.Database)
	}
	if schema == "" {
		metadata.Reason = "Unable to determine target schema"
		return metadata, true, nil
	}

	metadata.Schema = schema
	metadata.Table = table

	structure, ok, err := m.ensureTableStructure(ctx, schema, table, allowFetch)
	if err != nil {
		metadata.Reason = fmt.Sprintf("Failed to get table structure: %v", err)
		return metadata, true, err
	}

	if !ok {
		metadata.Pending = true
		if metadata.Reason == "" {
			metadata.Reason = "Loading table metadata"
		}
		return metadata, false, nil
	}

	columnMap := make(map[string]ColumnInfo, len(structure.Columns))
	primaryKeys := make([]string, 0)
	for _, col := range structure.Columns {
		columnMap[strings.ToLower(col.Name)] = col
		if col.PrimaryKey {
			primaryKeys = append(primaryKeys, col.Name)
		}
	}

	if len(primaryKeys) == 0 {
		metadata.Reason = "Table does not have a primary key"
		return metadata, true, nil
	}

	editableColumns := make([]EditableColumn, 0, len(columns))
	for _, col := range columns {
		if colInfo, exists := columnMap[strings.ToLower(col)]; exists {
			editableColumns = append(editableColumns, EditableColumn{
				Name:       colInfo.Name,
				ResultName: colInfo.Name,
				DataType:   colInfo.DataType,
				Editable:   true,
				PrimaryKey: colInfo.PrimaryKey,
			})
		} else {
			editableColumns = append(editableColumns, EditableColumn{
				Name:       col,
				ResultName: col,
				Editable:   false,
				PrimaryKey: false,
			})
		}
	}

	metadata.Enabled = true
	metadata.PrimaryKeys = primaryKeys
	metadata.Columns = editableColumns
	metadata.Pending = false
	metadata.Reason = ""

	return metadata, true, nil
}

func (m *MySQLDatabase) ensureTableStructure(ctx context.Context, schema, table string, allowFetch bool) (*TableStructure, bool, error) {
	if structure, ok := m.structureCache.get(schema, table); ok {
		return structure, true, nil
	}

	if !allowFetch {
		return nil, false, nil
	}

	structure, err := m.loadTableStructure(ctx, schema, table)
	if err != nil {
		return nil, false, err
	}

	m.structureCache.set(schema, table, structure)
	return cloneTableStructure(structure), true, nil
}

func (m *MySQLDatabase) loadTableStructure(ctx context.Context, schema, table string) (*TableStructure, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	structure := &TableStructure{}

	tableInfo, err := m.getTableInfo(ctx, db, schema, table)
	if err != nil {
		return nil, err
	}
	structure.Table = *tableInfo

	columns, err := m.getTableColumns(ctx, db, schema, table)
	if err != nil {
		return nil, err
	}
	structure.Columns = columns

	indexes, err := m.getTableIndexes(ctx, db, schema, table)
	if err != nil {
		return nil, err
	}
	structure.Indexes = indexes

	foreignKeys, err := m.getTableForeignKeys(ctx, db, schema, table)
	if err != nil {
		return nil, err
	}
	structure.ForeignKeys = foreignKeys

	return structure, nil
}

func (m *MySQLDatabase) ComputeEditableMetadata(ctx context.Context, query string, columns []string) (*EditableQueryMetadata, error) {
	metadata, _, err := m.computeEditableMetadata(ctx, query, columns, true)
	if metadata != nil {
		metadata.Pending = false
	}
	return metadata, err
}

// executeNonSelect handles INSERT, UPDATE, DELETE, DDL queries
func (m *MySQLDatabase) executeNonSelect(ctx context.Context, db *sql.DB, query string, args ...interface{}) (*QueryResult, error) {
	start := time.Now()

	sqlResult, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}

	affected, err := sqlResult.RowsAffected()
	if err != nil {
		affected = 0
	}

	return &QueryResult{
		Affected: affected,
		Duration: time.Since(start),
	}, nil
}

// ExecuteStream executes a query and streams results in batches
func (m *MySQLDatabase) ExecuteStream(ctx context.Context, query string, batchSize int, callback func([][]interface{}) error, args ...interface{}) error {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	batch := make([][]interface{}, 0, batchSize)

	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return err
		}

		// Convert byte arrays to strings for MySQL
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		batch = append(batch, values)

		if len(batch) >= batchSize {
			if err := callback(batch); err != nil {
				return err
			}
			batch = batch[:0] // Reset slice
		}
	}

	// Send remaining rows
	if len(batch) > 0 {
		if err := callback(batch); err != nil {
			return err
		}
	}

	return rows.Err()
}

// ExplainQuery returns the execution plan for a query
func (m *MySQLDatabase) ExplainQuery(ctx context.Context, query string, args ...interface{}) (string, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return "", err
	}

	explainQuery := "EXPLAIN FORMAT=JSON " + query

	var plan string
	err = db.QueryRowContext(ctx, explainQuery, args...).Scan(&plan)
	if err != nil {
		return "", fmt.Errorf("failed to explain query: %w", err)
	}

	return plan, nil
}

// GetSchemas returns list of schemas (databases) in MySQL
func (m *MySQLDatabase) GetSchemas(ctx context.Context) ([]string, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
		ORDER BY schema_name`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}

	return schemas, rows.Err()
}

// GetTables returns list of tables in a schema
func (m *MySQLDatabase) GetTables(ctx context.Context, schema string) ([]TableInfo, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT
			t.table_schema,
			t.table_name,
			t.table_type,
			COALESCE(t.table_comment, '') as comment,
			COALESCE(t.table_rows, 0) as row_count,
			COALESCE(t.data_length + t.index_length, 0) as size_bytes,
			t.create_time,
			t.update_time
		FROM information_schema.tables t
		WHERE t.table_schema = ?
		ORDER BY t.table_name`

	rows, err := db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var table TableInfo
		var createTime, updateTime sql.NullTime

		err := rows.Scan(
			&table.Schema,
			&table.Name,
			&table.Type,
			&table.Comment,
			&table.RowCount,
			&table.SizeBytes,
			&createTime,
			&updateTime,
		)
		if err != nil {
			return nil, err
		}

		if createTime.Valid {
			table.CreatedAt = &createTime.Time
		}
		if updateTime.Valid {
			table.UpdatedAt = &updateTime.Time
		}

		tables = append(tables, table)
	}

	return tables, rows.Err()
}

// GetTableStructure returns detailed structure information for a table
func (m *MySQLDatabase) GetTableStructure(ctx context.Context, schema, table string) (*TableStructure, error) {
	if structure, ok := m.structureCache.get(schema, table); ok {
		return structure, nil
	}

	structure, err := m.loadTableStructure(ctx, schema, table)
	if err != nil {
		return nil, err
	}

	m.structureCache.set(schema, table, structure)
	return cloneTableStructure(structure), nil
}

// Helper methods for getting table structure details
func (m *MySQLDatabase) getTableInfo(ctx context.Context, db *sql.DB, schema, table string) (*TableInfo, error) {
	query := `
		SELECT
			table_schema,
			table_name,
			table_type,
			COALESCE(table_comment, '') as comment,
			COALESCE(table_rows, 0) as row_count,
			COALESCE(data_length + index_length, 0) as size_bytes,
			create_time,
			update_time
		FROM information_schema.tables
		WHERE table_schema = ? AND table_name = ?`

	var tableInfo TableInfo
	var createTime, updateTime sql.NullTime

	err := db.QueryRowContext(ctx, query, schema, table).Scan(
		&tableInfo.Schema,
		&tableInfo.Name,
		&tableInfo.Type,
		&tableInfo.Comment,
		&tableInfo.RowCount,
		&tableInfo.SizeBytes,
		&createTime,
		&updateTime,
	)
	if err != nil {
		return nil, err
	}

	if createTime.Valid {
		tableInfo.CreatedAt = &createTime.Time
	}
	if updateTime.Valid {
		tableInfo.UpdatedAt = &updateTime.Time
	}

	return &tableInfo, nil
}

func (m *MySQLDatabase) getTableColumns(ctx context.Context, db *sql.DB, schema, table string) ([]ColumnInfo, error) {
	query := `
		SELECT
			column_name,
			data_type,
			is_nullable = 'YES' as nullable,
			column_default,
			ordinal_position,
			character_maximum_length,
			numeric_precision,
			numeric_scale,
			COALESCE(column_comment, '') as comment,
			column_key = 'PRI' as primary_key,
			column_key IN ('UNI', 'PRI') as unique_key
		FROM information_schema.columns
		WHERE table_schema = ? AND table_name = ?
		ORDER BY ordinal_position`

	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var defaultValue sql.NullString
		var charMaxLen sql.NullInt64
		var numPrecision, numScale sql.NullInt64

		err := rows.Scan(
			&col.Name,
			&col.DataType,
			&col.Nullable,
			&defaultValue,
			&col.OrdinalPosition,
			&charMaxLen,
			&numPrecision,
			&numScale,
			&col.Comment,
			&col.PrimaryKey,
			&col.Unique,
		)
		if err != nil {
			return nil, err
		}

		if defaultValue.Valid {
			col.DefaultValue = &defaultValue.String
		}
		if charMaxLen.Valid {
			col.CharacterMaxLength = &charMaxLen.Int64
		}
		if numPrecision.Valid {
			precision := int(numPrecision.Int64)
			col.NumericPrecision = &precision
		}
		if numScale.Valid {
			scale := int(numScale.Int64)
			col.NumericScale = &scale
		}

		columns = append(columns, col)
	}

	return columns, rows.Err()
}

func (m *MySQLDatabase) getTableIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]IndexInfo, error) {
	query := `
		SELECT
			index_name,
			GROUP_CONCAT(column_name ORDER BY seq_in_index) as columns,
			non_unique = 0 as unique_key,
			index_name = 'PRIMARY' as primary_key,
			index_type
		FROM information_schema.statistics
		WHERE table_schema = ? AND table_name = ?
		GROUP BY index_name, non_unique, index_type
		ORDER BY index_name`

	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var columns string

		err := rows.Scan(
			&idx.Name,
			&columns,
			&idx.Unique,
			&idx.Primary,
			&idx.Type,
		)
		if err != nil {
			return nil, err
		}

		if columns != "" {
			idx.Columns = strings.Split(columns, ",")
		}

		indexes = append(indexes, idx)
	}

	return indexes, rows.Err()
}

func (m *MySQLDatabase) getTableForeignKeys(ctx context.Context, db *sql.DB, schema, table string) ([]ForeignKeyInfo, error) {
	query := `
		SELECT
			constraint_name,
			GROUP_CONCAT(column_name ORDER BY ordinal_position) as columns,
			referenced_table_name,
			referenced_table_schema,
			GROUP_CONCAT(referenced_column_name ORDER BY ordinal_position) as referenced_columns
		FROM information_schema.key_column_usage
		WHERE table_schema = ? AND table_name = ?
		AND referenced_table_name IS NOT NULL
		GROUP BY constraint_name, referenced_table_name, referenced_table_schema
		ORDER BY constraint_name`

	rows, err := db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var foreignKeys []ForeignKeyInfo
	for rows.Next() {
		var fk ForeignKeyInfo
		var columns, refColumns string

		err := rows.Scan(
			&fk.Name,
			&columns,
			&fk.ReferencedTable,
			&fk.ReferencedSchema,
			&refColumns,
		)
		if err != nil {
			return nil, err
		}

		if columns != "" {
			fk.Columns = strings.Split(columns, ",")
		}
		if refColumns != "" {
			fk.ReferencedColumns = strings.Split(refColumns, ",")
		}

		// Get referential actions
		actionQuery := `
			SELECT delete_rule, update_rule
			FROM information_schema.referential_constraints
			WHERE constraint_schema = ? AND constraint_name = ?`

		err = db.QueryRowContext(ctx, actionQuery, schema, fk.Name).Scan(&fk.OnDelete, &fk.OnUpdate)
		if err != nil {
			// Set defaults if query fails
			fk.OnDelete = "RESTRICT"
			fk.OnUpdate = "RESTRICT"
		}

		foreignKeys = append(foreignKeys, fk)
	}

	return foreignKeys, rows.Err()
}

// BeginTransaction starts a new transaction
func (m *MySQLDatabase) BeginTransaction(ctx context.Context) (Transaction, error) {
	db, err := m.pool.Get(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &MySQLTransaction{tx: tx}, nil
}

// UpdateRow is currently not supported for MySQL
func (m *MySQLDatabase) UpdateRow(ctx context.Context, params UpdateRowParams) error {
	return errors.New("row editing is not yet supported for MySQL connections")
}

// GetDatabaseType returns the database type
func (m *MySQLDatabase) GetDatabaseType() DatabaseType {
	return MySQL
}

// GetConnectionStats returns connection pool statistics
func (m *MySQLDatabase) GetConnectionStats() PoolStats {
	return m.pool.Stats()
}

// QuoteIdentifier quotes an identifier for MySQL
func (m *MySQLDatabase) QuoteIdentifier(identifier string) string {
	return fmt.Sprintf("`%s`", strings.ReplaceAll(identifier, "`", "``"))
}

// GetDataTypeMappings returns MySQL-specific data type mappings
func (m *MySQLDatabase) GetDataTypeMappings() map[string]string {
	return map[string]string{
		"string":  "TEXT",
		"int":     "INT",
		"int64":   "BIGINT",
		"float":   "FLOAT",
		"float64": "DOUBLE",
		"bool":    "BOOLEAN",
		"time":    "DATETIME",
		"date":    "DATE",
		"json":    "JSON",
		"uuid":    "CHAR(36)",
		"text":    "TEXT",
		"varchar": "VARCHAR",
		"decimal": "DECIMAL",
	}
}

// MySQLTransaction implements the Transaction interface for MySQL
type MySQLTransaction struct {
	tx *sql.Tx
}

// Execute runs a query within the transaction
func (t *MySQLTransaction) Execute(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	query = strings.TrimSpace(query)
	isSelect := strings.HasPrefix(strings.ToUpper(query), "SELECT") ||
		strings.HasPrefix(strings.ToUpper(query), "WITH") ||
		strings.HasPrefix(strings.ToUpper(query), "SHOW") ||
		strings.HasPrefix(strings.ToUpper(query), "DESCRIBE") ||
		strings.HasPrefix(strings.ToUpper(query), "EXPLAIN")

	if isSelect {
		return t.executeSelect(ctx, query, args...)
	} else {
		return t.executeNonSelect(ctx, query, args...)
	}
}

func (t *MySQLTransaction) executeSelect(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	start := time.Now()

	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}

	result := &QueryResult{
		Columns:  columns,
		Rows:     make([][]interface{}, 0),
		Duration: time.Since(start),
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return &QueryResult{
				Error:    err,
				Duration: time.Since(start),
			}, err
		}

		// Convert byte arrays to strings for MySQL
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
		}

		result.Rows = append(result.Rows, values)
	}

	result.RowCount = int64(len(result.Rows))
	result.Duration = time.Since(start)

	return result, rows.Err()
}

func (t *MySQLTransaction) executeNonSelect(ctx context.Context, query string, args ...interface{}) (*QueryResult, error) {
	start := time.Now()

	sqlResult, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return &QueryResult{
			Error:    err,
			Duration: time.Since(start),
		}, err
	}

	affected, err := sqlResult.RowsAffected()
	if err != nil {
		affected = 0
	}

	return &QueryResult{
		Affected: affected,
		Duration: time.Since(start),
	}, nil
}

// Commit commits the transaction
func (t *MySQLTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *MySQLTransaction) Rollback() error {
	return t.tx.Rollback()
}
