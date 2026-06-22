package alertsched

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jbeck018/howlerops/internal/alerting"
	"github.com/jbeck018/howlerops/internal/forecast"
)

// fakeFetcher returns a programmable series (or error).
type fakeFetcher struct {
	mu     sync.Mutex
	series forecast.Series
	err    error
}

func (f *fakeFetcher) set(values ...float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := make(forecast.Series, len(values))
	for i, v := range values {
		s[i] = forecast.Point{Time: start.AddDate(0, 0, i), Value: v}
	}
	f.series = s
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _, _ string) (forecast.Series, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.series, f.err
}

// countingDispatcher records dispatched events.
type countingDispatcher struct {
	mu     sync.Mutex
	count  int
	events []alerting.Event
	err    error
}

func (d *countingDispatcher) Dispatch(_ context.Context, _ AlertSpec, ev alerting.Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.count++
	d.events = append(d.events, ev)
	return nil
}

func thresholdSpec() AlertSpec {
	return AlertSpec{
		ID:   "a1",
		Name: "high",
		Rule: alerting.Rule{Threshold: &alerting.ThresholdRule{Comparator: alerting.GT, Value: 100}},
	}
}

func TestMonitor_EdgeTriggered(t *testing.T) {
	f := &fakeFetcher{}
	d := &countingDispatcher{}
	m := New(f, d)
	spec := thresholdSpec()

	// Not breached -> no dispatch.
	f.set(10, 20, 30)
	r, err := m.Check(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if r.Fired || r.Dispatched || d.count != 0 {
		t.Fatalf("should not fire below threshold: %+v count=%d", r, d.count)
	}

	// Breach -> dispatch once.
	f.set(10, 20, 150)
	r, _ = m.Check(context.Background(), spec)
	if !r.Fired || !r.Dispatched || d.count != 1 {
		t.Fatalf("expected one dispatch on rising edge: %+v count=%d", r, d.count)
	}

	// Still breached -> no second dispatch (deduped).
	f.set(10, 20, 160)
	r, _ = m.Check(context.Background(), spec)
	if !r.Fired || r.Dispatched || d.count != 1 {
		t.Fatalf("should dedupe while still firing: %+v count=%d", r, d.count)
	}

	// Clears.
	f.set(10, 20, 30)
	r, _ = m.Check(context.Background(), spec)
	if r.Fired || r.Dispatched || d.count != 1 {
		t.Fatalf("should not dispatch on clear: %+v count=%d", r, d.count)
	}

	// Re-arms and fires again.
	f.set(10, 20, 200)
	r, _ = m.Check(context.Background(), spec)
	if !r.Dispatched || d.count != 2 {
		t.Fatalf("expected re-fire after clear: %+v count=%d", r, d.count)
	}
}

func TestMonitor_FetchErrorPropagates(t *testing.T) {
	f := &fakeFetcher{err: errors.New("db down")}
	m := New(f, &countingDispatcher{})
	if _, err := m.Check(context.Background(), thresholdSpec()); err == nil {
		t.Error("expected fetch error")
	}
}

func TestMonitor_DispatchErrorRetriesNextTick(t *testing.T) {
	f := &fakeFetcher{}
	d := &countingDispatcher{err: errors.New("notify failed")}
	m := New(f, d)
	spec := thresholdSpec()

	f.set(10, 200)
	if _, err := m.Check(context.Background(), spec); err == nil {
		t.Fatal("expected dispatch error")
	}
	// Firing state rolled back -> a subsequent successful dispatch fires.
	d.err = nil
	r, err := m.Check(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Dispatched || d.count != 1 {
		t.Errorf("expected retry to dispatch after earlier failure: %+v count=%d", r, d.count)
	}
}

func TestMonitor_Reset(t *testing.T) {
	f := &fakeFetcher{}
	d := &countingDispatcher{}
	m := New(f, d)
	spec := thresholdSpec()

	f.set(200)
	m.Check(context.Background(), spec)
	if d.count != 1 {
		t.Fatalf("expected first dispatch, got %d", d.count)
	}
	// Reset forgets the firing state, so the same breach fires again.
	m.Reset(spec.ID)
	m.Check(context.Background(), spec)
	if d.count != 2 {
		t.Errorf("expected re-dispatch after reset, got %d", d.count)
	}
}

func TestMonitor_AnomalyRule(t *testing.T) {
	f := &fakeFetcher{}
	d := &countingDispatcher{}
	m := New(f, d)
	spec := AlertSpec{ID: "an", Rule: alerting.Rule{Anomaly: &alerting.AnomalyRule{Lookback: 3}}}

	vals := make([]float64, 40)
	for i := range vals {
		vals[i] = 50
	}
	vals[39] = 999
	f.set(vals...)
	r, err := m.Check(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Dispatched {
		t.Errorf("expected anomaly dispatch: %+v", r)
	}
}

func TestMonitor_RunTicksAndStops(t *testing.T) {
	f := &fakeFetcher{}
	f.set(10, 200) // breached
	d := &countingDispatcher{}
	m := New(f, d)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx, 5*time.Millisecond, func() []AlertSpec { return []AlertSpec{thresholdSpec()} })
		close(done)
	}()

	// Wait until at least one dispatch happens, then cancel.
	deadline := time.After(2 * time.Second)
	for {
		d.mu.Lock()
		c := d.count
		d.mu.Unlock()
		if c >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Run did not dispatch within deadline")
		case <-time.After(2 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
