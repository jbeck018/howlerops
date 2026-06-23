package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/jbeck018/howlerops/internal/alerting"
	"github.com/jbeck018/howlerops/internal/alertsched"
	"github.com/jbeck018/howlerops/internal/alertsvc"
	"github.com/jbeck018/howlerops/internal/forecast"
	"github.com/jbeck018/howlerops/pkg/database"
	"github.com/jbeck018/howlerops/pkg/storage"
)

// alertTickInterval is how often the monitor evaluates all enabled alerts. The
// per-alert Interval is advisory metadata; a single global tick keeps the loop
// simple for the desktop app.
const alertTickInterval = time.Minute

// WailsAlertService exposes standalone time-series alert CRUD to the frontend
// and runs the background monitor that fires them. It wraps the Wails-free
// internal/alertsvc orchestration and internal/alertsched monitor, supplying a
// SeriesFetcher over the database service and a Dispatcher over the event
// emitter.
type WailsAlertService struct {
	deps *SharedDeps

	mu      sync.Mutex
	svc     *alertsvc.Service
	monitor *alertsched.Monitor
	cancel  context.CancelFunc
}

// NewWailsAlertService constructs the service; the store is wired in later.
func NewWailsAlertService(deps *SharedDeps) *WailsAlertService {
	return &WailsAlertService{deps: deps}
}

// SetStore injects the alert store once storage is ready.
func (s *WailsAlertService) SetStore(store *storage.TimeSeriesAlertStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.svc = alertsvc.New(store)
}

// Start launches the background monitor loop. It is safe to call once after the
// store is set; subsequent calls are no-ops while running.
func (s *WailsAlertService) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.svc == nil || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	svc := s.svc
	s.monitor = alertsched.New(&alertSeriesFetcher{deps: s.deps}, &alertDispatcher{deps: s.deps, svc: svc})
	monitor := s.monitor
	go monitor.Run(ctx, alertTickInterval, func() []alertsched.AlertSpec {
		specs, err := svc.Specs()
		if err != nil && s.deps.Logger != nil {
			s.deps.Logger.WithError(err).Debug("alert spec decode issue")
		}
		return specs
	})
}

// Stop halts the monitor loop (called on shutdown).
func (s *WailsAlertService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *WailsAlertService) service() (*alertsvc.Service, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.svc == nil {
		return nil, fmt.Errorf("alert storage is not initialized")
	}
	return s.svc, nil
}

// AlertRequest is the wire form for creating/updating an alert.
type AlertRequest struct {
	ID              string        `json:"id,omitempty"`
	Name            string        `json:"name"`
	ConnectionID    string        `json:"connectionId"`
	SQL             string        `json:"sql"`
	TimeColumn      string        `json:"timeColumn,omitempty"`
	ValueColumn     string        `json:"valueColumn,omitempty"`
	Channel         string        `json:"channel,omitempty"`
	IntervalSeconds int           `json:"intervalSeconds,omitempty"`
	Enabled         bool          `json:"enabled"`
	Rule            alerting.Rule `json:"rule"`
}

// SaveAlert validates and persists an alert, returning its ID.
func (s *WailsAlertService) SaveAlert(req AlertRequest) (string, error) {
	svc, err := s.service()
	if err != nil {
		return "", err
	}
	return svc.Save(alertsvc.AlertInput{
		ID:              req.ID,
		Name:            req.Name,
		ConnectionID:    req.ConnectionID,
		SQL:             req.SQL,
		TimeColumn:      req.TimeColumn,
		ValueColumn:     req.ValueColumn,
		Channel:         req.Channel,
		IntervalSeconds: req.IntervalSeconds,
		Enabled:         req.Enabled,
		Rule:            req.Rule,
	})
}

// ListAlerts returns all stored alerts.
func (s *WailsAlertService) ListAlerts() ([]storage.TimeSeriesAlert, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	return svc.List()
}

// GetAlert loads an alert by ID.
func (s *WailsAlertService) GetAlert(id string) (*storage.TimeSeriesAlert, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	a, err := svc.Get(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("alert %q not found", id)
	}
	return a, nil
}

// DeleteAlert removes an alert.
func (s *WailsAlertService) DeleteAlert(id string) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.Delete(id)
}

// SetAlertEnabled toggles an alert on or off.
func (s *WailsAlertService) SetAlertEnabled(id string, enabled bool) error {
	svc, err := s.service()
	if err != nil {
		return err
	}
	return svc.SetEnabled(id, enabled)
}

// AlertCheckResponse is the result of an immediate evaluation.
type AlertCheckResponse struct {
	Fired   bool    `json:"fired"`
	Kind    string  `json:"kind,omitempty"`
	Message string  `json:"message,omitempty"`
	Value   float64 `json:"value,omitempty"`
}

// CheckAlertNow evaluates an alert immediately (without affecting the monitor's
// firing state), so the UI can preview whether a rule currently fires.
func (s *WailsAlertService) CheckAlertNow(id string) (*AlertCheckResponse, error) {
	svc, err := s.service()
	if err != nil {
		return nil, err
	}
	a, err := svc.Get(id)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, fmt.Errorf("alert %q not found", id)
	}
	var rule alerting.Rule
	if err := json.Unmarshal(a.Rule, &rule); err != nil {
		return nil, fmt.Errorf("alert has an invalid rule: %w", err)
	}
	fetcher := &alertSeriesFetcher{deps: s.deps}
	series, err := fetcher.Fetch(context.Background(), a.ConnectionID, a.SQL, a.TimeColumn, a.ValueColumn)
	if err != nil {
		return nil, err
	}
	event, err := alerting.Evaluate(series, rule)
	if err != nil {
		return nil, err
	}
	return &AlertCheckResponse{
		Fired:   event.Fired,
		Kind:    string(event.Kind),
		Message: event.Message,
		Value:   event.Value,
	}, nil
}

// alertSeriesFetcher loads an alert's series from the database service.
type alertSeriesFetcher struct{ deps *SharedDeps }

func (f *alertSeriesFetcher) Fetch(_ context.Context, connectionID, sql, timeCol, valueCol string) (forecast.Series, error) {
	res, err := f.deps.DatabaseService.ExecuteQuery(connectionID, sql, &database.QueryOptions{
		ReadOnly: true,
		Timeout:  60 * time.Second,
		Limit:    5000,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("alert query returned no result")
	}
	rows := make([]map[string]any, 0, len(res.Rows))
	for _, row := range res.Rows {
		m := make(map[string]any, len(res.Columns))
		for i, col := range res.Columns {
			if i < len(row) {
				m[col] = row[i]
			}
		}
		rows = append(rows, m)
	}
	if timeCol == "" || valueCol == "" {
		tc, vc, ok := forecast.DetectColumns(res.Columns, rows)
		if timeCol == "" {
			timeCol = tc
		}
		if valueCol == "" {
			valueCol = vc
		}
		if !ok && (timeCol == "" || valueCol == "") {
			return nil, fmt.Errorf("could not determine time/value columns for the alert query")
		}
	}
	series, _, err := forecast.SeriesFromRows(res.Columns, rows, timeCol, valueCol)
	return series, err
}

// alertDispatcher records firings and emits a frontend event when an alert fires.
type alertDispatcher struct {
	deps *SharedDeps
	svc  *alertsvc.Service
}

func (d *alertDispatcher) Dispatch(_ context.Context, spec alertsched.AlertSpec, event alerting.Event) error {
	if err := d.svc.RecordFired(spec.ID); err != nil && d.deps.Logger != nil {
		d.deps.Logger.WithError(err).Debug("failed to record alert firing")
	}
	d.deps.emitEvent("alert:fired", map[string]interface{}{
		"alertId": spec.ID,
		"name":    spec.Name,
		"kind":    string(event.Kind),
		"message": event.Message,
		"value":   event.Value,
		"at":      event.At,
	})
	return nil
}
