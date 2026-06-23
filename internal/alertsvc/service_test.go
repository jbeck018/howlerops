package alertsvc

import (
	"database/sql"
	"io"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/internal/alerting"
	"github.com/jbeck018/howlerops/pkg/storage"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := storage.NewTimeSeriesAlertStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return New(store)
}

func thresholdInput() AlertInput {
	return AlertInput{
		Name:            "high revenue",
		ConnectionID:    "c1",
		SQL:             "SELECT day, revenue FROM sales",
		ValueColumn:     "revenue",
		IntervalSeconds: 60,
		Enabled:         true,
		Rule:            alerting.Rule{Threshold: &alerting.ThresholdRule{Comparator: alerting.GT, Value: 1000}},
	}
}

func TestService_SaveAndSpecs(t *testing.T) {
	svc := newSvc(t)
	id, err := svc.Save(thresholdInput())
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected an ID")
	}

	specs, err := svc.Specs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.ID != id || spec.ConnectionID != "c1" || spec.ValueColumn != "revenue" {
		t.Errorf("spec mismatch: %+v", spec)
	}
	if spec.Rule.Threshold == nil || spec.Rule.Threshold.Comparator != alerting.GT || spec.Rule.Threshold.Value != 1000 {
		t.Errorf("rule did not round-trip: %+v", spec.Rule)
	}
	if spec.Interval.Seconds() != 60 {
		t.Errorf("interval = %v, want 60s", spec.Interval)
	}
}

func TestService_SpecsExcludesDisabled(t *testing.T) {
	svc := newSvc(t)
	if _, err := svc.Save(thresholdInput()); err != nil {
		t.Fatal(err)
	}
	off := thresholdInput()
	off.Name = "off"
	off.Enabled = false
	if _, err := svc.Save(off); err != nil {
		t.Fatal(err)
	}

	specs, err := svc.Specs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Errorf("Specs should only include enabled alerts, got %d", len(specs))
	}
	all, _ := svc.List()
	if len(all) != 2 {
		t.Errorf("List should include all, got %d", len(all))
	}
}

func TestService_SpecsSkipsBadRule(t *testing.T) {
	svc := newSvc(t)
	// Save a good one via the service.
	if _, err := svc.Save(thresholdInput()); err != nil {
		t.Fatal(err)
	}
	// Inject a malformed rule directly through the store.
	bad := &storage.TimeSeriesAlert{
		Name: "bad", ConnectionID: "c", SQL: "x", Enabled: true,
		Rule: []byte(`{not json`),
	}
	if err := svc.store.Save(bad); err != nil {
		t.Fatal(err)
	}

	specs, err := svc.Specs()
	if err == nil {
		t.Error("expected a decode error to be reported")
	}
	// The good alert is still returned despite the bad one.
	if len(specs) != 1 {
		t.Errorf("expected 1 good spec, got %d", len(specs))
	}
}

func TestService_EnableDisableDelete(t *testing.T) {
	svc := newSvc(t)
	id, err := svc.Save(thresholdInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetEnabled(id, false); err != nil {
		t.Fatal(err)
	}
	specs, _ := svc.Specs()
	if len(specs) != 0 {
		t.Errorf("disabled alert should not appear in specs, got %d", len(specs))
	}
	if err := svc.RecordFired(id); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(id)
	if got.LastFiredAt == nil {
		t.Error("expected last fired to be recorded")
	}
	if err := svc.Delete(id); err != nil {
		t.Fatal(err)
	}
	if g, _ := svc.Get(id); g != nil {
		t.Error("expected gone after delete")
	}
}

func TestService_AnomalyRuleRoundTrips(t *testing.T) {
	svc := newSvc(t)
	in := thresholdInput()
	in.Rule = alerting.Rule{Anomaly: &alerting.AnomalyRule{SeasonLength: 7, Lookback: 3, MinScore: 4}}
	if _, err := svc.Save(in); err != nil {
		t.Fatal(err)
	}
	specs, err := svc.Specs()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Rule.Anomaly == nil {
		t.Fatalf("anomaly rule lost: %+v", specs)
	}
	if specs[0].Rule.Anomaly.SeasonLength != 7 || specs[0].Rule.Anomaly.Lookback != 3 {
		t.Errorf("anomaly fields lost: %+v", specs[0].Rule.Anomaly)
	}
}
