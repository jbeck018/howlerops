// Package runner is HowlerOps' reusable step-execution engine. It runs a set of
// steps that may depend on one another (a DAG), executing independent steps
// concurrently, passing each step its upstream results, and skipping steps whose
// dependencies failed. It is the "build once" execution primitive behind report
// components, runbook steps, and notebook cells.
//
// It is deliberately generic and Wails-free: a Step's work is an arbitrary
// StepFunc, so callers map their own domain (a report component, a runbook
// action) onto it. The engine handles ordering, parallelism, timeouts, and
// failure propagation.
package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status is the terminal state of a step.
type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped" // a dependency failed/was skipped, or the run aborted
)

// StepFunc performs a step's work. It receives the results of the step's direct
// dependencies (keyed by step ID) so it can consume upstream output.
type StepFunc func(ctx context.Context, deps map[string]Result) (any, error)

// Step is one unit of work in a Plan.
type Step struct {
	ID        string
	DependsOn []string
	Run       StepFunc
	// Timeout bounds this step; 0 falls back to Options.DefaultTimeout (0 = no
	// timeout).
	Timeout time.Duration
}

// Result is the outcome of a step.
type Result struct {
	ID         string
	Status     Status
	Output     any
	Err        error
	SkipReason string
	StartedAt  time.Time
	FinishedAt time.Time
}

// Duration is how long the step ran (zero for skipped steps).
func (r Result) Duration() time.Duration {
	if r.StartedAt.IsZero() || r.FinishedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}

// Plan is the set of steps to execute.
type Plan struct {
	Steps []Step
}

// Options tune execution.
type Options struct {
	// MaxParallel caps concurrently running steps; 0 means unbounded.
	MaxParallel int
	// DefaultTimeout applies to steps that don't set their own; 0 means none.
	DefaultTimeout time.Duration
	// StopOnError, when true, stops scheduling new steps after the first
	// failure; not-yet-started steps are marked skipped. In-flight steps are
	// allowed to finish.
	StopOnError bool
}

// Run executes the plan and returns each step's result keyed by ID. The returned
// error is non-nil only for an invalid plan (duplicate/empty IDs, unknown
// dependency, or a cycle); individual step failures are reported in the results.
func Run(ctx context.Context, plan Plan, opts Options) (map[string]Result, error) {
	if err := Validate(plan); err != nil {
		return nil, err
	}

	var mu sync.Mutex
	results := make(map[string]Result, len(plan.Steps))
	started := make(map[string]bool, len(plan.Steps))
	remaining := len(plan.Steps)
	inflight := 0
	abort := false
	done := make(chan string, len(plan.Steps))

	var sem chan struct{}
	if opts.MaxParallel > 0 {
		sem = make(chan struct{}, opts.MaxParallel)
	}

	for remaining > 0 {
		mu.Lock()
		progressed := false
		for _, step := range plan.Steps {
			if started[step.ID] {
				continue
			}
			ready, bad := true, false
			for _, dep := range step.DependsOn {
				r, ok := results[dep]
				if !ok {
					ready = false
					break
				}
				if r.Status != StatusSuccess {
					bad = true
				}
			}
			if !ready {
				continue
			}

			started[step.ID] = true
			progressed = true

			if bad || abort {
				reason := "an upstream dependency did not succeed"
				if abort {
					reason = "run aborted after an earlier failure"
				}
				results[step.ID] = Result{ID: step.ID, Status: StatusSkipped, SkipReason: reason}
				remaining--
				continue
			}

			inflight++
			s := step
			deps := make(map[string]Result, len(s.DependsOn))
			for _, dep := range s.DependsOn {
				deps[dep] = results[dep]
			}
			go func() {
				if sem != nil {
					sem <- struct{}{}
					defer func() { <-sem }()
				}
				res := execStep(ctx, s, deps, opts.DefaultTimeout)
				mu.Lock()
				results[s.ID] = res
				mu.Unlock()
				done <- s.ID
			}()
		}
		running := inflight
		mu.Unlock()

		if running == 0 {
			if !progressed {
				// No runnable steps and nothing in flight: only possible if the
				// plan is valid but something is wrong; avoid a hang.
				break
			}
			continue
		}

		id := <-done
		mu.Lock()
		inflight--
		remaining--
		if opts.StopOnError && results[id].Status == StatusFailed {
			abort = true
		}
		mu.Unlock()
	}

	return results, nil
}

// execStep runs a single step with an optional timeout and captures its outcome.
func execStep(ctx context.Context, step Step, deps map[string]Result, defaultTimeout time.Duration) Result {
	res := Result{ID: step.ID, StartedAt: time.Now()}

	stepCtx := ctx
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	out, err := step.Run(stepCtx, deps)
	res.FinishedAt = time.Now()
	if err != nil {
		res.Status = StatusFailed
		res.Err = err
		return res
	}
	res.Status = StatusSuccess
	res.Output = out
	return res
}

// Validate checks the plan for structural errors: empty or duplicate IDs,
// dependencies on unknown steps, and cycles.
func Validate(plan Plan) error {
	ids := make(map[string]bool, len(plan.Steps))
	for _, s := range plan.Steps {
		if s.ID == "" {
			return errors.New("runner: step with empty ID")
		}
		if ids[s.ID] {
			return fmt.Errorf("runner: duplicate step ID %q", s.ID)
		}
		ids[s.ID] = true
	}
	for _, s := range plan.Steps {
		for _, dep := range s.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("runner: step %q depends on unknown step %q", s.ID, dep)
			}
			if dep == s.ID {
				return fmt.Errorf("runner: step %q depends on itself", s.ID)
			}
		}
	}
	return detectCycle(plan)
}

// detectCycle performs a DFS with a recursion stack to find back-edges.
func detectCycle(plan Plan) error {
	deps := make(map[string][]string, len(plan.Steps))
	for _, s := range plan.Steps {
		deps[s.ID] = s.DependsOn
	}

	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make(map[string]int, len(plan.Steps))

	var visit func(id string) error
	visit = func(id string) error {
		color[id] = gray
		for _, dep := range deps[id] {
			switch color[dep] {
			case gray:
				return fmt.Errorf("runner: dependency cycle involving %q and %q", id, dep)
			case white:
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[id] = black
		return nil
	}

	for _, s := range plan.Steps {
		if color[s.ID] == white {
			if err := visit(s.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// HasFailures reports whether any step in the results failed.
func HasFailures(results map[string]Result) bool {
	for _, r := range results {
		if r.Status == StatusFailed {
			return true
		}
	}
	return false
}
