//go:build duckdb

package duckdb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// passwordRedact strips a `password=...` token from a string so connection
// credentials never reach logs or error messages.
var passwordRedact = regexp.MustCompile(`(?i)password=[^\s';]*`)

func redact(s string) string {
	return passwordRedact.ReplaceAllString(s, "password=***")
}

// quoteIdent quotes a DuckDB identifier (catalog alias).
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteLiteral quotes a DuckDB string literal (the ATTACH connection string).
func quoteLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

// Attach attaches a backend database (postgres/mysql/sqlite) into the embedded
// DuckDB under the given alias so federated queries can reference its tables as
// alias.schema.table. It is idempotent: if the same connection is already
// attached with an identical fingerprint, it is a no-op; if the fingerprint
// changed (credentials/host/database changed) the stale attachment is replaced.
//
// dbType is the DuckDB ATTACH TYPE: "postgres", "mysql", or "sqlite".
// Credentials live only in this statement and are never logged.
func (e *Engine) Attach(ctx context.Context, sessionID, alias, connStr, dbType, fingerprint string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.db == nil {
		return fmt.Errorf("federation engine not initialized")
	}

	if cur, ok := e.attached[sessionID]; ok {
		if cur.alias == alias && cur.fingerprint == fingerprint {
			return nil // already attached and unchanged
		}
		// Stale (credentials/host/db changed) — detach before re-attaching.
		_, _ = e.db.ExecContext(ctx, "DETACH "+quoteIdent(cur.alias))
		delete(e.attached, sessionID)
	}

	stmt := fmt.Sprintf(
		"ATTACH %s AS %s (TYPE %s, READ_ONLY)",
		quoteLiteral(connStr), quoteIdent(alias), dbType,
	)
	if _, err := e.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("failed to attach connection %s (%s): %s", sessionID, dbType, redact(err.Error()))
	}

	e.attached[sessionID] = attachInfo{alias: alias, fingerprint: fingerprint}
	e.logger.WithField("alias", alias).WithField("type", dbType).Debug("Attached federated database")
	return nil
}

// Detach removes a previously attached backend database. Safe to call when the
// connection isn't attached.
func (e *Engine) Detach(ctx context.Context, sessionID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.db == nil {
		return nil
	}
	info, ok := e.attached[sessionID]
	if !ok {
		return nil
	}
	_, err := e.db.ExecContext(ctx, "DETACH "+quoteIdent(info.alias))
	delete(e.attached, sessionID)
	if err != nil {
		return fmt.Errorf("failed to detach %s: %w", info.alias, err)
	}
	return nil
}
