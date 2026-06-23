package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// step builds a trivial step that returns the given output.
func step(id string, out any, deps ...string) Step {
	return Step{
		ID:        id,
		DependsOn: deps,
		Run: func(_ context.Context, _ map[string]Result) (any, error) {
			return out, nil
		},
	}
}

func TestRun_LinearChainPassesOutputs(t *testing.T) {
	plan := Plan{Steps: []Step{
		step("a", 1),
		{
			ID:        "b",
			DependsOn: []string{"a"},
			Run: func(_ context.Context, deps map[string]Result) (any, error) {
				return deps["a"].Output.(int) + 10, nil
			},
		},
		{
			ID:        "c",
			DependsOn: []string{"b"},
			Run: func(_ context.Context, deps map[string]Result) (any, error) {
				return deps["b"].Output.(int) * 2, nil
			},
		},
	}}

	results, err := Run(context.Background(), plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if results["a"].Output != 1 {
		t.Errorf("a = %v, want 1", results["a"].Output)
	}
	if results["b"].Output != 11 {
		t.Errorf("b = %v, want 11", results["b"].Output)
	}
	if results["c"].Output != 22 {
		t.Errorf("c = %v, want 22", results["c"].Output)
	}
	for id, r := range results {
		if r.Status != StatusSuccess {
			t.Errorf("step %s status = %s, want success", id, r.Status)
		}
	}
}

func TestRun_FailedDependencySkipsDownstream(t *testing.T) {
	plan := Plan{Steps: []Step{
		{
			ID: "a",
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				return nil, errors.New("boom")
			},
		},
		step("b", 2, "a"),
		step("c", 3, "b"),
		step("d", 4), // independent, should still succeed
	}}

	results, err := Run(context.Background(), plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if results["a"].Status != StatusFailed {
		t.Errorf("a status = %s, want failed", results["a"].Status)
	}
	if results["b"].Status != StatusSkipped || results["c"].Status != StatusSkipped {
		t.Errorf("b/c should be skipped: b=%s c=%s", results["b"].Status, results["c"].Status)
	}
	if results["d"].Status != StatusSuccess {
		t.Errorf("independent d should succeed, got %s", results["d"].Status)
	}
	if !HasFailures(results) {
		t.Error("HasFailures should be true")
	}
}

func TestRun_StopOnErrorAbortsNotStarted(t *testing.T) {
	var cStarted atomic.Bool
	plan := Plan{Steps: []Step{
		{
			ID: "fail",
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				return nil, errors.New("nope")
			},
		},
		{
			// Independent of "fail" but should be skipped once the run aborts,
			// provided it hasn't started yet. To make ordering deterministic it
			// depends on a slow gate that finishes after "fail".
			ID:        "later",
			DependsOn: []string{"gate"},
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				cStarted.Store(true)
				return 1, nil
			},
		},
		{
			ID: "gate",
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				time.Sleep(30 * time.Millisecond)
				return 1, nil
			},
		},
	}}

	results, err := Run(context.Background(), plan, Options{StopOnError: true})
	if err != nil {
		t.Fatal(err)
	}
	if results["fail"].Status != StatusFailed {
		t.Errorf("fail status = %s", results["fail"].Status)
	}
	if results["later"].Status != StatusSkipped {
		t.Errorf("later should be skipped after abort, got %s", results["later"].Status)
	}
	if cStarted.Load() {
		t.Error("later step should not have executed")
	}
}

func TestRun_MaxParallelBounded(t *testing.T) {
	const n = 6
	var current, max atomic.Int32
	steps := make([]Step, n)
	for i := 0; i < n; i++ {
		steps[i] = Step{
			ID: fmt.Sprintf("s%d", i),
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				c := current.Add(1)
				for {
					m := max.Load()
					if c <= m || max.CompareAndSwap(m, c) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				current.Add(-1)
				return nil, nil
			},
		}
	}

	_, err := Run(context.Background(), Plan{Steps: steps}, Options{MaxParallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	if max.Load() > 2 {
		t.Errorf("observed concurrency %d, want <= 2", max.Load())
	}
}

func TestRun_IndependentStepsRunConcurrently(t *testing.T) {
	const n = 4
	var running atomic.Int32
	var peak atomic.Int32
	steps := make([]Step, n)
	for i := 0; i < n; i++ {
		steps[i] = Step{
			ID: fmt.Sprintf("s%d", i),
			Run: func(_ context.Context, _ map[string]Result) (any, error) {
				c := running.Add(1)
				for {
					p := peak.Load()
					if c <= p || peak.CompareAndSwap(p, c) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				running.Add(-1)
				return nil, nil
			},
		}
	}
	_, err := Run(context.Background(), Plan{Steps: steps}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() < 2 {
		t.Errorf("expected independent steps to overlap, peak concurrency = %d", peak.Load())
	}
}

func TestRun_StepTimeout(t *testing.T) {
	plan := Plan{Steps: []Step{
		{
			ID:      "slow",
			Timeout: 10 * time.Millisecond,
			Run: func(ctx context.Context, _ map[string]Result) (any, error) {
				select {
				case <-time.After(time.Second):
					return "done", nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		},
	}}
	results, err := Run(context.Background(), plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if results["slow"].Status != StatusFailed {
		t.Fatalf("slow status = %s, want failed", results["slow"].Status)
	}
	if !errors.Is(results["slow"].Err, context.DeadlineExceeded) {
		t.Errorf("expected deadline error, got %v", results["slow"].Err)
	}
}

func TestRun_DefaultTimeoutApplies(t *testing.T) {
	plan := Plan{Steps: []Step{
		{
			ID: "slow",
			Run: func(ctx context.Context, _ map[string]Result) (any, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	}}
	results, err := Run(context.Background(), plan, Options{DefaultTimeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if results["slow"].Status != StatusFailed {
		t.Errorf("want failed under default timeout, got %s", results["slow"].Status)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := map[string]Plan{
		"empty id":    {Steps: []Step{{ID: ""}}},
		"duplicate":   {Steps: []Step{{ID: "a"}, {ID: "a"}}},
		"unknown dep": {Steps: []Step{{ID: "a", DependsOn: []string{"x"}}}},
		"self dep":    {Steps: []Step{{ID: "a", DependsOn: []string{"a"}}}},
		"cycle": {Steps: []Step{
			{ID: "a", DependsOn: []string{"b"}},
			{ID: "b", DependsOn: []string{"a"}},
		}},
		"deep cycle": {Steps: []Step{
			{ID: "a", DependsOn: []string{"b"}},
			{ID: "b", DependsOn: []string{"c"}},
			{ID: "c", DependsOn: []string{"a"}},
		}},
	}
	for name, plan := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(plan); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestValidate_ValidDAG(t *testing.T) {
	plan := Plan{Steps: []Step{
		{ID: "a"},
		{ID: "b", DependsOn: []string{"a"}},
		{ID: "c", DependsOn: []string{"a"}},
		{ID: "d", DependsOn: []string{"b", "c"}},
	}}
	if err := Validate(plan); err != nil {
		t.Errorf("valid DAG rejected: %v", err)
	}
}

func TestRun_InvalidPlanErrors(t *testing.T) {
	if _, err := Run(context.Background(), Plan{Steps: []Step{{ID: "a", DependsOn: []string{"missing"}}}}, Options{}); err == nil {
		t.Error("expected error for invalid plan")
	}
}

func TestRun_DiamondDependency(t *testing.T) {
	// a -> b, a -> c, (b,c) -> d. d sees both b and c outputs.
	var mu sync.Mutex
	order := []string{}
	record := func(id string) {
		mu.Lock()
		order = append(order, id)
		mu.Unlock()
	}
	plan := Plan{Steps: []Step{
		{ID: "a", Run: func(_ context.Context, _ map[string]Result) (any, error) { record("a"); return 1, nil }},
		{ID: "b", DependsOn: []string{"a"}, Run: func(_ context.Context, d map[string]Result) (any, error) {
			record("b")
			return d["a"].Output.(int) + 1, nil
		}},
		{ID: "c", DependsOn: []string{"a"}, Run: func(_ context.Context, d map[string]Result) (any, error) {
			record("c")
			return d["a"].Output.(int) + 2, nil
		}},
		{ID: "d", DependsOn: []string{"b", "c"}, Run: func(_ context.Context, d map[string]Result) (any, error) {
			record("d")
			return d["b"].Output.(int) + d["c"].Output.(int), nil
		}},
	}}

	results, err := Run(context.Background(), plan, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if results["d"].Output != 5 { // (1+1) + (1+2)
		t.Errorf("d = %v, want 5", results["d"].Output)
	}
	// a must run before b/c/d.
	if order[0] != "a" {
		t.Errorf("a should run first, order = %v", order)
	}
	if order[len(order)-1] != "d" {
		t.Errorf("d should run last, order = %v", order)
	}
}
