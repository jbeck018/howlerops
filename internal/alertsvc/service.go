// Package alertsvc orchestrates standalone time-series alert persistence and
// monitoring: it stores alert definitions, hands the enabled ones to the
// internal/alertsched monitor as specs, and records firings. It is Wails-free so
// the whole flow is unit-testable; the app layer supplies the SeriesFetcher and
// Dispatcher (over the database service and event emitter) and drives the
// monitor loop.
package alertsvc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jbeck018/howlerops/internal/alerting"
	"github.com/jbeck018/howlerops/internal/alertsched"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// Store is the persistence surface, satisfied by *storage.TimeSeriesAlertStore.
type Store interface {
	Save(*storage.TimeSeriesAlert) error
	Get(id string) (*storage.TimeSeriesAlert, error)
	List(enabledOnly bool) ([]storage.TimeSeriesAlert, error)
	Delete(id string) error
	SetEnabled(id string, enabled bool) error
	RecordFired(id string, when time.Time) error
}

// Service coordinates alert CRUD and spec assembly for the monitor.
type Service struct {
	store Store
}

// New constructs a Service.
func New(store Store) *Service { return &Service{store: store} }

// AlertInput is the typed shape callers use to create/update an alert; the rule
// is a structured alerting.Rule (serialized to storage as JSON).
type AlertInput struct {
	ID              string
	Name            string
	ConnectionID    string
	SQL             string
	TimeColumn      string
	ValueColumn     string
	Channel         string
	IntervalSeconds int
	Enabled         bool
	Rule            alerting.Rule
}

// Save validates and persists an alert, returning its ID.
func (s *Service) Save(in AlertInput) (string, error) {
	ruleJSON, err := json.Marshal(in.Rule)
	if err != nil {
		return "", fmt.Errorf("alertsvc: marshal rule: %w", err)
	}
	rec := &storage.TimeSeriesAlert{
		ID:              in.ID,
		Name:            in.Name,
		ConnectionID:    in.ConnectionID,
		SQL:             in.SQL,
		TimeColumn:      in.TimeColumn,
		ValueColumn:     in.ValueColumn,
		Channel:         in.Channel,
		IntervalSeconds: in.IntervalSeconds,
		Enabled:         in.Enabled,
		Rule:            ruleJSON,
	}
	if err := s.store.Save(rec); err != nil {
		return "", err
	}
	return rec.ID, nil
}

// List returns all stored alerts.
func (s *Service) List() ([]storage.TimeSeriesAlert, error) { return s.store.List(false) }

// Get returns a stored alert by ID (nil, nil when absent).
func (s *Service) Get(id string) (*storage.TimeSeriesAlert, error) { return s.store.Get(id) }

// Delete removes an alert.
func (s *Service) Delete(id string) error { return s.store.Delete(id) }

// SetEnabled toggles an alert.
func (s *Service) SetEnabled(id string, enabled bool) error { return s.store.SetEnabled(id, enabled) }

// RecordFired stamps the firing time; called by the dispatcher when an alert
// fires.
func (s *Service) RecordFired(id string) error { return s.store.RecordFired(id, time.Now().UTC()) }

// Specs assembles monitor specs from the enabled alerts. Alerts whose rule
// cannot be decoded are skipped (and reported in the returned error) rather than
// failing the whole set.
func (s *Service) Specs() ([]alertsched.AlertSpec, error) {
	alerts, err := s.store.List(true)
	if err != nil {
		return nil, err
	}
	var specs []alertsched.AlertSpec
	var decodeErr error
	for _, a := range alerts {
		var rule alerting.Rule
		if err := json.Unmarshal(a.Rule, &rule); err != nil {
			decodeErr = fmt.Errorf("alertsvc: alert %q: bad rule: %w", a.ID, err)
			continue
		}
		interval := time.Duration(a.IntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 5 * time.Minute
		}
		specs = append(specs, alertsched.AlertSpec{
			ID:           a.ID,
			Name:         a.Name,
			ConnectionID: a.ConnectionID,
			SQL:          a.SQL,
			TimeColumn:   a.TimeColumn,
			ValueColumn:  a.ValueColumn,
			Rule:         rule,
			Interval:     interval,
		})
	}
	return specs, decodeErr
}
