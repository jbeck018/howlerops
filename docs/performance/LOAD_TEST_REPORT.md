# Query Engine Load Test Report

- Generated: 2026-06-15T11:21:18Z
- Engine: in-process SQLite via `pkg/database`
- Scale: **large** (500000 rows), concurrency: 16
- Go: go1.26.0 on linux/amd64

| Scenario | Iters | p50 | p95 | max | Rows | Heap Δ | Errors |
|---|---:|---:|---:|---:|---:|---:|---:|
| baseline: indexed point lookups (LIMIT 50) | 200 | 76.118ms | 80.925ms | 111.103ms | 10000 | 110.1 KB | 0 |
| full scan, page 1 with COUNT(*) (LIMIT 1000) | 30 | 241.732ms | 349.991ms | 356.731ms | 30000 | 110.2 KB | 0 |
| pagination sweep (20 pages) — COUNT each page | 20 | 50.959ms | 56.942ms | 56.942ms | 20000 | 0 B | 0 |
| pagination sweep (20 pages) — SkipCount (current behaviour) | 20 | 3.61ms | 50.329ms | 50.329ms | 20000 | 0 B | 0 |
| normalization: 5000 rows incl. JSON + text payloads | 20 | 30.178ms | 34.947ms | 34.947ms | 100000 | ~0 B | 0 |
| large result into memory (LIMIT 200000) | 3 | 640.713ms | 680.762ms | 680.762ms | 600000 | 36.2 KB | 0 |
| concurrency: 16 workers × 25 queries | 400 | 838.042ms | 5.691572s | 7.975782s | 20001 | 247.1 KB | 0 |

## Breakpoints

- ⚠️ **concurrency: 16 workers × 25 queries** — p95 latency 5.692s exceeds 2s budget.

## Notes

- **full scan, page 1 with COUNT(*) (LIMIT 1000)**: Page 1 must compute the total; this is the COUNT(*) cost we keep on the first page.
- **pagination sweep (20 pages) — COUNT each page**: Old behaviour: every page recomputes the full COUNT(*).
- **pagination sweep (20 pages) — SkipCount (current behaviour)**: Current behaviour: pages 2+ skip the COUNT(*).
- **normalization: 5000 rows incl. JSON + text payloads**: Exercises per-cell NormalizeValue, including the JSON-skip fast path.
- **large result into memory (LIMIT 200000)**: Materializes a large result set; watch heap growth — there is no server-side streaming on this path.
- **concurrency: 16 workers × 25 queries**: Wall 52.95s, throughput ≈ 8 queries/sec.
