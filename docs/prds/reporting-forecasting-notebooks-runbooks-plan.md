# Reporting → Forecasting, NL Reporting, Notebooks & Runbooks — Strategy & Build Plan

> Status: Draft for discussion · Owner: TBD · Last updated: 2026-06-20
>
> Scope: Extend HowlerOps' reporting into (1) forecasting + anomaly detection,
> (2) natural-language reporting, (3) notebooks (exploratory, cell-based), and
> (4) runbooks (reusable, re-runnable parameterized tasks).

---

## 1. Where HowlerOps starts from (our unfair advantages)

HowlerOps is **not** a cloud BI tool — and that's the wedge. Today it is a
**local-first, AI-native desktop SQL client** with capabilities most competitors
don't combine:

- **Cross-database federation** via DuckDB `ATTACH` (query across Postgres /
  MySQL / SQLite / Mongo / etc. in one statement) — shipped in v0.17.
- **AI query agent + RAG** over schema and query history (NL→SQL, suggestions,
  error-fixing), with a pluggable LLM layer (Anthropic / OpenAI / Ollama).
- **Local-first storage** (SQLite) with optional **Turso cloud sync**;
  credentials never leave the device.
- A working **report system**: `ReportService` (save / load / run report
  definitions made of *components*), a dashboard canvas, and a chart renderer.

Implication: we can offer an **AI-native data workspace that runs on your
machine, against all your databases at once, without shipping your data to a
SaaS** — a position no incumbent fully owns.

---

## 2. Competitive landscape (2025–2026)

### 2a. Notebooks (exploratory, cell-based analysis)
| Product | What they do well | Gap we can exploit |
|---|---|---|
| **Hex** | "Notebook Agent" + **Threads** conversational analytics; SQL+Python+chart cells; data apps; runs Anthropic Sonnet 4.5 as primary model | Cloud-only; warehouse-centric; not local; no true multi-DB federation in one query |
| **Deepnote** | "Auto AI" agent that writes/runs/debugs; co-authoring agent; Jupyter-compatible | Cloud; Python-runtime heavy; less of a "SQL client" |
| **Observable / Count / Jupyter** | Flexible canvases / classic notebooks | Little AI-native authoring; no DB-client ergonomics |

**Trend:** the notebook is becoming **agent-authored** — you describe intent, the
agent builds and self-corrects SQL/chart/text cells.

### 2b. Natural-language reporting / AI BI
| Product | Signature capability |
|---|---|
| **ThoughtSpot Spotter** | NL search over a semantics layer; **Insight Briefs** — exec summaries across KPIs (shared drivers, correlated trends, outliers) |
| **Sigma** | **Ask Sigma** NL interface + **Sigma Agents** (locate sources, build multi-step analyses) |
| **Power BI Copilot / Tableau Agent (Pulse)** | Conversational Q&A, report summarization, NL→viz, proactive narratives |
| **Narrative BI / NetSuite Narrative Insights** | "No dashboards" — generate **written narratives** from data |

**Trend (Gartner):** ~80% of enterprise analytics teams adopted conversational
AI by 2026; multi-agent systems split roles (data parsing → trend analysis →
narrative generation). The expectation is now: **prompt → charts + dashboards +
written narrative**.

### 2c. Forecasting & anomaly detection
| Product | Built-in capability |
|---|---|
| **Power BI** | Forecasting via **exponential smoothing** (trend + seasonality, confidence bands); **anomaly detection** via time-series decomposition (trend/seasonality/noise). Limits: line charts only, ≥4 points |
| **Tableau** | Forecasting + clustering built into viz |
| **Specialized** | Anodot etc. for streaming anomaly detection/alerting |

**Trend:** forecasting/anomaly is **table-stakes inside the chart**, not a
separate data-science project — one click, with confidence intervals + alerts.

### 2d. Runbooks / data apps / reusable actions
| Product | Reusable-task model |
|---|---|
| **Retool Workflows** | Cron-scheduled / webhook / event-driven pipelines; query DBs, call APIs, transform, notify, branch; **parameterized**, reused across apps |
| **Hex data apps** | Parameterize SQL with user inputs → publishable app |
| **Basedash** | AI concierge builds CRUD queries you **save as reusable actions** |
| **Outerbase (EZQL)** | NL agent over the DB; saved queries |

**Trend:** the unit of reuse is a **parameterized, runnable operation** — exactly
your definition of a "runbook." Differentiators are scheduling/triggers,
parameter UX, write/action steps, and notifications.

---

## 3. Product framing — four pillars, one platform

The four asks aren't four features; they're four **surfaces over a shared
execution + AI substrate**. Define them crisply so they don't blur:

- **Reports** (have): curated, refreshable dashboards of components.
- **Forecasting**: a *capability* applied inside reports/notebooks — project a
  time series forward with confidence bands; flag anomalies; (later) alert.
- **NL reporting**: an *interaction model* — (a) **prompt → report**
  (agent builds components), and (b) **results → narrative** (auto Insight
  Brief), with (c) conversational follow-up.
- **Notebooks**: **exploration** — a cell document (SQL / markdown / chart /
  light transform) where cells chain, cross-DB, and an agent can author/edit.
- **Runbooks**: **productionized tasks** — a named, **parameterized, re-runnable
  function**: typed inputs → ordered steps (query / transform / action / notify)
  → outputs; run on demand (later: scheduled / triggered).

> Notebook vs Runbook in one line: a **notebook** is for *figuring it out*; a
> **runbook** is for *doing it again, reliably, with inputs*. A great UX is to
> let a user **"promote" a notebook (or AI session) into a runbook.**

---

## 4. Shared platform primitives (build these once)

Most of the cost — and most of the leverage — is in connective tissue used by
all four pillars:

1. **Parameters / variables system** — typed inputs (string/number/date/enum/
   connection), default values, validation, and `{{param}}` binding into SQL.
   Used by reports (filters), runbooks (inputs), notebooks (widgets).
2. **Step / cell execution engine** — execute an ordered (later: DAG) set of
   steps, passing results between them, with cross-DB federation available to
   any step. Generalize today's per-component `RunReport` into a reusable runner.
3. **Time-series / forecast engine** (Go) — Holt-Winters / exponential smoothing
   + seasonal-naive baseline; STL-style decomposition for anomaly residuals;
   confidence intervals. Pure-Go so it runs in the desktop app (no Python
   runtime); use DuckDB for windowing/resampling.
4. **Narrative / insight service** — LLM prompt templates that turn result sets
   (+ forecast/anomaly metadata) into executive narratives. **Uses whatever the
   user configured in AI Settings** (Anthropic / OpenAI / Ollama / etc.) via the
   existing provider abstraction — no hardcoded default. By default send **only
   schema + aggregates**, never raw rows, so local-first privacy holds for any
   provider (and a local Ollama model gives a fully-offline path); cache results.
5. **Agentic runtime — multi-agent, multi-turn (Codex / Claude-Code style).**
   We already have the foundation: `internal/agent` is an **Eino-powered ReAct
   tool-calling loop** (`Toolset` = `Schema` / `RunSQL` / `SearchMemory`,
   `maxSteps`, provider-agnostic `BuildModel`/`ModelConfig`). Extend it in two
   directions:
   - **More tools:** `Forecast`, `DetectAnomalies`, `CreateReportComponent`,
     `WriteNarrative`, `AddNotebookCell`, `DefineRunbookParam`, `RunStep`, etc.,
     so the same loop can build reports, author notebook cells, and assemble
     runbooks — not just answer questions.
   - **Multi-agent + multi-turn:** an orchestrator/planner that delegates to
     specialized sub-agents (the 2026 pattern: *data parsing → analysis →
     narrative*), over **persisted, resumable conversation sessions** so a user
     can iterate turn-by-turn like Codex / Claude Code ("now add a forecast",
     "break it down by region", "save this as a runbook").
6. **Scheduler / trigger + notifications — local-first, deploy-ready.** Run
   schedules/alerts **in-app today**, but behind a **runtime-agnostic
   `Executor` + `Scheduler` interface** (enqueue job → run steps → emit result/
   notification) so the *same* runbook/report definitions can later run in the
   **server component (`cmd/server`) or a cloud worker** with zero changes to
   the definitions — only a new backend implementation. Design the job/queue
   contract now; ship the local implementation first.
7. **Sync schema extensions** — model notebooks/runbooks/forecast configs as
   first-class entities in local SQLite + Turso sync, with conflict handling.

---

## 5. Recommended roadmap (sequenced by leverage & dependency)

Ordering principle: **extend the surface we already have (reports) first**, ship
visible AI value fast, then build the two larger new surfaces.

### Phase 0 — Foundations (enabler, mostly invisible)
- Parameters/variables system (#4.1) and the generalized execution engine
  (#4.2). Forecast engine skeleton (#4.3).
- *Why first:* every later phase depends on these; doing them now avoids three
  bespoke implementations.

### Phase 1 — Forecasting + Anomalies + Narrative (NL reporting v1)
- Forecast as a first-class **report component / chart overlay** with confidence
  bands; anomaly highlighting on time-series charts; "explain this forecast/
  anomaly" via the narrative service (using the user's configured AI provider).
- **Auto Insight Brief**: one click on a report → LLM narrative (trends, drivers,
  outliers, forecast commentary).
- *Why:* highest value-to-effort; reuses the report system + chart renderer +
  AI layer; directly matches Power BI / ThoughtSpot table-stakes.

### Phase 2 — Full NL reporting on the multi-agent runtime (all three modes)
Per decision, deliver **all three** NL modes — but as facets of one agentic,
multi-turn experience built on `internal/agent`, not three separate features:
- **Prompt → report builder:** the orchestrator plans + assembles a
  multi-component report (SQL per component via NL→SQL + RAG, viz selection).
- **Conversational Q&A / Threads:** persisted, resumable sessions; ask, refine,
  drill down turn-by-turn (Codex / Claude-Code style).
- **Auto-narrative** (from Phase 1) becomes a tool the agent calls.
- *Why:* builds on Phase 1 + the agent foundation; matches Sigma Agents / Hex
  Threads / ThoughtSpot — and the multi-turn loop is the same substrate
  notebooks (Phase 3) and runbooks (Phase 4) reuse.

### Phase 3 — Notebooks (exploratory, AI-authored)
- Cell document (SQL / markdown / chart / light transform), cell chaining,
  cross-DB, agent that authors/edits cells (Hex Notebook Agent analog).
- Persist + Turso sync; reuse chart renderer + execution engine + parameters.
- *Why:* larger new surface; depends on the execution engine & planner maturing
  in Phases 0–2.

### Phase 4 — Runbooks (reusable parameterized tasks) + scheduling/alerts
- Runbook = typed inputs → ordered steps (query/transform/**action**/notify) →
  outputs; on-demand run with a parameter form; **promote-from-notebook** (and
  promote-from-agent-session).
- Scheduling/triggers + anomaly-driven alerts run **in-app first**, but on the
  runtime-agnostic `Executor`/`Scheduler` interface (#4.6) so the same runbooks
  later run server-side/cloud unchanged.
- *Why:* depends on parameters + execution engine + the agent runtime;
  "actions" (writes) need a dry-run/approval guardrail model.

> Faster-payoff alternative: if a flashy demo matters more than depth, swap
> Phases 2 and 3 (ship Notebooks before NL→Report). I recommend the order above
> because narrative + forecasting are the cheapest wins and de-risk the planner.

---

## 6. Key risks & constraints
- **No Python runtime on desktop** → forecasting/stats must be Go (or WASM/
  DuckDB). Keep models explainable (exponential smoothing, STL) before ARIMA/ML.
- **Scheduling on a desktop app** → in-app schedules only fire while the app is
  open. *Decision:* ship local-first, but put all execution/scheduling behind a
  **runtime-agnostic interface** so a server/cloud worker is a drop-in later —
  the runbook/report definitions never change. The risk to manage is keeping
  that boundary clean from day one.
- **LLM privacy/cost/latency** → *Decision:* features use **whatever the user
  configured in AI Settings** (no forced default). Preserve trust regardless of
  provider: send schema + aggregates, never raw rows by default; cache
  narratives; a local Ollama config yields a fully-offline path.
- **Runbook "actions" (writes)** → need dry-run, preview, and an approval/guard
  model; destructive steps must be explicit.
- **Collaborative notebooks + sync** → concurrent cell edits need conflict
  resolution (extend the existing Turso sync strategy).
- **Federation memory** → forecasting over large series should push aggregation
  into DuckDB, not pull rows into the app.

---

## 7. Differentiation / North Star
**"The AI-native data workspace that runs on your machine and across all your
databases."** Explore in **notebooks**, productionize as **runbooks**, present as
**reports** with **forecasts** and **AI narratives** — all local-first, all
cross-DB, with a local-model option. Incumbents are cloud + single-warehouse;
we are desktop + federation + privacy.

---

## 8. Decisions & remaining questions

**Decided (2026-06-20):**
- **Scheduling runtime:** local-first now, but behind a **runtime-agnostic
  `Executor`/`Scheduler`** designed so server/cloud deployment is a drop-in
  later. (See #4.6.)
- **NL reporting:** ship **all three** modes (prompt→report, conversational Q&A,
  auto-narrative) as one **multi-agent, multi-turn** experience on
  `internal/agent`, Codex/Claude-Code style. (See Phase 2 + #4.5.)
- **LLM posture:** use **whatever the user sets in AI Settings**; no forced
  default; never send raw rows by default. (See #4.4.)

**Still open:**
1. **Primary persona:** analytics engineers / founders / ops? Tunes whether we
   lean "BI narrative" or "operational runbooks."
2. **Forecasting depth for v1:** exponential smoothing + anomaly flags only, or
   also alerting in Phase 1? (Plan assumes flags first, alerts in Phase 4.)
3. **Runbook "actions":** read-only steps only at first, or allow writes /
   external calls from the start (needs a dry-run + approval guardrail model)?

---

## 9. Sources
- Hex Notebook Agent / Fall 2025 launch (Threads, Sonnet 4.5): https://hex.tech/blog/introducing-notebook-agent/ , https://hex.tech/blog/fall-2025-launch/
- Hex vs Deepnote: https://deepnote.com/compare/hex-vs-deepnote
- AI BI tools comparison (Holistics): https://www.holistics.io/bi-tools/ai-analytics/
- Sigma new AI/analytics features: https://www.sigmacomputing.com/resources/announcements/sigma-reveals-new-ai-bi-and-analytics-features , https://www.techtarget.com/searchbusinessanalytics/news/366630504/Sigma-Computing-intros-array-of-new-AI-analytics-tools
- ThoughtSpot Spotter / Insight Briefs (NL query comparison): https://querio.ai/articles/natural-language-query-business-intelligence-thoughtspot-vs-power-bi-vs-tableau-2026
- Power BI forecasting & anomaly detection: https://learn.microsoft.com/en-us/power-bi/visuals/power-bi-visualization-anomaly-detection , https://www.latentview.com/blog/how-powerbis-in-built-anomaly-detection-and-forecasting-capabilities-are-redefining-bi-analytics/
- Retool Workflows (parameterized, scheduled): https://docs.retool.com/queries/guides/workflows
- Hex data apps from SQL (parameterized): https://hex.tech/blog/sql-data-apps/
- Basedash (reusable actions) / Outerbase (EZQL): https://www.basedash.com/ , https://outerbase.com/ai/
- AI narrative/storytelling & 2026 trends: https://improvado.io/blog/ai-report-generation , https://www.domo.com/learn/article/best-data-storytelling-tools
