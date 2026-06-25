//go:build !duckdb

package main

import "github.com/jbeck018/howlerops/internal/notebook"

// stagerFor returns a no-op stager when the DuckDB compute engine is not
// compiled in: cross-cell composition is unavailable and composing cells report
// a clear message.
func (p *notebookStagers) stagerFor(string) notebook.Stager {
	return notebook.NoStager{}
}
