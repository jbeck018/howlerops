# Query Engine Load & Stress Suite

A reusable harness that stresses the real `pkg/database` query engine to find
breakpoints — latency cliffs, memory blow-ups, the cost of the pagination
`COUNT(*)`, and concurrency collapse.

It drives the **actual engine code path** (`ExecuteWithOptions`, normalization,
pagination) against a database loaded with a large synthetic dataset. The load
is **destroyed on exit** (temp SQLite dir removed / `events` table dropped), so a
run leaves nothing behind except the report.

## Running

### SQLite (in-process, no external service)

```bash
# small (~10k rows), medium (~100k), large (~500k)
go run ./cmd/loadtest --scale large --out docs/performance/LOAD_TEST_REPORT.md
go run ./cmd/loadtest --scale medium --concurrency 32
```

### Postgres / MySQL (dockerized kit)

```bash
docker compose -f docs/performance/docker-compose.loadtest.yml up -d
go run ./cmd/loadtest --engine postgres --scale large
go run ./cmd/loadtest --engine mysql    --scale large
docker compose -f docs/performance/docker-compose.loadtest.yml down -v
```

Credentials default to `loadtest`/`loadtest`/`loadtest` to match the compose
file; override with `--host/--port/--user/--password/--db/--sslmode`.

The latest committed run is [`LOAD_TEST_REPORT.md`](./LOAD_TEST_REPORT.md)
(SQLite, large scale).

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

## Findings & actions (SQLite, 500k rows, 16 workers)

### 🔴→🟢 Concurrency collapse — FIXED
The first run collapsed under load: throughput fell from ~188 q/s (100k rows) to
**~8 q/s** (500k), with p95 **5.7s**. Root cause: SQLite `cache=shared`
serializes readers behind a table-level lock even under WAL, compounded by
unindexed full scans.

**Fix (this branch):** dropped `cache=shared` (private page cache) on the
app-owned SQLite databases (`pkg/storage/sqlite_local.go`, the RAG vector store)
and added `_busy_timeout`. Re-running shows concurrency **p95 ~0.68s** and
**~37 q/s** — the breakpoint clears. Trade-off: isolated single-query latency
rises slightly (less cross-connection cache sharing), which is the right call for
a multi-tab desktop client.

### 🟢 Pagination SkipCount — validated
Paging with the current `SkipCount` behaviour is **~10–14× faster per page**
(p50 **2.5ms** vs **48ms** when counting every page). This is the optimization
landed earlier on this branch.

### 🟠 Page-1 `COUNT(*)` — deferred (async total)
The full-scan page-1 query costs ~**220ms p50**, much of it the wrapping
`SELECT COUNT(*)`. A naive fix — running the count concurrently with the fetch —
was implemented and measured, but it **doubles concurrent full scans under load
and regressed the concurrency throughput we just fixed**, so it was reverted.
The correct fix is a true *async total*: return rows immediately and push the
count to the UI via an event once it lands. That requires frontend plumbing and
is tracked as a follow-up.

### 🟠 Large result sets are fully materialized — follow-up (streaming)
200k rows load in ~0.7–1.5s and are held entirely in memory; there is no
server-side streaming on `ExecuteWithOptions`. At higher scales this is a memory
breakpoint. The engine already exposes `ExecuteQueryStream`/`ExecuteStream`;
the follow-up is to route large/export reads (and optionally results above a row
threshold) through it and render incrementally on the frontend.

### 🟢 Normalization fast path — healthy
Normalizing 100k cells (incl. JSON payloads) stays ~30ms with negligible
retained heap, confirming the in-place normalization + JSON-skip changes hold up.

## Other issues noted along the way (follow-ups)

- **Unindexed filters don't scale.** Every `WHERE`/`ORDER BY` on a non-indexed
  column is a full scan that collapses under concurrency. For the app's *own*
  storage tables, ensure indexes exist; for user-connected databases, consider
  surfacing an EXPLAIN / "add index" hint in the UI.
- **Other SQLite open sites.** The team/sync DB in `pkg/storage/manager.go`
  opens via a raw URL without the WAL/private-cache treatment applied here —
  worth aligning if it sees concurrent access.
- **Dead code in `pkg/database/normalizer.go`.** `ApplyPagination` and
  `ExtractTotalCount` appear unused (the engines build their own LIMIT/count);
  candidates for removal.
- **`Factory.ValidateConfig` is unreferenced** in the connect path — validation
  is effectively only the DSN build. Either wire it in or drop it.

> Scope note: the in-process run exercises SQLite. The Postgres/MySQL engines
> share the same `executeSelect` shape (count → fetch → normalize), so the
> *relative* findings (COUNT cost, SkipCount win, normalization cost) generalize;
> use the docker kit to confirm absolute numbers on real servers.
