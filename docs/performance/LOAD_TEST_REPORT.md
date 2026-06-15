# Query Engine Load Test Report

- Generated: 2026-06-15T16:50:04Z
- Engine: **sqlite** (`pkg/database`) — target `/tmp/howler-loadtest-592076/loadtest.db`
- Scale: **large** (500000 rows), concurrency: 16
- Go: go1.26.0 on linux/amd64

| Scenario | Iters | p50 | p95 | max | Rows | Heap Δ | Errors |
|---|---:|---:|---:|---:|---:|---:|---:|
| baseline: point lookups (LIMIT 50) | 200 | 74.742ms | 82.953ms | 102.224ms | 10000 | 110.3 KB | 0 |
| full scan, page 1 with COUNT(*) (LIMIT 1000) | 30 | 222.622ms | 322.879ms | 396.731ms | 30000 | 37.6 KB | 0 |
| pagination sweep (20 pages) — COUNT each page | 20 | 47.636ms | 52.984ms | 52.984ms | 20000 | 0 B | 0 |
| pagination sweep (20 pages) — SkipCount (current behaviour) | 20 | 2.468ms | 45.91ms | 45.91ms | 20000 | 0 B | 0 |
| normalization: 5000 rows incl. JSON + text payloads | 20 | 31.146ms | 36.077ms | 36.077ms | 100000 | 336 B | 0 |
| large result into memory (LIMIT 200000) | 3 | 714.365ms | 1.026165s | 1.026165s | 600000 | 36.0 KB | 0 |
| concurrency: 16 workers × 25 queries | 400 | 412.923ms | 678.42ms | 982.818ms | 20001 | 246.2 KB | 0 |

## Breakpoints

- No breakpoints triggered at this scale. Re-run with `--scale large` to push harder.

## Notes

- **full scan, page 1 with COUNT(*) (LIMIT 1000)**: Page 1 must compute the total; this is the COUNT(*) cost we keep on the first page.
- **pagination sweep (20 pages) — COUNT each page**: Old behaviour: every page recomputes the full COUNT(*).
- **pagination sweep (20 pages) — SkipCount (current behaviour)**: Current behaviour: pages 2+ skip the COUNT(*).
- **normalization: 5000 rows incl. JSON + text payloads**: Exercises per-cell NormalizeValue, including the JSON-skip fast path.
- **large result into memory (LIMIT 200000)**: Materializes a large result set; watch heap growth — there is no server-side streaming on this path.
- **concurrency: 16 workers × 25 queries**: Wall 10.715s, throughput ≈ 37 queries/sec.
