# Testing & Verification Guide

How the reporting → forecasting / NL-reporting / runbooks work is verified, at
each layer, plus the manual desktop pass for things that need the GUI runtime.

## Layers & how to run them

### 1. Go unit/engine tests (no GUI required)

The platform primitives live in Wails-free `internal/*` packages and are fully
unit-tested. They run anywhere Go runs:

```bash
go test ./internal/...            # all engine packages
go test -race ./internal/runner/ ./internal/runbook/ ./internal/agent/
```

Key packages and what they cover:

| Package | Covers |
|---|---|
| `internal/forecast` | exponential smoothing, anomaly detection, rows→series, accuracy metrics |
| `internal/params` | typed parameters, SQL-safe binding (injection payloads neutralized) |
| `internal/reportbind` | report filter → params binding |
| `internal/narrative` | aggregate summarization + the no-raw-rows privacy contract |
| `internal/insight` | forecast + anomalies + narrative composition |
| `internal/runner` | DAG execution: ordering, parallelism, timeouts, failure propagation |
| `internal/runbook` | typed inputs, ordered steps, write actions + dry-run/approval guardrail |
| `internal/agent` | ReAct engine, forecast tool, multi-turn sessions, orchestrator routing |

The concurrency-sensitive packages (`runner`, `runbook`, `agent`) are expected
to pass under `-race`.

### 2. Frontend tests & build (no GUI required)

```bash
cd frontend
npm ci
npm run typecheck       # tsc --noEmit
npm run lint            # eslint
npm run test:run        # vitest (component tests, happy-dom)
npm run build           # vite production bundle
```

Component tests for new UI live under `src/components/__tests__/`, e.g.
`insight-brief-panel.test.tsx` (loading/error/full-brief/empty states).

> Note: the Wails bindings under `frontend/bindings/` are regenerated only by a
> desktop build. New backend methods (e.g. `GenerateInsightBrief`) are accessed
> through a typed indirection in `src/lib/insight-api.ts` so `tsc` passes before
> regeneration; the call resolves at runtime once the app is built.

### 3. CI (cross-platform compile + tests)

CI builds `package main` with the full Wails toolchain on linux/windows/darwin
(the "Build Verification" jobs) and runs lint + unit + integration tests. A
green PR means the GUI-bound Go code (Wails bindings, agent stream handlers)
compiles on every platform — which the headless dev container cannot do.

## What needs a desktop (manual e2e)

The only thing not coverable by the above is a live GUI click-through, because
it needs gtk4/webkit and the `wails` CLI. Run these on a desktop:

### Setup
```bash
make dev      # or: wails dev   — regenerates frontend/bindings and launches the app
```
Configure an AI provider under **Settings → AI** (any of Anthropic / OpenAI /
Ollama / …). Connect a database with at least one time-series-ish table.

### Auto Insight Brief
1. Run a query that returns a date/time column and a numeric column.
2. Open the **Insights** tab in the results panel → **Generate Brief**.
3. Expect: a narrative paragraph, a forecast chart with a confidence band, and
   any anomaly callouts. Confirm the brief mentions real aggregates (means,
   totals) and does **not** echo raw rows.

### Agent forecasting
1. In the AI query agent, ask something like *"forecast next month's revenue
   from the orders table."*
2. Expect a **Forecast** message in the stream containing predictions +
   anomalies, plus the SQL the agent ran.

### Runbook (when host wiring lands)
1. Open a runbook, fill its typed parameter form.
2. Run with **Dry run** first → confirm writes/notifications are *planned*, not
   executed.
3. Run for real → approve the write at the prompt → confirm rows affected.

## Conventions

- New engine logic goes in a Wails-free `internal/*` package with tests, so it
  is verifiable without the GUI. The `services`/`package main` layer stays a
  thin adapter that CI compiles.
- Anything touching SQL string-building must go through `internal/params`
  (SQL-safe) — never hand-concatenate user input.
