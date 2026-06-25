package main

import (
	"sync"

	"github.com/jbeck018/howlerops/internal/duckstage"
	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/notebooksvc"
)

// notebookStagers provides per-notebook DuckDB compute sessions for cross-cell
// composition (a downstream cell querying an upstream cell's result by handle).
// Sessions are cached per notebook so staged tables stay warm across reactive
// partial re-runs. The DuckDB backing lives in internal/duckstage behind the
// `duckdb` build tag; without it every notebook gets a NoStager and composing
// cells surface a clear "compute engine unavailable" message.
type notebookStagers struct {
	deps     *SharedDeps
	mu       sync.Mutex
	sessions map[string]notebook.Stager
}

func newNotebookStagers(deps *SharedDeps) *notebookStagers {
	return &notebookStagers{deps: deps, sessions: map[string]notebook.Stager{}}
}

var _ notebooksvc.StagerProvider = (*notebookStagers)(nil)

// StagerFor returns the warm compute session for a notebook, creating it on
// first use.
func (p *notebookStagers) StagerFor(notebookID string) notebook.Stager {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.sessions[notebookID]; ok {
		return s
	}
	s, err := duckstage.New()
	if err != nil {
		ns := notebook.NoStager{}
		p.sessions[notebookID] = ns
		return ns
	}
	p.sessions[notebookID] = s
	return s
}
