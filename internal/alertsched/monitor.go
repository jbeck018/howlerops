// Package alertsched periodically evaluates time-series alert rules and
// dispatches notifications when they fire. It is the scheduler/trigger half of
// the platform's alerting (the evaluation logic lives in internal/alerting): a
// local-first, deploy-ready loop that runs identically in the desktop app or a
// server worker because it depends only on injected SeriesFetcher and Dispatcher
// interfaces, never on Wails or a database driver directly.
//
// Alerts are edge-triggered: a firing rule dispatches once on the not-firing ->
// firing transition and re-arms only after it clears, so a persistently breached
// threshold does not spam notifications every tick.
package alertsched

import (
	"context"
	"sync"
	"time"

	"github.com/jbeck018/howlerops/internal/alerting"
	"github.com/jbeck018/howlerops/internal/forecast"
)

// SeriesFetcher loads the series an alert evaluates. timeCol/valueCol may be
// empty to request auto-detection.
type SeriesFetcher interface {
	Fetch(ctx context.Context, connectionID, sql, timeCol, valueCol string) (forecast.Series, error)
}

// Dispatcher delivers a fired alert (e.g. a notification, a runbook trigger).
type Dispatcher interface {
	Dispatch(ctx context.Context, spec AlertSpec, event alerting.Event) error
}

// AlertSpec is one monitored alert.
type AlertSpec struct {
	ID           string
	Name         string
	ConnectionID string
	SQL          string
	TimeColumn   string
	ValueColumn  string
	Rule         alerting.Rule
	// Interval is advisory metadata for callers driving Run; Check ignores it.
	Interval time.Duration
}

// Monitor evaluates alerts and tracks firing state for edge-triggering.
type Monitor struct {
	fetch    SeriesFetcher
	dispatch Dispatcher

	mu    sync.Mutex
	fired map[string]bool // alert ID -> currently firing
}

// New constructs a Monitor.
func New(fetch SeriesFetcher, dispatch Dispatcher) *Monitor {
	return &Monitor{fetch: fetch, dispatch: dispatch, fired: map[string]bool{}}
}

// CheckResult reports what one evaluation did.
type CheckResult struct {
	Fired      bool
	Dispatched bool // true only on a fresh not-firing -> firing transition
	Event      alerting.Event
}

// Check runs one evaluation cycle for a spec: fetch the series, evaluate the
// rule, and dispatch on a rising edge. It is safe for concurrent use across
// different specs.
func (m *Monitor) Check(ctx context.Context, spec AlertSpec) (CheckResult, error) {
	series, err := m.fetch.Fetch(ctx, spec.ConnectionID, spec.SQL, spec.TimeColumn, spec.ValueColumn)
	if err != nil {
		return CheckResult{}, err
	}
	event, err := alerting.Evaluate(series, spec.Rule)
	if err != nil {
		return CheckResult{}, err
	}

	m.mu.Lock()
	was := m.fired[spec.ID]
	m.fired[spec.ID] = event.Fired
	m.mu.Unlock()

	res := CheckResult{Fired: event.Fired, Event: event}
	if event.Fired && !was {
		if m.dispatch != nil {
			if err := m.dispatch.Dispatch(ctx, spec, event); err != nil {
				// Roll back the firing state so the next tick retries dispatch.
				m.mu.Lock()
				m.fired[spec.ID] = was
				m.mu.Unlock()
				return res, err
			}
		}
		res.Dispatched = true
	}
	return res, nil
}

// Reset clears the remembered firing state for an alert (e.g. after it is
// edited or removed), so it can fire again on the next breach.
func (m *Monitor) Reset(alertID string) {
	m.mu.Lock()
	delete(m.fired, alertID)
	m.mu.Unlock()
}

// Run drives periodic evaluation until ctx is cancelled. On each tick it
// evaluates every spec the provider returns (which may change between ticks as
// the user adds/removes alerts). Fetch/evaluate errors for one spec do not stop
// the loop. This same loop runs locally or server-side.
func (m *Monitor) Run(ctx context.Context, interval time.Duration, provider func() []AlertSpec) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, spec := range provider() {
				_, _ = m.Check(ctx, spec)
			}
		}
	}
}
