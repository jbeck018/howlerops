# Batched Schema Endpoint + Wails Binding Workflow — Implementation Handoff

**Status:** Ready to implement. Requires a **full dev environment** (Go toolchain with CGO,
the **Wails v3 CLI**, Node + `node_modules`, and the ability to **run the desktop app**). The
originating environment had no Wails CLI and could not run the frontend, so this is a handoff.

**Two deliverables, in order:**
1. A backend **`GetConnectionSchemaFull`** method that returns a connection's whole schema
   (schemas + tables + columns + FKs) in **one** call, replacing the frontend's
   `databases → tables → columns` fan-out.
2. The **Wails binding regeneration** needed to expose it — documented generally so it also
   unblocks any other new backend method (e.g. the connection-store unification).

> Context: PR #54 already made the *frontend* `getSchema` fetch schemas **concurrently**
> (`Promise.all`) — a binding-free win. This doc is the **deeper** fix: collapse the N round-trips
> into one server-side introspection. Do this only in a dev env where you can regen bindings and
> run the app.

---

## Part 1 — The batched schema endpoint

### 1.1 Problem

`frontend/src/store/schema-store.ts` `getSchema` issues, per connection (cold cache):
- 1× `api.schema.databases` (list schemas)
- S× `api.schema.tables` (tables per schema)
- S×T× `api.schema.columns` (columns + FKs per table)

Each is a Wails IPC round-trip. With in-flight dedup + a 24h cache this is a *cold-load* cost,
but on a wide DB it's hundreds of IPC calls before the tree renders. The fix: **one** IPC call
that introspects everything server-side (where it can run efficiently and in parallel).

### 1.2 Current backend introspection surface (ground truth)

`services/database.go` (the real work, talks to the DB drivers):
- `GetDatabaseSchema(connectionID, database string) ([]string, []database.TableInfo, error)` — `:226`
- `GetSchemas(connectionID string) ([]string, error)` — `:681`
- `GetTables(connectionID, schema string) ([]database.TableInfo, error)` — `:691`
- `GetTableStructure(connectionID, schema, table string) (*database.TableStructure, error)` — `:701`

`query_service.go` (the **Wails-exposed** wrapper service `QueryService`, registered at
`app_lifecycle.go:181` via `application.NewService(lc.querySvc)`):
- `GetDatabaseSchema(connectionID, database string) (*DatabaseSchema, error)` — `:287`
  (maps to DTO `DatabaseSchema{Schemas []string; Tables []TableInfo}`)
- `GetTableStructure(connectionID, schema, table string) (*TableStructure, error)` — `:307`
  (maps to DTO `TableStructure{Table, Columns, Indexes, ForeignKeys, Triggers, Statistics}`)
- `GetSchemas(connectionID string) ([]string, error)` — `:252`

DTOs in `dtos.go`: `TableInfo` (`:195`), `ColumnInfo` (`:205`), `ForeignKeyInfo` (`:233`),
`TableStructure` (`:244`), and `DatabaseSchema` (used by `GetDatabaseSchema`). The frontend
consumes these via `api.schema.databases/tables/columns`
(`frontend/src/lib/api-client/wails-client.ts:244` → `wailsEndpoints.schema.*` → name-based
bindings).

### 1.3 Backend design

Add **one** method that does the whole job server-side, parallelizing the per-table introspection
with bounded concurrency — exactly the pattern already used in
`pkg/schemadiff/comparator.go` `captureSnapshot` (added in PR #51: an `errgroup` with
`SetLimit(8)` over tables, each call borrowing its own pooled connection — safe and fast).

**New DTO** (in `dtos.go`):

```go
// FullDatabaseSchema is the complete introspection of a connection's schema in
// one payload: every schema, its tables, and each table's full structure.
type FullDatabaseSchema struct {
    Schemas    []string          `json:"schemas"`
    Tables     []TableInfo       `json:"tables"`      // flat list across schemas
    Structures []TableStructure  `json:"structures"`  // one per table (cols, indexes, FKs)
}
```

**New `DatabaseService` method** (in `services/database.go`) — reuses existing per-table logic but
runs it concurrently:

```go
// GetConnectionSchemaFull introspects every schema + table for a connection in a
// single call, parallelizing per-table structure reads with bounded concurrency.
func (s *DatabaseService) GetConnectionSchemaFull(connectionID string) (
    []string, []database.TableInfo, []*database.TableStructure, error,
) {
    schemas, err := s.GetSchemas(connectionID)
    if err != nil {
        return nil, nil, nil, err
    }

    // Gather all (schema, table) refs (one GetTables call per schema).
    type ref struct{ schema, table string }
    var refs []ref
    var tables []database.TableInfo
    for _, schema := range schemas {
        ts, err := s.GetTables(connectionID, schema)
        if err != nil {
            return nil, nil, nil, fmt.Errorf("tables for %s: %w", schema, err)
        }
        tables = append(tables, ts...)
        for _, t := range ts {
            refs = append(refs, ref{schema: schema, table: t.Name})
        }
    }

    // Introspect each table's structure concurrently (bounded). Each call uses
    // its own pooled connection (same safety basis as pkg/schemadiff).
    structures := make([]*database.TableStructure, len(refs))
    g, ctx := errgroup.WithContext(context.Background())
    g.SetLimit(8)
    for i, r := range refs {
        i, r := i, r
        g.Go(func() error {
            st, err := s.getTableStructureCtx(ctx, connectionID, r.schema, r.table) // see note
            if err != nil {
                return fmt.Errorf("structure %s.%s: %w", r.schema, r.table, err)
            }
            structures[i] = st
            return nil
        })
    }
    if err := g.Wait(); err != nil {
        return nil, nil, nil, err
    }
    return schemas, tables, structures, nil
}
```

> Note: `GetTableStructure` (`:701`) likely takes no ctx today. Either add a ctx-aware internal
> variant `getTableStructureCtx`, or call the existing method (acceptable — the DB pool bounds
> real concurrency). Keep `SetLimit(8)` to avoid a thundering herd.

**New `QueryService` wrapper** (in `query_service.go`) — maps to the DTO, exactly mirroring the
field mapping already in `GetDatabaseSchema` (`:293`) and `GetTableStructure` (`:314`):

```go
// GetConnectionSchemaFull returns the entire schema of a connection in one call.
func (s *QueryService) GetConnectionSchemaFull(connectionID string) (*FullDatabaseSchema, error) {
    schemas, tables, structures, err := s.deps.DatabaseService.GetConnectionSchemaFull(connectionID)
    if err != nil {
        return nil, err
    }
    out := &FullDatabaseSchema{Schemas: schemas}
    out.Tables = make([]TableInfo, len(tables))
    for i, t := range tables {
        out.Tables[i] = TableInfo{Schema: t.Schema, Name: t.Name, Type: t.Type,
            Comment: t.Comment, RowCount: t.RowCount, SizeBytes: t.SizeBytes}
    }
    out.Structures = make([]TableStructure, 0, len(structures))
    for _, st := range structures {
        if st == nil { continue }
        out.Structures = append(out.Structures, mapTableStructureDTO(st)) // extract from GetTableStructure
    }
    return out, nil
}
```

Refactor the column/index/FK mapping inside `GetTableStructure` (`:314-373`) into a shared
`mapTableStructureDTO(*database.TableStructure) TableStructure` so both methods use it.

**Why this is safe:** it's additive (a new method), reuses validated introspection, and mirrors
the concurrency pattern already shipped in `pkg/schemadiff`. No existing method changes.

### 1.4 Frontend rewiring

After the binding exists (Part 2), change `schema-store.ts` `getSchema` to **one** call:

1. Replace the `api.schema.databases` + per-schema `tables` + per-table `columns` block with a
   single `GetConnectionSchemaFull(sessionId)` call.
2. Build the `SchemaNode[]` tree from the response: group `Structures` by `Table.Schema`, map each
   structure's `Columns`/`ForeignKeys` exactly as the current code does (reuse
   `normalizeForeignKeys` / `normalizeColumnInfo` / `formatColumnName` — `schema-store.ts`).
3. **Keep** the 24h cache, in-flight dedup, `loading`/`errors` state, and the `connectionName`
   cache entry — only the fetch shape changes.
4. Add the binding wrapper to the api-client chain so `getSchema` can call it: add
   `GetConnectionSchemaFull` to `frontend/src/lib/api-client/wails-client.ts` (and a REST analogue
   in `rest-client.ts` if cloud mode needs it), exposed as e.g. `api.schema.full(sessionId)`.

Preserve the exact `SchemaNode` shape so the schema tree / visualizer / autocomplete consumers
are unaffected.

---

## Part 2 — Wails binding workflow (how to expose a new Go method)

This repo uses **Wails v3**. The frontend never calls Go directly; it calls **generated bindings**
that dispatch **by name** at runtime.

### 2.1 How bindings work here
- Go services are registered in `app_lifecycle.go:176-190`:
  `application.NewService(lc.connectionSvc)`, `application.NewService(lc.querySvc)`, … Any
  **public method** on a registered service struct is exposable.
- Generated TS lives in `frontend/bindings/github.com/jbeck018/howlerops/` — e.g.
  `queryservice.ts`, and `app.ts` which contains:
  - `servicePrefixesByMethod` — map of method name → owning service FQN
    (e.g. `GetDatabaseSchema: ['main.QueryService']`).
  - `export const GetX = makeBinding('GetX')`.
  - `invokeBinding` → `runtime.Call.ByName('main.QueryService.GetX', ...args)` (`app.ts:~160`).
- **Dispatch is name-based**, so the runtime can call any registered public method by its FQN.
  The generated TS is for typing + the prefix map.

### 2.2 Regenerate bindings (the supported path)
```bash
# Install the CLI if missing (Makefile check-wails does this):
go install github.com/wailsapp/wails/v3/cmd/wails3@latest   # -> $(go env GOPATH)/bin/wails3

# Regenerate after adding/the Go method (Makefile target):
make bindings        # == $(WAILS) generate bindings  (WAILS = $GOPATH/bin/wails3)
```
This regenerates `frontend/bindings/...`: adds `GetConnectionSchemaFull` to `queryservice.ts`,
to `app.ts`'s `servicePrefixesByMethod` + an `export const`, and emits the model type for
`FullDatabaseSchema` in `models.ts`. Commit the regenerated files.

### 2.3 Manual fallback (NOT preferred — only if the CLI is truly unavailable)
Because dispatch is name-based, you *can* hand-wire a binding without regenerating:
1. Add the public Go method to `QueryService`.
2. In `frontend/bindings/.../app.ts`: add
   `GetConnectionSchemaFull: ['main.QueryService']` to `servicePrefixesByMethod` and
   `export const GetConnectionSchemaFull = makeBinding('GetConnectionSchemaFull')`.
3. Call it; the runtime resolves `main.QueryService.GetConnectionSchemaFull` by name.
This works for the **call**, but you get **no generated TS types** for `FullDatabaseSchema` and
no compile-time safety — you'd hand-write the response interface. **Verify at runtime** (the
manual route can fail silently with `binding unavailable` if the method isn't actually registered/
public). Prefer `make bindings`.

### 2.4 Gotchas
- CGO is required (SQLite driver) — `CGO_ENABLED=1`, a C toolchain, and the GTK/WebKit dev libs
  for a desktop build. `make deps` / `make dev` set this up.
- After changing Go service method **signatures**, always `make bindings` — stale bindings cause
  runtime arg-mismatch errors.
- Keep method names unique across services, or add the correct prefix to `servicePrefixesByMethod`
  (a method on the wrong-prefixed service throws and falls through the candidate list).

---

## Test plan

### Backend (Go) — runs in any Go env
- Unit-test `mapTableStructureDTO` (field parity with `GetTableStructure`).
- Integration-test `GetConnectionSchemaFull` against a seeded DB (the repo has
  docker-compose test DBs and `internal/testutil`): assert schemas + tables + per-table columns/FKs
  match N individual `GetTableStructure` calls. Mirror the style of the DuckDB federation
  integration test and `pkg/schemadiff` tests.
- Confirm `go build -tags duckdb .`, `go build ./cmd/server`, `go vet`, `golangci-lint` pass —
  **this PR touches Go**, so the repo's Go-path-gated CI **will** run on it (unlike the FE-only PRs).

### Frontend — needs the running app
- `tsc --noEmit` after the rewire.
- Smoke-test (`make dev`): open the schema tree for a multi-schema connection; confirm it matches
  the pre-change tree (schemas, tables, columns, FK indicators), loads in **one** backend call
  (check devtools/logs), and that cache + dedup still work (second open is instant, force-refresh
  re-fetches).

---

## Acceptance criteria
- [ ] `FullDatabaseSchema` DTO + `DatabaseService.GetConnectionSchemaFull` +
      `QueryService.GetConnectionSchemaFull` added; `mapTableStructureDTO` extracted and shared.
- [ ] Per-table introspection bounded-concurrent (errgroup `SetLimit`), each via its own pooled conn.
- [ ] Bindings regenerated via `make bindings` and committed.
- [ ] `getSchema` rewired to one call + response mapper; `SchemaNode` shape unchanged; cache/dedup
      preserved.
- [ ] Go CI green; FE smoke-test confirms identical schema tree in one round-trip.
- [ ] No change to the visualizer/autocomplete consumers of `getSchema`.

---

## Relationship to the other handoff
See `docs/connection-store-unification-plan.md` for the connection-store unification (Approach A).
That work **also** needs the Wails binding workflow in Part 2 if it adds any new backend method;
otherwise it's frontend-only. Both items were documented (not executed) because they require a
runnable dev environment with the Wails toolchain.
