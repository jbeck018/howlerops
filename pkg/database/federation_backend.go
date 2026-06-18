package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/pkg/database/multiquery"
	"github.com/jbeck018/howlerops/pkg/federation/duckdb"
	"github.com/sirupsen/logrus"
)

// federationBackend bridges the multiquery executor to the DuckDB engine: it
// resolves connection credentials from the Manager and ATTACHes each referenced
// connection so @conn queries run as real cross-database federation.
type federationBackend struct {
	manager *Manager
	engine  *duckdb.Engine
	logger  *logrus.Logger
}

// EnsureAttached attaches every named connection into DuckDB (idempotently) and
// returns a map of connection name -> attached catalog alias.
func (b *federationBackend) EnsureAttached(ctx context.Context, names []string) (map[string]string, error) {
	out := make(map[string]string, len(names))
	for _, name := range names {
		cfg, sessionID, ok := b.manager.connectionConfigForFederation(name)
		if !ok {
			return nil, fmt.Errorf("connection not found: %s", name)
		}
		connStr, duckType, fingerprint, err := BuildAttachString(cfg)
		if err != nil {
			return nil, err
		}
		alias := multiquery.DuckAliasForSession(sessionID)
		if err := b.engine.Attach(ctx, sessionID, alias, connStr, duckType, fingerprint); err != nil {
			return nil, err
		}
		out[name] = alias
	}
	return out, nil
}

// Execute runs already-rewritten federation SQL through DuckDB.
func (b *federationBackend) Execute(ctx context.Context, sql string, timeout time.Duration) (*multiquery.QueryResult, error) {
	res, err := b.engine.ExecuteQuery(ctx, sql, timeout)
	if err != nil {
		return nil, err
	}
	return &multiquery.QueryResult{
		Columns:  res.Columns,
		Rows:     res.Rows,
		RowCount: int64(res.RowCount),
		Duration: res.Duration,
	}, nil
}
