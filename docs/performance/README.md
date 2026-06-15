# Query Engine Load & Stress Suite

A reusable harness that stresses the real `pkg/database` query engine to find
breakpoints — latency cliffs, memory blow-ups, the cost of the pagination
`COUNT(*)`, and concurrency collapse.

It drives the **actual engine code path** (`ExecuteWithOptions`, normalization,
pagination) against an in-process SQLite database loaded with a large synthetic
dataset. The dataset is generated into a temp directory and **deleted on exit**,
so a run leaves nothing behind except the report.

> Scope note: this exercises the SQLite engine in-process (runnable in CI / a
> dev box with no GUI or external DB). The Postgres/MySQL engines share the same
> `executeSelect` structure (count → fetch → normalize), so the relative
> findings — COUNT cost, normalization cost, the SkipCount win — generalize;
> absolute numbers will differ per driver/server.

## Running

```bash
# small (~10k rows), medium (~100k), large (~500k)
go run ./cmd/loadtest --scale large --out docs/performance/LOAD_TEST_REPORT.md

# tune concurrent workers for the contention scenario
go run ./cmd/loadtest --scale medium --concurrency 32
```

The latest run is captured in [`LOAD_TEST_REPORT.md`](./LOAD_TEST_REPORT.md).

## Scenarios

| Scenario | What it stresses |
|---|---|
| baseline point lookups | per-query floor latency |
| full scan + COUNT(*) (page 1) | the unavoidable page-1 count cost |
| pagination sweep — COUNT each page | the *old* behaviour (count every page) |
| pagination sweep — SkipCount | the current behaviour (skip count on pages 2+) |
| normalization (JSON + text) | per-cell `NormalizeValue`, incl. the JSON-skip fast path |
| large result into memory | materializing big result sets (no streaming) |
| concurrency (N workers) | pool + lock contention, throughput under load |

A scenario is flagged as a **breakpoint** when it errors, when p95 latency
exceeds 2s, or when a single scenario retains >256 MB of heap.

## Findings (large scale: 500k rows, 16 workers)

### 🔴 Breakpoint — concurrency collapse
Throughput fell from ~188 q/s (100k rows) to **~8 q/s** (500k rows), with p95
**5.7s** and max **8s**. Root causes, in order of impact:

1. **Unindexed filters become full scans.** The workload filters on `user_id`
   with no index, so every query scans all 500k rows. Under 16 concurrent
   workers this dominates.
2. **SQLite `cache=shared` serializes readers.** Shared-cache mode takes a
   table-level read lock, so concurrent readers contend instead of running in
   parallel.

**Recommendations for the app's local SQLite usage:** enable WAL journal mode
(`_journal_mode=WAL`) and prefer a private cache for read concurrency; ensure
indexes exist on columns the app filters/sorts on in its own storage tables.
(For user-connected databases this is the user's schema, but the same lesson —
unindexed filters don't scale under concurrency — is worth surfacing in the UI,
e.g. an EXPLAIN/"add index" hint.)

### 🟠 Page-1 `COUNT(*)` roughly doubles first-page latency
The full-scan page-1 query costs **242ms p50**, of which the wrapping
`SELECT COUNT(*) FROM (<query>)` is a large share (it re-scans the whole result
set). This is the cost we intentionally keep only on page 1.

**Recommendation:** consider computing the page-1 total asynchronously (return
rows immediately, stream the total in after) so first paint isn't blocked on the
count.

### 🟢 Pagination SkipCount optimization — validated
Paging with the current `SkipCount` behaviour is **~14× faster per page**
(p50 **3.6ms** vs **51ms** when counting every page). This is the optimization
landed on this branch.

### 🟠 Large result sets are fully materialized (no streaming)
200k rows load in ~640ms and are held entirely in memory. There is no
server-side streaming on the `ExecuteWithOptions` path; at higher scales this
becomes a memory breakpoint.

**Recommendation:** route large/export reads through the existing
`ExecuteQueryStream` path rather than buffering the whole result set.

### 🟢 Normalization fast path — healthy
Normalizing 100k cells (incl. JSON payloads) stays ~30ms with negligible
retained heap, confirming the in-place normalization + JSON-skip changes hold up.
