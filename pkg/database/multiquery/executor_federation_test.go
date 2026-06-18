package multiquery

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// mockFederationBackend records what it was asked to attach and the SQL it was
// asked to execute, returning a canned result.
type mockFederationBackend struct {
	attached []string
	gotSQL   string
	result   *QueryResult
}

func (m *mockFederationBackend) EnsureAttached(_ context.Context, names []string) (map[string]string, error) {
	m.attached = names
	out := map[string]string{}
	for _, n := range names {
		out[n] = "db_" + strings.ToLower(n)
	}
	return out, nil
}

func (m *mockFederationBackend) Execute(_ context.Context, sql string, _ time.Duration) (*QueryResult, error) {
	m.gotSQL = sql
	return m.result, nil
}

func newTestExecutorConfig() *Config {
	return &Config{Enabled: true, Timeout: 5 * time.Second, MaxResultRows: 100}
}

func TestExecutor_Federated_UsesBackendAndRewrites(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(stderrDiscard{})

	backend := &mockFederationBackend{
		result: &QueryResult{
			Columns:  []string{"name", "total"},
			Rows:     [][]interface{}{{"ada", int64(150)}},
			RowCount: 1,
		},
	}
	exec := NewExecutorWithFederation(newTestExecutorConfig(), logger, backend)

	parser := NewQueryParser(newTestExecutorConfig(), logger)
	parsed, err := parser.Parse("SELECT u.name, SUM(o.amount) total FROM @a.public.users u JOIN @b.public.orders o ON u.id=o.user_id GROUP BY u.name")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res, err := exec.Execute(context.Background(), parsed, map[string]Database{}, &Options{Strategy: StrategyFederated, Timeout: time.Second})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if len(backend.attached) != 2 {
		t.Fatalf("expected both connections attached, got %v", backend.attached)
	}
	// @a / @b must have been rewritten to the attached aliases, and the literal
	// table refs preserved.
	if !strings.Contains(backend.gotSQL, `"db_a".public.users`) || !strings.Contains(backend.gotSQL, `"db_b".public.orders`) {
		t.Fatalf("rewrite not applied to federation SQL: %s", backend.gotSQL)
	}
	if strings.Contains(backend.gotSQL, "@a.") || strings.Contains(backend.gotSQL, "@b.") {
		t.Fatalf("@conn refs not rewritten: %s", backend.gotSQL)
	}
	if res.Strategy != StrategyFederated {
		t.Fatalf("expected federated strategy, got %s", res.Strategy)
	}
	if res.Editable != nil {
		t.Fatalf("federated results must not be editable")
	}
	if res.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", res.RowCount)
	}
}

func TestExecutor_Federated_FallsBackWhenNoBackend(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(stderrDiscard{})

	// No federation backend -> legacy path, which executes on the first
	// connection. We provide a stub Database that echoes a result.
	exec := NewExecutor(newTestExecutorConfig(), logger)
	parser := NewQueryParser(newTestExecutorConfig(), logger)
	parsed, err := parser.Parse("SELECT * FROM @a.users u JOIN @b.orders o ON u.id=o.id")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	conns := map[string]Database{
		"a": stubDatabase{cols: []string{"id"}},
		"b": stubDatabase{cols: []string{"id"}},
	}
	res, err := exec.Execute(context.Background(), parsed, conns, &Options{Strategy: StrategyFederated, Timeout: time.Second})
	if err != nil {
		t.Fatalf("legacy execute: %v", err)
	}
	if res == nil || res.Strategy != StrategyFederated {
		t.Fatalf("expected a legacy federated result, got %+v", res)
	}
}

type stubDatabase struct{ cols []string }

func (s stubDatabase) Execute(_ context.Context, _ string, _ ...interface{}) (*QueryResult, error) {
	return &QueryResult{Columns: s.cols, Rows: [][]interface{}{{int64(1)}}, RowCount: 1}, nil
}

type stderrDiscard struct{}

func (stderrDiscard) Write(p []byte) (int, error) { return len(p), nil }
