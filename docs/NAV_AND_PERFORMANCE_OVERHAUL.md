# Performance + Navigation Overhaul — Work Summary

This document summarizes the full body of work across the performance branch
(`claude/report-performance-optimization-71ftih`, PR #6) and the navigation
follow-up branch (`claude/nav-vertical-tabs-71ftih`).

---

## 1. Query & report performance

- **Removed a redundant `COUNT(*)` round-trip in reports** (`services/report.go`).
  The database layer already returns `TotalRows` when a `Limit` is set, so the
  separate count query was an unbounded re-scan on every cache miss. Now the
  single query's `TotalRows` is used.
- **Two-phase report execution.** Query components run in a worker pool; LLM
  components then run **sequentially in order** against an accumulating snapshot
  of prior results. This removed an O(n²) per-task index rebuild and an
  LLM-context race, and makes LLM→query *and* LLM→LLM context deterministic.
- **`QueryOptions.SkipCount` — skip the count on pagination.** Paging re-executes
  per page, so each page change used to re-run the full `COUNT(*)`. The editor
  query path now sets `SkipCount` when `Offset > 0`; the count is skipped on
  pages 2+, and the frontend carries the page-1 total forward. Guarded across all
  six engines (postgres, mysql, sqlite, clickhouse, elasticsearch, mongodb).
  `HasMore` is inferred from page fullness when the total is unknown.
  Regression test: `pkg/database/pagination_test.go`.
- **Per-cell normalization** (`pkg/database/normalizer.go`). Rows are normalized
  in place (one fewer slice allocation per row), and `NormalizeValue` only
  attempts `json.Unmarshal` on `[]byte` whose first non-whitespace byte can begin
  JSON — skipping a wasted parse on plain text. Verified behavior-identical.

## 2. Local SQLite concurrency

- App-owned SQLite databases (`pkg/storage/sqlite_local.go`, the RAG vector
  store) dropped `cache=shared` (private page cache) and added `_busy_timeout`.
  Shared-cache mode serializes readers even under WAL; the load test showed
  concurrency collapsing to ~8 q/s (p95 5.7s), and the fix restores ~37 q/s
  (p95 ~0.68s). Pure `:memory:` stores keep shared cache (each pooled connection
  would otherwise get its own empty DB).

## 3. Connections — pgAdmin-style optional database

- The Database field is now optional. A blank database connects via a per-engine
  **maintenance database** (Postgres→`postgres`, ClickHouse→`default`; MySQL/TiDB
  connect with no default schema), and the working database is chosen after
  connecting. The actually-connected database is resolved and surfaced back to
  the UI. Frontend form + validation updated accordingly.

## 4. Sidebar refactor

- The 725-line sidebar monolith was split into focused, memoized components
  (`schema-tree`, `sidebar-nav`, a memoized `ConnectionRow`) so interacting with
  one connection row no longer re-renders the whole list. Active-tab and
  filtered-connection computations are memoized once per render (was O(n²)).

## 5. Load & stress suite

- `cmd/loadtest`: a reusable harness that drives the real `pkg/database` engine
  (SQLite in-process; Postgres/MySQL via the docker kit) with a large synthetic
  dataset, runs breakpoint-hunting scenarios, writes a Markdown report, and
  destroys the generated data on exit. See `docs/performance/`.

## 6. Typography

- Introduced **Inter** for UI chrome (`--font-sans`) while keeping **Fira Code**
  (`--font-mono`) for the SQL editor and result grids; base font dropped
  `14.4px → 13px` for a denser UI.

## 7. Navigation redesign (the main UX change)

Replaced the stacked-horizontal layout (logo bar → mode-switch → horizontal
Query/AI tab strip) with a **VS Code-style three-column model**:

```
[ IconRail 48px ] [ ContextPanel ~224px ] [ editor/content ]
```

- **IconRail** (`icon-rail.tsx`): 48px icon-only primary nav (from `NAV_ITEMS`),
  hover tooltips, click-to-navigate, active accent bar.
- **ContextPanel** (`context-panel.tsx`): route-aware sub-nav. Queries shows the
  **vertical Open Tabs list** above the Active Connections panel; other routes
  get lean labelled sub-navs.
- **Vertical Open Tabs** (`subnav/open-tabs-list.tsx`): replaces the horizontal
  strip. Each row has a type glyph (Terminal=SQL, ✦=AI), title, dirty dot, a
  per-tab connection picker (popover), and close-on-hover. Switch/close/create
  (SQL **and** AI) all work.
- **Reactive tab content.** Switching the active tab from the side panel works
  because CodeMirror is reactive to its `value` prop and `use-editor-state`
  mirrors the active tab's content — no imperative sync needed.
- **Shared tab actions** (`hooks/use-tab-actions.ts`): tab create (SQL/AI, incl.
  AI agent session) and per-tab connection change were lifted out of the editor
  into a store-backed hook so the ContextPanel (a sibling of the editor) can
  drive them. This unblocked removing the horizontal strip.
- **Horizontal `QueryTabs` strip removed** from the editor; `HeaderBar`
  (mode / AI selector / diagnostics) is retained.

## 8. Fan-out PR review (11 agents)

A parallel review of PR #6 surfaced and fixed: a **blocker** (`HasMore` collapsed
to false on skipped-count pages → couldn't page past page 2), and majors
(`:memory:` vector-store cache regression, lost DB-list refresh on re-select,
lost LLM→LLM context, a double panel border). A pagination regression test was
added. Security, normalizer, and the connections Go flow reviewed clean.

---

## Known follow-ups (need visual iteration)

- **Multi-DB per-tab selection**: the vertical row's connection picker is
  single-select; the old strip's multi-DB checkbox flow isn't ported yet.
- **HeaderBar slimming / status strip**: `HeaderBar` is retained as-is; the
  connected-count could move to a bottom status strip.
- **Per-route sub-navs** (Reports/Catalog/etc.) are lean scaffolds to flesh out.
- **`Factory.ValidateConfig`** is dead/test-only; consider wiring or removing.
- **Backend `TotalRows=0` on skipped pages** lies to non-web API consumers (the
  web client carries the total forward); echo or omit for other consumers.
