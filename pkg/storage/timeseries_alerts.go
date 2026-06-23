package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// TimeSeriesAlert is a standalone, forecasting-aware alert: it periodically runs
// a query and fires when the resulting series breaches a rule (threshold,
// anomaly, or forecast crossing). It is distinct from the report-component
// threshold alerts in pkg/alerts — those attach to a report; these monitor an
// arbitrary connection + query.
type TimeSeriesAlert struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	ConnectionID    string          `json:"connectionId"`
	SQL             string          `json:"sql"`
	TimeColumn      string          `json:"timeColumn,omitempty"`
	ValueColumn     string          `json:"valueColumn,omitempty"`
	Rule            json.RawMessage `json:"rule"` // serialized alerting.Rule
	IntervalSeconds int             `json:"intervalSeconds"`
	Channel         string          `json:"channel,omitempty"`
	Enabled         bool            `json:"enabled"`
	LastFiredAt     *time.Time      `json:"lastFiredAt,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// TimeSeriesAlertStore persists standalone time-series alerts.
type TimeSeriesAlertStore struct {
	db     *sql.DB
	logger *logrus.Logger
}

// NewTimeSeriesAlertStore constructs a store.
func NewTimeSeriesAlertStore(db *sql.DB, logger *logrus.Logger) *TimeSeriesAlertStore {
	return &TimeSeriesAlertStore{db: db, logger: logger}
}

// EnsureSchema creates the table and indexes if absent.
func (s *TimeSeriesAlertStore) EnsureSchema() error {
	if s.db == nil {
		return errors.New("alert storage database not available")
	}
	stmt := `
CREATE TABLE IF NOT EXISTS timeseries_alerts (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	connection_id TEXT NOT NULL,
	sql_query TEXT NOT NULL,
	time_column TEXT,
	value_column TEXT,
	rule TEXT NOT NULL,
	interval_seconds INTEGER NOT NULL DEFAULT 300,
	channel TEXT,
	enabled BOOLEAN NOT NULL DEFAULT 1,
	last_fired_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_timeseries_alerts_enabled ON timeseries_alerts(enabled);
`
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("failed to ensure timeseries_alerts schema: %w", err)
	}
	return nil
}

// Save inserts or updates an alert.
func (s *TimeSeriesAlertStore) Save(a *TimeSeriesAlert) error {
	if s.db == nil {
		return errors.New("alert storage database not available")
	}
	if a == nil {
		return errors.New("alert is nil")
	}
	if a.Name == "" {
		return errors.New("alert name is required")
	}
	if a.ConnectionID == "" || a.SQL == "" {
		return errors.New("alert requires a connection and query")
	}
	if len(a.Rule) == 0 {
		return errors.New("alert rule is required")
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.IntervalSeconds <= 0 {
		a.IntervalSeconds = 300
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	a.UpdatedAt = time.Now().UTC()

	var lastFired interface{}
	if a.LastFiredAt != nil {
		lastFired = a.LastFiredAt.UTC()
	}

	_, err := s.db.Exec(`
INSERT INTO timeseries_alerts (id, name, connection_id, sql_query, time_column, value_column, rule, interval_seconds, channel, enabled, last_fired_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name = excluded.name,
	connection_id = excluded.connection_id,
	sql_query = excluded.sql_query,
	time_column = excluded.time_column,
	value_column = excluded.value_column,
	rule = excluded.rule,
	interval_seconds = excluded.interval_seconds,
	channel = excluded.channel,
	enabled = excluded.enabled,
	last_fired_at = COALESCE(excluded.last_fired_at, last_fired_at),
	updated_at = excluded.updated_at;`,
		a.ID, a.Name, a.ConnectionID, a.SQL, a.TimeColumn, a.ValueColumn, string(a.Rule),
		a.IntervalSeconds, a.Channel, a.Enabled, lastFired, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save timeseries alert: %w", err)
	}
	return nil
}

func scanAlert(scanner interface{ Scan(...any) error }) (*TimeSeriesAlert, error) {
	var (
		a         TimeSeriesAlert
		rule      string
		timeCol   sql.NullString
		valueCol  sql.NullString
		channel   sql.NullString
		lastFired sql.NullTime
	)
	if err := scanner.Scan(&a.ID, &a.Name, &a.ConnectionID, &a.SQL, &timeCol, &valueCol, &rule,
		&a.IntervalSeconds, &channel, &a.Enabled, &lastFired, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.Rule = json.RawMessage(rule)
	a.TimeColumn = timeCol.String
	a.ValueColumn = valueCol.String
	a.Channel = channel.String
	if lastFired.Valid {
		t := lastFired.Time
		a.LastFiredAt = &t
	}
	return &a, nil
}

const alertColumns = `id, name, connection_id, sql_query, time_column, value_column, rule, interval_seconds, channel, enabled, last_fired_at, created_at, updated_at`

// Get loads an alert by ID, returning (nil, nil) when not found.
func (s *TimeSeriesAlertStore) Get(id string) (*TimeSeriesAlert, error) {
	if s.db == nil {
		return nil, errors.New("alert storage database not available")
	}
	a, err := scanAlert(s.db.QueryRow(`SELECT `+alertColumns+` FROM timeseries_alerts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load timeseries alert: %w", err)
	}
	return a, nil
}

// List returns all alerts; when enabledOnly is true, only enabled ones.
func (s *TimeSeriesAlertStore) List(enabledOnly bool) ([]TimeSeriesAlert, error) {
	if s.db == nil {
		return nil, errors.New("alert storage database not available")
	}
	query := `SELECT ` + alertColumns + ` FROM timeseries_alerts`
	if enabledOnly {
		query += ` WHERE enabled = 1`
	}
	query += ` ORDER BY updated_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list timeseries alerts: %w", err)
	}
	defer rows.Close()

	var out []TimeSeriesAlert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan timeseries alert: %w", err)
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Delete removes an alert.
func (s *TimeSeriesAlertStore) Delete(id string) error {
	if s.db == nil {
		return errors.New("alert storage database not available")
	}
	if _, err := s.db.Exec(`DELETE FROM timeseries_alerts WHERE id = ?`, id); err != nil {
		return fmt.Errorf("failed to delete timeseries alert: %w", err)
	}
	return nil
}

// SetEnabled toggles an alert on or off.
func (s *TimeSeriesAlertStore) SetEnabled(id string, enabled bool) error {
	if s.db == nil {
		return errors.New("alert storage database not available")
	}
	_, err := s.db.Exec(`UPDATE timeseries_alerts SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to toggle timeseries alert: %w", err)
	}
	return nil
}

// RecordFired stamps the last-fired time on an alert.
func (s *TimeSeriesAlertStore) RecordFired(id string, when time.Time) error {
	if s.db == nil {
		return errors.New("alert storage database not available")
	}
	_, err := s.db.Exec(`UPDATE timeseries_alerts SET last_fired_at = ? WHERE id = ?`, when.UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to record alert firing: %w", err)
	}
	return nil
}
