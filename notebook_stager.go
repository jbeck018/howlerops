package main

import (
	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/notebooksvc"
)

// notebookStagers provides per-notebook DuckDB compute sessions used for
// cross-cell composition (a downstream cell querying an upstream cell's result
// by handle). Sessions are kept per notebook so staged tables stay warm across
// reactive partial re-runs.
//
// DuckDB-backed staging is wired in notebook_stager_duckdb.go behind the
// `duckdb` build tag; in builds without it, StagerFor returns a NoStager and
// composing cells surface a clear "compute engine unavailable" message.
type notebookStagers struct {
	deps *SharedDeps
}

func newNotebookStagers(deps *SharedDeps) *notebookStagers {
	return &notebookStagers{deps: deps}
}

var _ notebooksvc.StagerProvider = (*notebookStagers)(nil)

func (p *notebookStagers) StagerFor(notebookID string) notebook.Stager {
	return p.stagerFor(notebookID)
}
