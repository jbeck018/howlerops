package storage

import (
	"database/sql"
	"encoding/json"
	"io"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

func newAlertStore(t *testing.T) *TimeSeriesAlertStore {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	store := NewTimeSeriesAlertStore(db, logger)
	if err := store.EnsureSchema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return store
}

func sampleAlert() *TimeSeriesAlert {
	return &TimeSeriesAlert{
		Name:         "revenue spike",
		ConnectionID: "c1",
		SQL:          "SELECT day, revenue FROM sales",
		ValueColumn:  "revenue",
		Rule:         json.RawMessage(`{"threshold":{"comparator":"gt","value":1000}}`),
		Channel:      "ops",
		Enabled:      true,
	}
}

func TestTimeSeriesAlertStore_SaveGet(t *testing.T) {
	store := newAlertStore(t)
	a := sampleAlert()
	if err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.IntervalSeconds != 300 {
		t.Errorf("defaults not applied: id=%q interval=%d", a.ID, a.IntervalSeconds)
	}

	got, err := store.Get(a.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "revenue spike" || got.ValueColumn != "revenue" || got.Channel != "ops" {
		t.Errorf("loaded mismatch: %+v", got)
	}
	if string(got.Rule) != string(a.Rule) {
		t.Errorf("rule round-trip: %s", got.Rule)
	}
	if !got.Enabled {
		t.Error("expected enabled")
	}
}

func TestTimeSeriesAlertStore_GetMissing(t *testing.T) {
	store := newAlertStore(t)
	got, err := store.Get("nope")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestTimeSeriesAlertStore_Validation(t *testing.T) {
	store := newAlertStore(t)
	cases := []*TimeSeriesAlert{
		{ConnectionID: "c", SQL: "x", Rule: json.RawMessage(`{}`)},  // no name
		{Name: "n", SQL: "x", Rule: json.RawMessage(`{}`)},          // no connection
		{Name: "n", ConnectionID: "c", Rule: json.RawMessage(`{}`)}, // no sql
		{Name: "n", ConnectionID: "c", SQL: "x"},                    // no rule
		nil,
	}
	for i, a := range cases {
		if err := store.Save(a); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestTimeSeriesAlertStore_ListEnabledOnly(t *testing.T) {
	store := newAlertStore(t)
	on := sampleAlert()
	if err := store.Save(on); err != nil {
		t.Fatal(err)
	}
	off := sampleAlert()
	off.Name = "disabled one"
	off.Enabled = false
	if err := store.Save(off); err != nil {
		t.Fatal(err)
	}

	all, err := store.List(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 alerts, got %d", len(all))
	}
	enabled, err := store.List(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(enabled) != 1 || enabled[0].ID != on.ID {
		t.Errorf("enabled-only list wrong: %+v", enabled)
	}
}

func TestTimeSeriesAlertStore_SetEnabledAndDelete(t *testing.T) {
	store := newAlertStore(t)
	a := sampleAlert()
	if err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEnabled(a.ID, false); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(a.ID)
	if got.Enabled {
		t.Error("expected disabled after SetEnabled(false)")
	}
	if err := store.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if g, _ := store.Get(a.ID); g != nil {
		t.Error("expected gone after delete")
	}
}

func TestTimeSeriesAlertStore_RecordFired(t *testing.T) {
	store := newAlertStore(t)
	a := sampleAlert()
	if err := store.Save(a); err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC()
	if err := store.RecordFired(a.ID, when); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(a.ID)
	if got.LastFiredAt == nil {
		t.Error("expected last_fired_at to be set")
	}
}
