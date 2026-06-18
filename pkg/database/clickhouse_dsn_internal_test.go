package database

import (
	"strings"
	"testing"
	"time"
)

// TestBuildClickHouseDSN exercises the ClickHouse DSN builder directly (internal
// package test) to lock in protocol selection and, crucially, that the
// connection-shaping params (`nativeProtocol`, `protocol`) are consumed locally
// and never forwarded to the driver as ClickHouse server settings.
func TestBuildClickHouseDSN(t *testing.T) {
	base := func() ConnectionConfig {
		return ConnectionConfig{
			Type:       ClickHouse,
			Host:       "clickhouse.example.com",
			Port:       8123,
			Database:   "default",
			Username:   "default",
			Password:   "secret",
			Parameters: map[string]string{},
		}
	}

	tests := []struct {
		name           string
		mutate         func(c *ConnectionConfig)
		wantScheme     string // required prefix "<scheme>://"
		wantContains   []string
		wantNotContain []string
	}{
		{
			name:           "http port 8123 stays plain http",
			mutate:         func(c *ConnectionConfig) { c.Port = 8123 },
			wantScheme:     "http",
			wantNotContain: []string{"secure=true"},
		},
		{
			name: "nativeProtocol=false on http port does not leak the setting",
			mutate: func(c *ConnectionConfig) {
				c.Port = 8123
				c.SSLMode = "require" // mirrors the reported failing config
				c.Parameters["nativeProtocol"] = "false"
			},
			wantScheme:     "http",
			wantNotContain: []string{"nativeProtocol"},
		},
		{
			name: "nativeProtocol=true selects native protocol",
			mutate: func(c *ConnectionConfig) {
				c.Port = 9000
				c.Parameters["nativeProtocol"] = "true"
			},
			wantScheme:     "clickhouse",
			wantNotContain: []string{"nativeProtocol"},
		},
		{
			name: "8443 https cloud port",
			mutate: func(c *ConnectionConfig) {
				c.Port = 8443
				c.Parameters["nativeProtocol"] = "false"
			},
			wantScheme:     "https",
			wantContains:   []string{"secure=true"},
			wantNotContain: []string{"nativeProtocol"},
		},
		{
			name: "9440 native over tls",
			mutate: func(c *ConnectionConfig) {
				c.Port = 9440
				c.SSLMode = "require"
			},
			wantScheme:   "clickhouse",
			wantContains: []string{"secure=true"},
		},
		{
			name: "skip-verify adds skip_verify",
			mutate: func(c *ConnectionConfig) {
				c.Port = 9440
				c.SSLMode = "skip-verify"
			},
			wantScheme:   "clickhouse",
			wantContains: []string{"secure=true", "skip_verify=true"},
		},
		{
			name: "explicit protocol override wins",
			mutate: func(c *ConnectionConfig) {
				c.Port = 8123
				c.Parameters["nativeProtocol"] = "false"
				c.Parameters["protocol"] = "native"
			},
			wantScheme:     "clickhouse",
			wantNotContain: []string{"protocol=", "nativeProtocol"},
		},
		{
			name: "dial_timeout from connection timeout",
			mutate: func(c *ConnectionConfig) {
				c.Port = 8123
				c.ConnectionTimeout = 15 * time.Second
			},
			wantScheme:   "http",
			wantContains: []string{"dial_timeout=15s"},
		},
		{
			name: "unrelated custom params still pass through",
			mutate: func(c *ConnectionConfig) {
				c.Port = 8123
				c.Parameters["nativeProtocol"] = "false"
				c.Parameters["max_execution_time"] = "60"
			},
			wantScheme:     "http",
			wantContains:   []string{"max_execution_time=60"},
			wantNotContain: []string{"nativeProtocol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base()
			tt.mutate(&cfg)
			p := &ConnectionPool{config: cfg}

			dsn := p.buildClickHouseDSN()

			if !strings.HasPrefix(dsn, tt.wantScheme+"://") {
				t.Fatalf("scheme mismatch: got %q, want prefix %q://", dsn, tt.wantScheme)
			}
			for _, sub := range tt.wantContains {
				if !strings.Contains(dsn, sub) {
					t.Errorf("DSN %q missing expected substring %q", dsn, sub)
				}
			}
			for _, sub := range tt.wantNotContain {
				if strings.Contains(dsn, sub) {
					t.Errorf("DSN %q must not contain %q (it leaks into ClickHouse as a setting)", dsn, sub)
				}
			}
		})
	}
}
