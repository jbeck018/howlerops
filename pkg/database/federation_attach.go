package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// BuildAttachString builds a DuckDB ATTACH connection string and TYPE for a
// connection config, plus a fingerprint that changes whenever the effective
// target or credentials change (used to invalidate cached attachments).
//
// Credentials are embedded in the returned connStr (it becomes part of a single
// ATTACH statement that is never logged). Tunneled/VPC connections are not yet
// supported for federation — the engine reaches the backend directly, which
// would bypass the tunnel — so they return an error and the caller falls back
// to legacy single-connection execution.
func BuildAttachString(cfg ConnectionConfig) (connStr string, duckType string, fingerprint string, err error) {
	if cfg.UseTunnel || cfg.UseVPC {
		return "", "", "", fmt.Errorf("federation is not yet supported for tunneled/VPC connections")
	}

	switch cfg.Type {
	case PostgreSQL:
		db := cfg.Database
		if db == "" {
			db = maintenanceDatabase(PostgreSQL)
		}
		sslmode := cfg.SSLMode
		if sslmode == "" {
			sslmode = "prefer"
		}
		// libpq keyword form (same shape as buildPostgresDSN), which the DuckDB
		// postgres extension accepts directly.
		connStr = fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
			stripProtocol(cfg.Host), cfg.Port, db, cfg.Username, cfg.Password, sslmode)
		duckType = "postgres"

	case MySQL, MariaDB:
		// DuckDB's mysql extension uses libmysql keyword form, NOT the
		// go-sql-driver user:pass@tcp(...) DSN used by the pool.
		connStr = fmt.Sprintf("host=%s port=%d user=%s password=%s database=%s",
			stripProtocol(cfg.Host), cfg.Port, cfg.Username, cfg.Password, cfg.Database)
		duckType = "mysql"

	case SQLite:
		if cfg.Database == "" {
			return "", "", "", fmt.Errorf("sqlite connection has no database file path")
		}
		connStr = cfg.Database
		duckType = "sqlite"

	default:
		return "", "", "", fmt.Errorf("cross-database federation is not supported for %s connections", cfg.Type)
	}

	return connStr, duckType, attachFingerprint(cfg), nil
}

// attachFingerprint changes when any field that affects the attached target or
// its credentials changes, so a cached attachment can be replaced.
func attachFingerprint(cfg ConnectionConfig) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s",
		cfg.Type, cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password, cfg.SSLMode)))
	return hex.EncodeToString(sum[:8])
}
