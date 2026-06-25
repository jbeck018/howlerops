// Package nbmigrate performs the one-time migration that folds existing runbooks
// into notebooks, completing the merge of the two surfaces. A runbook's steps
// become notebook cells (query -> sql, action -> action, notify -> notify),
// preserving IDs, dependencies, and typed inputs. The migration is idempotent:
// a runbook is skipped if a notebook with its ID already exists, so it is safe to
// run on every startup and never clobbers a notebook the user has since edited.
package nbmigrate

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/notebook"
	"github.com/jbeck018/howlerops/internal/runbook"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// RunbookSource reads runbooks to migrate (satisfied by *storage.RunbookStore).
type RunbookSource interface {
	ListRunbooks() ([]storage.RunbookSummary, error)
	GetRunbook(id string) (*storage.Runbook, error)
}

// NotebookSink writes migrated notebooks (satisfied by *storage.NotebookStore).
type NotebookSink interface {
	GetNotebook(id string) (*storage.Notebook, error)
	SaveNotebook(*storage.Notebook) error
}

// RunbookToNotebook converts a runbook definition into an equivalent notebook.
// Steps map to cells by kind; the runbook's explicit DependsOn DAG and typed
// inputs carry over unchanged.
func RunbookToNotebook(rb runbook.Runbook) notebook.Notebook {
	nb := notebook.Notebook{
		ID:          rb.ID,
		Name:        rb.Name,
		Description: rb.Description,
		Inputs:      rb.Inputs,
	}
	for _, st := range rb.Steps {
		cell := notebook.Cell{
			ID:           st.ID,
			Title:        st.Name,
			DependsOn:    st.DependsOn,
			ConnectionID: st.ConnectionID,
			SQL:          st.SQL,
			Channel:      st.Channel,
			Message:      st.Message,
			Timeout:      st.Timeout,
		}
		switch st.Kind {
		case runbook.StepAction:
			cell.Kind = notebook.CellAction
		case runbook.StepNotify:
			cell.Kind = notebook.CellNotify
		default: // StepQuery or unspecified
			cell.Kind = notebook.CellSQL
		}
		nb.Cells = append(nb.Cells, cell)
	}
	return nb
}

// RunbooksToNotebooks migrates every runbook that does not yet have a
// corresponding notebook, returning the number migrated. A single malformed
// runbook is logged and skipped rather than aborting the whole migration.
func RunbooksToNotebooks(src RunbookSource, dst NotebookSink, logger *logrus.Logger) (int, error) {
	summaries, err := src.ListRunbooks()
	if err != nil {
		return 0, fmt.Errorf("nbmigrate: list runbooks: %w", err)
	}

	migrated := 0
	for _, s := range summaries {
		existing, err := dst.GetNotebook(s.ID)
		if err != nil {
			return migrated, fmt.Errorf("nbmigrate: check notebook %q: %w", s.ID, err)
		}
		if existing != nil {
			continue // already migrated (or user-edited) — leave it alone
		}

		rec, err := src.GetRunbook(s.ID)
		if err != nil {
			return migrated, fmt.Errorf("nbmigrate: load runbook %q: %w", s.ID, err)
		}
		if rec == nil {
			continue
		}

		var rb runbook.Runbook
		if err := json.Unmarshal(rec.Definition, &rb); err != nil {
			if logger != nil {
				logger.WithError(err).WithField("runbook", s.ID).Warn("nbmigrate: skipping unparseable runbook")
			}
			continue
		}
		rb.ID = rec.ID
		rb.Name = rec.Name
		rb.Description = rec.Description

		nb := RunbookToNotebook(rb)
		if err := notebook.Validate(nb); err != nil {
			if logger != nil {
				logger.WithError(err).WithField("runbook", s.ID).Warn("nbmigrate: skipping runbook that does not convert cleanly")
			}
			continue
		}
		def, err := json.Marshal(nb)
		if err != nil {
			return migrated, fmt.Errorf("nbmigrate: marshal notebook %q: %w", s.ID, err)
		}
		if err := dst.SaveNotebook(&storage.Notebook{
			ID:          nb.ID,
			Name:        nb.Name,
			Description: nb.Description,
			Definition:  def,
		}); err != nil {
			return migrated, fmt.Errorf("nbmigrate: save notebook %q: %w", s.ID, err)
		}
		migrated++
	}

	if migrated > 0 && logger != nil {
		logger.WithField("count", migrated).Info("nbmigrate: migrated runbooks into notebooks")
	}
	return migrated, nil
}
