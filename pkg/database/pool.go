package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jbeck018/howlerops/pkg/crypto"
	"github.com/sirupsen/logrus"
)

// ConnectionPool manages database connections with pooling
type ConnectionPool struct {
	config        ConnectionConfig
	db            *sql.DB
	mu            sync.RWMutex
	closed        bool
	logger        *logrus.Logger
	sshTunnel     *SSHTunnel
	tunnelManager *SSHTunnelManager
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config ConnectionConfig, secretStore crypto.SecretStore, logger *logrus.Logger) (*ConnectionPool, error) {
	pool := &ConnectionPool{
		config:        config,
		logger:        logger,
		tunnelManager: NewSSHTunnelManager(secretStore, logger),
	}

	if err := pool.connect(); err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return pool, nil
}

// connect establishes the database connection
func (p *ConnectionPool) connect() error {
	// Establish SSH tunnel if configured
	if p.config.UseTunnel && p.config.SSHTunnel != nil {
		ctx := context.Background()
		tunnel, err := p.tunnelManager.EstablishTunnel(
			ctx,
			p.config.SSHTunnel,
			p.config.Host,
			p.config.Port,
		)
		if err != nil {
			return fmt.Errorf("failed to establish SSH tunnel: %w", err)
		}

		p.sshTunnel = tunnel

		// Replace host and port with tunnel's local endpoint
		p.config.Host = "127.0.0.1"
		p.config.Port = tunnel.GetLocalPort()

		p.logger.WithFields(logrus.Fields{
			"local_port": tunnel.GetLocalPort(),
			"remote":     fmt.Sprintf("%s:%d", p.sshTunnel.remoteHost, p.sshTunnel.remotePort),
		}).Info("Database connection will use SSH tunnel")
	}

	dsn, err := p.buildDSN()
	if err != nil {
		return fmt.Errorf("failed to build DSN: %w", err)
	}

	driverName, err := driverNameForType(p.config.Type)
	if err != nil {
		return err
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	if p.config.MaxConnections > 0 {
		db.SetMaxOpenConns(p.config.MaxConnections)
	} else {
		db.SetMaxOpenConns(25) // Default
	}

	if p.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(p.config.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(5) // Default
	}

	if p.config.IdleTimeout > 0 {
		db.SetConnMaxIdleTime(p.config.IdleTimeout)
	} else {
		db.SetConnMaxIdleTime(5 * time.Minute) // Default
	}

	// Set connection max lifetime
	db.SetConnMaxLifetime(time.Hour)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), p.getConnectionTimeout())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close() // Best-effort close
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.db = db
	p.logger.WithFields(logrus.Fields{
		"type":     p.config.Type,
		"database": p.config.Database,
		"host":     p.config.Host,
	}).Info("Database connection pool created successfully")

	return nil
}

// buildDSN builds the data source name for the connection
func (p *ConnectionPool) buildDSN() (string, error) {
	switch p.config.Type {
	case PostgreSQL:
		return p.buildPostgresDSN(), nil
	case MySQL, MariaDB:
		return p.buildMySQLDSN(), nil
	case SQLite:
		return p.buildSQLiteDSN(), nil
	case ClickHouse:
		return p.buildClickHouseDSN(), nil
	case TiDB:
		return p.buildTiDBDSN(), nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", p.config.Type)
	}
}

// stripProtocol removes http:// or https:// prefix from host
func stripProtocol(host string) string {
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	return host
}

// maintenanceDatabase returns the database used to establish a session when the
// user has not chosen a specific one (pgAdmin-style). It only opens the
// connection so databases can be enumerated; the user selects the working
// database afterwards via SwitchDatabase. An empty return means the engine needs
// no default database to connect.
func maintenanceDatabase(t DatabaseType) string {
	switch t {
	case PostgreSQL:
		return "postgres"
	case ClickHouse:
		return "default"
	default:
		// MySQL/MariaDB/TiDB connect happily with no default schema; the
		// document/search engines have no database concept at connect time.
		return ""
	}
}

// buildPostgresDSN builds PostgreSQL DSN
func (p *ConnectionPool) buildPostgresDSN() string {
	dbName := p.config.Database
	if dbName == "" {
		dbName = maintenanceDatabase(PostgreSQL)
	}
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s",
		stripProtocol(p.config.Host), p.config.Port, dbName, p.config.Username, p.config.Password)

	if p.config.SSLMode != "" {
		dsn += fmt.Sprintf(" sslmode=%s", p.config.SSLMode)
	} else {
		dsn += " sslmode=prefer"
	}

	if p.config.ConnectionTimeout > 0 {
		dsn += fmt.Sprintf(" connect_timeout=%d", int(p.config.ConnectionTimeout.Seconds()))
	}

	// When behind a connection pooler (pgcat, PgBouncer) in transaction mode,
	// binary_parameters=yes eliminates the Describe+Sync round-trip in lib/pq
	// that causes "unnamed prepared statement does not exist" errors when the
	// pooler reassigns the backend connection between protocol messages.
	if p.config.PoolerCompatible {
		dsn += " binary_parameters=yes"
	}

	// Add custom parameters
	for key, value := range p.config.Parameters {
		dsn += fmt.Sprintf(" %s=%s", key, value)
	}

	return dsn
}

// buildMySQLDSN builds MySQL/MariaDB DSN
func (p *ConnectionPool) buildMySQLDSN() string {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		p.config.Username, p.config.Password, p.config.Host, p.config.Port, p.config.Database)

	params := make(map[string]string)

	// Set default parameters
	params["parseTime"] = "true"
	params["loc"] = "UTC"

	if p.config.ConnectionTimeout > 0 {
		params["timeout"] = p.config.ConnectionTimeout.String()
	}

	// Add SSL configuration
	if p.config.SSLMode != "" {
		switch p.config.SSLMode {
		case "disable":
			params["tls"] = "false"
		case "require":
			params["tls"] = "true"
		default:
			params["tls"] = "preferred"
		}
	}

	// Add custom parameters
	for key, value := range p.config.Parameters {
		params[key] = value
	}

	// Build parameter string
	var paramStr string
	first := true
	for key, value := range params {
		if first {
			paramStr += "?"
			first = false
		} else {
			paramStr += "&"
		}
		paramStr += fmt.Sprintf("%s=%s", key, value)
	}

	return dsn + paramStr
}

// buildSQLiteDSN builds SQLite DSN
func (p *ConnectionPool) buildSQLiteDSN() string {
	dsn := p.config.Database

	// For :memory: databases with cache=shared, use file::memory: format
	// This ensures the in-memory database is properly shared across connections
	if dsn == ":memory:" {
		if cacheMode, ok := p.config.Parameters["cache"]; ok && cacheMode == "shared" {
			dsn = "file::memory:"
		}
	}

	if len(p.config.Parameters) > 0 {
		// Check if DSN already has parameters (contains '?')
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}

		dsn += separator
		first := true
		for key, value := range p.config.Parameters {
			if !first {
				dsn += "&"
			}
			dsn += fmt.Sprintf("%s=%s", key, value)
			first = false
		}
	}

	return dsn
}

// buildClickHouseDSN builds ClickHouse DSN.
//
// ClickHouse exposes two wire protocols on different ports: the native TCP
// protocol (9000, or 9440 with TLS) and an HTTP interface (8123, or 8443 with
// TLS — what ClickHouse Cloud exposes). The driver picks the protocol from the
// DSN scheme (clickhouse:// = native, http:// / https:// = HTTP). Pointing the
// native protocol at an HTTP port fails the handshake with
// "unexpected packet [72] from server" (72 = 'H' from "HTTP/..."), so we select
// the scheme from the port (overridable via a `protocol` parameter).
func (p *ConnectionPool) buildClickHouseDSN() string {
	dbName := p.config.Database
	if dbName == "" {
		dbName = maintenanceDatabase(ClickHouse)
	}

	// Pick scheme/security from the port, then allow explicit overrides.
	scheme := "clickhouse" // native
	secure := p.config.SSLMode != "" && p.config.SSLMode != "disable"
	switch p.config.Port {
	case 8123: // native HTTP interface
		scheme = "http"
	case 8443: // HTTPS interface (ClickHouse Cloud)
		scheme = "https"
		secure = true
	case 9440: // native protocol over TLS
		scheme = "clickhouse"
		secure = true
	}

	// Explicit override via parameters (protocol=http|https|native|tcp).
	if proto, ok := p.config.Parameters["protocol"]; ok {
		switch strings.ToLower(proto) {
		case "http":
			scheme = "http"
		case "https":
			scheme = "https"
			secure = true
		case "native", "tcp", "clickhouse":
			scheme = "clickhouse"
		}
	}

	// The driver rejects http+TLS and requires https to be secure.
	if scheme == "http" {
		secure = false
	} else if scheme == "https" {
		secure = true
	}

	dsn := fmt.Sprintf("%s://%s:%s@%s:%d/%s",
		scheme, p.config.Username, p.config.Password, stripProtocol(p.config.Host), p.config.Port, dbName)

	params := make(map[string]string)

	// Add connection timeout
	if p.config.ConnectionTimeout > 0 {
		params["dial_timeout"] = fmt.Sprintf("%ds", int(p.config.ConnectionTimeout.Seconds()))
	}

	// TLS: https implies secure; native uses the secure flag (9440 / SSLMode).
	// http must never set secure (the driver errors on http+TLS).
	if secure {
		params["secure"] = "true"
		if p.config.SSLMode == "skip-verify" {
			params["skip_verify"] = "true"
		}
	}

	// Add custom parameters (skip the protocol override; it's not a driver param)
	for key, value := range p.config.Parameters {
		if key == "protocol" {
			continue
		}
		params[key] = value
	}

	// Build parameter string
	if len(params) > 0 {
		dsn += "?"
		first := true
		for key, value := range params {
			if !first {
				dsn += "&"
			}
			dsn += fmt.Sprintf("%s=%s", key, value)
			first = false
		}
	}

	return dsn
}

// buildTiDBDSN builds TiDB DSN (uses MySQL format)
func (p *ConnectionPool) buildTiDBDSN() string {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		p.config.Username, p.config.Password, p.config.Host, p.config.Port, p.config.Database)

	params := make(map[string]string)

	// Set default parameters
	params["parseTime"] = "true"
	params["loc"] = "UTC"

	if p.config.ConnectionTimeout > 0 {
		params["timeout"] = p.config.ConnectionTimeout.String()
	}

	// Add SSL configuration
	if p.config.SSLMode != "" {
		switch p.config.SSLMode {
		case "disable":
			params["tls"] = "false"
		case "require":
			params["tls"] = "true"
		default:
			params["tls"] = "preferred"
		}
	}

	// Add custom parameters (including TiDB-specific ones)
	for key, value := range p.config.Parameters {
		params[key] = value
	}

	// Build parameter string
	var paramStr string
	first := true
	for key, value := range params {
		if first {
			paramStr += "?"
			first = false
		} else {
			paramStr += "&"
		}
		paramStr += fmt.Sprintf("%s=%s", key, value)
	}

	return dsn + paramStr
}

// getConnectionTimeout returns the connection timeout or a default value
func (p *ConnectionPool) getConnectionTimeout() time.Duration {
	if p.config.ConnectionTimeout > 0 {
		return p.config.ConnectionTimeout
	}
	return 30 * time.Second
}

func driverNameForType(dbType DatabaseType) (string, error) {
	switch dbType {
	case PostgreSQL:
		return "postgres", nil
	case MySQL, MariaDB:
		return "mysql", nil
	case SQLite:
		return "sqlite3", nil
	case ClickHouse:
		return "clickhouse", nil
	case TiDB:
		return "mysql", nil // TiDB uses MySQL driver
	default:
		return "", fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// Get returns a database connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (*sql.DB, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, fmt.Errorf("connection pool is closed")
	}

	if p.db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	return p.db, nil
}

// Close closes the connection pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true

	var lastErr error

	// Close database connection first
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			lastErr = err
			p.logger.WithError(err).Error("Failed to close database connection")
		}
	}

	// Close SSH tunnel if it exists
	if p.sshTunnel != nil {
		if err := p.tunnelManager.CloseTunnel(p.sshTunnel); err != nil {
			lastErr = err
			p.logger.WithError(err).Error("Failed to close SSH tunnel")
		}
		p.sshTunnel = nil
	}

	p.logger.WithFields(logrus.Fields{
		"type":     p.config.Type,
		"database": p.config.Database,
	}).Info("Database connection pool closed")

	return lastErr
}

// Stats returns connection pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.db == nil {
		return PoolStats{}
	}

	stats := p.db.Stats()
	return PoolStats{
		OpenConnections:   stats.OpenConnections,
		InUse:             stats.InUse,
		Idle:              stats.Idle,
		WaitCount:         stats.WaitCount,
		WaitDuration:      stats.WaitDuration,
		MaxIdleClosed:     stats.MaxIdleClosed,
		MaxIdleTimeClosed: stats.MaxIdleTimeClosed,
		MaxLifetimeClosed: stats.MaxLifetimeClosed,
	}
}

// Ping tests the database connection
func (p *ConnectionPool) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("connection pool is closed")
	}

	if p.db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	return p.db.PingContext(ctx)
}

// GetHealth returns the health status of the connection pool
func (p *ConnectionPool) GetHealth(ctx context.Context) HealthStatus {
	start := time.Now()
	status := HealthStatus{
		Timestamp: start,
		Metrics:   make(map[string]string),
	}

	// Add pool statistics to metrics
	stats := p.Stats()
	status.Metrics["open_connections"] = fmt.Sprintf("%d", stats.OpenConnections)
	status.Metrics["in_use"] = fmt.Sprintf("%d", stats.InUse)
	status.Metrics["idle"] = fmt.Sprintf("%d", stats.Idle)

	// Test connection
	if err := p.Ping(ctx); err != nil {
		status.Status = "unhealthy"
		status.Message = fmt.Sprintf("Failed to ping database: %v", err)
		status.ResponseTime = time.Since(start)
		return status
	}

	status.Status = "healthy"
	status.Message = "Database connection is healthy"
	status.ResponseTime = time.Since(start)

	return status
}

// Reconnect attempts to reconnect to the database
func (p *ConnectionPool) Reconnect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Close existing connection
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			log.Printf("Failed to close existing database connection: %v", err)
		}
		p.db = nil
	}

	// Reconnect
	return p.connect()
}
