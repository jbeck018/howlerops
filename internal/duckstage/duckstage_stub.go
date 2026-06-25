//go:build !duckdb

// Package duckstage's stub build: without the `duckdb` tag the embedded compute
// engine is unavailable, so New reports as much and callers fall back to a
// no-op stager, leaving composing cells to surface a clear "compute engine
// unavailable" message.
package duckstage

import (
	"errors"

	"github.com/jbeck018/howlerops/internal/notebook"
)

// Stager is unavailable in this build. It embeds notebook.NoStager so it still
// satisfies notebook.Stager for code shared across build tags, but New never
// returns a usable instance.
type Stager struct {
	notebook.NoStager
}

// New reports that the DuckDB compute engine is not compiled in.
func New() (*Stager, error) {
	return nil, errors.New("duckstage: DuckDB compute engine not available (build without the 'duckdb' tag)")
}
