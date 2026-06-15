// Command loadtest is a reusable stress harness for HowlerOps' query engine.
//
// It drives the real pkg/database engine against an in-process SQLite database
// loaded with a large synthetic dataset, runs a suite of workloads designed to
// surface breakpoints (latency cliffs, memory blow-ups, concurrency failures,
// and the cost of the pagination COUNT(*)), and writes a Markdown report.
//
// The generated dataset lives in a temp directory that is removed on exit, so
// the load "destroys itself when done".
//
// Usage:
//
//	go run ./cmd/loadtest --scale medium --out docs/performance/LOAD_TEST_REPORT.md
//
// Scales: small (~10k rows), medium (~100k), large (~500k).
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/jbeck018/howlerops/pkg/database"
)

// scaleProfiles maps a scale name to the number of rows generated.
var scaleProfiles = map[string]int{
	"small":  10_000,
	"medium": 100_000,
	"large":  500_000,
}

func main() {
	scale := flag.String("scale", "medium", "dataset scale: small|medium|large")
	out := flag.String("out", "docs/performance/LOAD_TEST_REPORT.md", "report output path")
	concurrency := flag.Int("concurrency", 16, "concurrent workers for the contention scenario")
	flag.Parse()

	rows, ok := scaleProfiles[*scale]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown scale %q (use small|medium|large)\n", *scale)
		os.Exit(2)
	}

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // keep harness output clean; we want metrics, not query logs

	h := &harness{logger: logger, rows: rows, concurrency: *concurrency, scale: *scale}
	if err := h.run(*out); err != nil {
		fmt.Fprintf(os.Stderr, "load test failed: %v\n", err)
		os.Exit(1)
	}
}

type harness struct {
	logger      *logrus.Logger
	db          *database.SQLiteDatabase
	rows        int
	concurrency int
	scale       string
	results     []scenarioResult
}

type scenarioResult struct {
	name       string
	iterations int
	errors     int
	p50        time.Duration
	p95        time.Duration
	max        time.Duration
	rows       int64
	heapDelta  int64 // bytes
	notes      string
	breakpoint string // non-empty => flagged
}

func (h *harness) run(reportPath string) error {
	ctx := context.Background()

	tmpDir, err := os.MkdirTemp("", "howler-loadtest-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	// Destroy the generated load when done.
	defer func() { _ = os.RemoveAll(tmpDir) }()

	dbPath := filepath.Join(tmpDir, "loadtest.db")
	config := database.ConnectionConfig{
		Type:              database.SQLite,
		Database:          dbPath,
		ConnectionTimeout: 30 * time.Second,
		IdleTimeout:       5 * time.Minute,
		MaxConnections:    25,
		MaxIdleConns:      5,
		Parameters: map[string]string{
			// Mirror the app's local-storage config: private cache + WAL so
			// concurrent readers run in parallel (see pkg/storage/sqlite_local.go).
			"mode":          "rwc",
			"_journal_mode": "WAL",
			"_busy_timeout": "5000",
		},
	}

	db, err := database.NewSQLiteDatabase(config, h.logger)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	h.db = db
	defer func() { _ = db.Disconnect() }()

	fmt.Printf("⏳ Generating %s dataset (%d rows) in %s ...\n", h.scale, h.rows, dbPath)
	genStart := time.Now()
	if err := h.generate(ctx); err != nil {
		return fmt.Errorf("generate data: %w", err)
	}
	fmt.Printf("✅ Data generated in %s\n\n", time.Since(genStart).Round(time.Millisecond))

	// Run the scenarios.
	h.scenarioBaseline(ctx)
	h.scenarioFullScanWithCount(ctx)
	h.scenarioPaginationSweep(ctx, false)
	h.scenarioPaginationSweep(ctx, true)
	h.scenarioWideNormalization(ctx)
	h.scenarioLargeResultMemory(ctx)
	h.scenarioConcurrency(ctx)

	report := h.renderReport()
	fmt.Println(report)

	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	fmt.Printf("\n📄 Report written to %s\n", reportPath)
	return nil
}

// generate creates the schema and bulk-loads synthetic rows.
func (h *harness) generate(ctx context.Context) error {
	ddl := `CREATE TABLE events (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		category TEXT NOT NULL,
		amount REAL NOT NULL,
		note TEXT,
		payload TEXT,
		is_active INTEGER NOT NULL,
		created_at TEXT NOT NULL
	)`
	if _, err := h.db.Execute(ctx, ddl); err != nil {
		return err
	}

	categories := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	rng := rand.New(rand.NewSource(42)) // deterministic dataset
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	const batch = 500
	cols := "(user_id, category, amount, note, payload, is_active, created_at)"
	placeholder := "(?, ?, ?, ?, ?, ?, ?)"

	for start := 0; start < h.rows; start += batch {
		n := batch
		if start+n > h.rows {
			n = h.rows - start
		}
		var sb strings.Builder
		sb.WriteString("INSERT INTO events " + cols + " VALUES ")
		args := make([]interface{}, 0, n*7)
		for i := 0; i < n; i++ {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(placeholder)
			id := start + i
			note := fmt.Sprintf("event %d for processing batch with descriptive text", id)
			// Every 10th row carries a JSON payload to exercise JSON normalization.
			var payload string
			if id%10 == 0 {
				payload = fmt.Sprintf(`{"id":%d,"tags":["a","b","c"],"meta":{"score":%d}}`, id, rng.Intn(100))
			} else {
				payload = "plain text payload that is not json " + note
			}
			args = append(args,
				rng.Intn(10_000),
				categories[rng.Intn(len(categories))],
				rng.Float64()*1000,
				note,
				payload,
				rng.Intn(2),
				base.Add(time.Duration(id)*time.Minute).Format(time.RFC3339),
			)
		}
		if _, err := h.db.Execute(ctx, sb.String(), args...); err != nil {
			return fmt.Errorf("insert batch at %d: %w", start, err)
		}
	}
	return nil
}

// ---- scenarios ----

func (h *harness) scenarioBaseline(ctx context.Context) {
	h.measure("baseline: indexed point lookups (LIMIT 50)", 200, func() (int64, error) {
		opts := &database.QueryOptions{Timeout: 30 * time.Second, ReadOnly: true, Limit: 50}
		res, err := h.db.ExecuteWithOptions(ctx, "SELECT * FROM events WHERE user_id = 1234", opts)
		if err != nil {
			return 0, err
		}
		return res.RowCount, nil
	}, "")
}

func (h *harness) scenarioFullScanWithCount(ctx context.Context) {
	h.measure("full scan, page 1 with COUNT(*) (LIMIT 1000)", 30, func() (int64, error) {
		opts := &database.QueryOptions{Timeout: 60 * time.Second, ReadOnly: true, Limit: 1000}
		res, err := h.db.ExecuteWithOptions(ctx, "SELECT * FROM events ORDER BY created_at", opts)
		if err != nil {
			return 0, err
		}
		return res.RowCount, nil
	}, "Page 1 must compute the total; this is the COUNT(*) cost we keep on the first page.")
}

// scenarioPaginationSweep pages deep into the result set with and without
// SkipCount to quantify the optimization landed in this branch.
func (h *harness) scenarioPaginationSweep(ctx context.Context, skipCount bool) {
	name := "pagination sweep (20 pages) — COUNT each page"
	note := "Old behaviour: every page recomputes the full COUNT(*)."
	if skipCount {
		name = "pagination sweep (20 pages) — SkipCount (current behaviour)"
		note = "Current behaviour: pages 2+ skip the COUNT(*)."
	}
	h.measure(name, 20, func() (int64, error) {
		// Each iteration is one page; offset advances per iteration handled below.
		return 0, nil
	}, note)

	// Re-run with real offsets (measure helper can't vary per-iteration), so do it inline.
	idx := len(h.results) - 1
	var lats []time.Duration
	var errs int
	var rowsSeen int64
	for page := 0; page < 20; page++ {
		opts := &database.QueryOptions{
			Timeout:   60 * time.Second,
			ReadOnly:  true,
			Limit:     1000,
			Offset:    page * 1000,
			SkipCount: skipCount && page > 0,
		}
		start := time.Now()
		res, err := h.db.ExecuteWithOptions(ctx, "SELECT * FROM events ORDER BY id", opts)
		lats = append(lats, time.Since(start))
		if err != nil {
			errs++
			continue
		}
		rowsSeen += res.RowCount
	}
	h.results[idx] = summarize(name, lats, errs, rowsSeen, 0, note)
}

func (h *harness) scenarioWideNormalization(ctx context.Context) {
	h.measure("normalization: 5000 rows incl. JSON + text payloads", 20, func() (int64, error) {
		opts := &database.QueryOptions{Timeout: 60 * time.Second, ReadOnly: true, Limit: 5000}
		res, err := h.db.ExecuteWithOptions(ctx, "SELECT id, note, payload, created_at FROM events", opts)
		if err != nil {
			return 0, err
		}
		return res.RowCount, nil
	}, "Exercises per-cell NormalizeValue, including the JSON-skip fast path.")
}

func (h *harness) scenarioLargeResultMemory(ctx context.Context) {
	limit := h.rows
	if limit > 200_000 {
		limit = 200_000
	}
	name := fmt.Sprintf("large result into memory (LIMIT %d)", limit)
	h.measure(name, 3, func() (int64, error) {
		opts := &database.QueryOptions{Timeout: 120 * time.Second, ReadOnly: true, Limit: limit, SkipCount: true}
		res, err := h.db.ExecuteWithOptions(ctx, "SELECT * FROM events", opts)
		if err != nil {
			return 0, err
		}
		return res.RowCount, nil
	}, "Materializes a large result set; watch heap growth — there is no server-side streaming on this path.")
}

func (h *harness) scenarioConcurrency(ctx context.Context) {
	name := fmt.Sprintf("concurrency: %d workers × 25 queries", h.concurrency)
	var wg sync.WaitGroup
	var errs int64
	var rowsSeen int64
	latCh := make(chan time.Duration, h.concurrency*25)

	heapBefore := heapAlloc()
	start := time.Now()
	for w := 0; w < h.concurrency; w++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(seed)))
			for i := 0; i < 25; i++ {
				uid := rng.Intn(10_000)
				opts := &database.QueryOptions{Timeout: 60 * time.Second, ReadOnly: true, Limit: 500}
				qStart := time.Now()
				res, err := h.db.ExecuteWithOptions(ctx,
					fmt.Sprintf("SELECT * FROM events WHERE user_id = %d ORDER BY id", uid), opts)
				latCh <- time.Since(qStart)
				if err != nil {
					atomic.AddInt64(&errs, 1)
					continue
				}
				atomic.AddInt64(&rowsSeen, res.RowCount)
			}
		}(w)
	}
	wg.Wait()
	close(latCh)
	wall := time.Since(start)
	heapDelta := int64(heapAlloc()) - int64(heapBefore)

	lats := make([]time.Duration, 0, len(latCh))
	for d := range latCh {
		lats = append(lats, d)
	}
	res := summarize(name, lats, int(errs), rowsSeen, heapDelta, "")
	qps := float64(len(lats)) / wall.Seconds()
	res.notes = fmt.Sprintf("Wall %s, throughput ≈ %.0f queries/sec.", wall.Round(time.Millisecond), qps)
	if errs > 0 {
		res.breakpoint = fmt.Sprintf("%d concurrent queries returned errors — possible pool exhaustion or lock contention.", errs)
	}
	h.results = append(h.results, res)
}

// ---- measurement helpers ----

func (h *harness) measure(name string, iterations int, fn func() (int64, error), notes string) {
	lats := make([]time.Duration, 0, iterations)
	var errs int
	var rowsSeen int64
	heapBefore := heapAlloc()
	for i := 0; i < iterations; i++ {
		start := time.Now()
		n, err := fn()
		lats = append(lats, time.Since(start))
		if err != nil {
			errs++
			continue
		}
		rowsSeen += n
	}
	heapDelta := int64(heapAlloc()) - int64(heapBefore)
	h.results = append(h.results, summarize(name, lats, errs, rowsSeen, heapDelta, notes))
}

func summarize(name string, lats []time.Duration, errs int, rowsSeen, heapDelta int64, notes string) scenarioResult {
	r := scenarioResult{
		name:       name,
		iterations: len(lats),
		errors:     errs,
		rows:       rowsSeen,
		heapDelta:  heapDelta,
		notes:      notes,
	}
	if len(lats) > 0 {
		sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
		r.p50 = lats[len(lats)*50/100]
		r.p95 = lats[min(len(lats)-1, len(lats)*95/100)]
		r.max = lats[len(lats)-1]
	}

	// Breakpoint heuristics.
	switch {
	case errs > 0:
		r.breakpoint = fmt.Sprintf("%d/%d iterations errored.", errs, len(lats))
	case r.p95 > 2*time.Second:
		r.breakpoint = fmt.Sprintf("p95 latency %s exceeds 2s budget.", r.p95.Round(time.Millisecond))
	case heapDelta > 256<<20:
		r.breakpoint = fmt.Sprintf("heap grew %s for a single scenario.", humanBytes(heapDelta))
	}
	return r
}

func heapAlloc() uint64 {
	// Force a GC so the sample reflects live (retained) heap rather than
	// not-yet-collected garbage, which otherwise produces noisy/negative deltas.
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

func humanBytes(b int64) string {
	if b < 0 {
		return "~0 B" // GC reclaimed more than the scenario retained
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGT"[exp])
}

func (h *harness) renderReport() string {
	var sb strings.Builder
	sb.WriteString("# Query Engine Load Test Report\n\n")
	sb.WriteString(fmt.Sprintf("- Generated: %s\n", time.Now().UTC().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- Engine: in-process SQLite via `pkg/database`\n"))
	sb.WriteString(fmt.Sprintf("- Scale: **%s** (%d rows), concurrency: %d\n", h.scale, h.rows, h.concurrency))
	sb.WriteString(fmt.Sprintf("- Go: %s on %s/%s\n\n", runtime.Version(), runtime.GOOS, runtime.GOARCH))

	sb.WriteString("| Scenario | Iters | p50 | p95 | max | Rows | Heap Δ | Errors |\n")
	sb.WriteString("|---|---:|---:|---:|---:|---:|---:|---:|\n")
	for _, r := range h.results {
		sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s | %s | %d | %s | %d |\n",
			r.name, r.iterations,
			r.p50.Round(time.Microsecond), r.p95.Round(time.Microsecond), r.max.Round(time.Microsecond),
			r.rows, humanBytes(r.heapDelta), r.errors))
	}

	sb.WriteString("\n## Breakpoints\n\n")
	found := false
	for _, r := range h.results {
		if r.breakpoint != "" {
			found = true
			sb.WriteString(fmt.Sprintf("- ⚠️ **%s** — %s\n", r.name, r.breakpoint))
		}
	}
	if !found {
		sb.WriteString("- No breakpoints triggered at this scale. Re-run with `--scale large` to push harder.\n")
	}

	sb.WriteString("\n## Notes\n\n")
	for _, r := range h.results {
		if r.notes != "" {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", r.name, r.notes))
		}
	}
	return sb.String()
}
