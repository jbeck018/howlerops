# Connection Store Unification — Implementation Handoff (Approach A)

**Status:** Ready to implement. Authored by the tech-debt pass after a 5-agent review.
**Audience:** An engineer/agent working in a **full dev environment** (Node + `node_modules`
installed, the Wails v3 CLI available, and the ability to **run the desktop app** to smoke-test).
The originating environment could not run the frontend or regenerate Wails bindings, so this
work was documented rather than executed.

**Decision already made by the product owner:**
1. An org-shared connection and a local connection are the **same underlying record**
   (same `id`, just shared to an org) — i.e. unification is a **join by `id`**, not a merge of
   two unrelated id spaces.
2. Implement **Approach A: a single normalized facade store** fed by two pluggable sources
   (local = Wails, remote = REST), as the long-term source of truth.

> ⚠️ Before writing code, **verify assumption #1 against the backend** (see
> [§7 Backend correctness check](#7-backend-correctness-check)). If a connection's *local* id
> and its *server* id are NOT guaranteed equal, the merge key must change. The whole design
> hinges on this.

---

## 1. Goal

Today "a database connection" is represented by **two independent Zustand stores** that don't
share state. Consolidate them behind **one canonical model and one facade store**, without
regressing the (heavily used, battle-tested) local-connection operational logic. The end state:

- One canonical `ConnectionView` type used by all UI consumers.
- One `useConnections` facade exposing both **operational** actions (connect, create, remove,
  switch database) and **sharing** actions (share/unshare/fetch-shared).
- The two existing stores become **internal sources** behind the facade and are retired once all
  consumers migrate.

Non-goals: changing the backends, changing credential handling, or changing the desktop↔cloud
split. The facade *composes* existing behavior; it does not rewrite it.

---

## 2. Current state (ground truth)

### 2.1 `src/store/connection-store.ts` — LOCAL / OPERATIONAL (≈780 lines)

The pervasive, core store. **34 consumers** (see §6.3).

- **Type:** `DatabaseConnection` (camelCase) — `frontend/src/store/connection-store.ts:46`.
  Fields: `id, sessionId?, name, type (DatabaseTypeString), host?, port?, database, username?,
  password?, sslMode?, useTunnel?, sshTunnel?, useVpc?, vpcConfig?, parameters?, isConnected,
  lastUsed?, environments?`.
- **Backend:** Wails — `api.connections.*` (`src/lib/api-client/wails-client.ts:63`):
  `list, create, save, remove, listDatabases, switchDatabase`. Plus direct bindings
  `SQLiteSaveConnection / SQLiteGetConnections / SQLiteDeleteConnection`.
- **Owns:** credentials (`password`, SSH/VPC), **live state** (`sessionId`, `isConnected`),
  `environments` tags, `lastUsed`.
- **Actions** (`ConnectionState`, `connection-store.ts:74`): `addConnection, updateConnection,
  removeConnection, setActiveConnection, setAutoConnect, connectToDatabase,
  disconnectFromDatabase, fetchDatabases, switchDatabase, setEnvironmentFilter,
  getFilteredConnections, addEnvironmentToConnection, removeEnvironmentFromConnection,
  refreshAvailableEnvironments`. Plus state: `activeConnection, lastActiveConnectionId,
  autoConnectEnabled, isConnecting, activeEnvironmentFilter, availableEnvironments,
  deletedConnectionIds`.
- **Persistence (dual):**
  - `persist` middleware → localStorage key `connection-store`, **partialized** to strip
    `sessionId/isConnected/lastUsed/password` and SSH passwords/keys (`connection-store.ts:597`).
  - Best-effort **SQLite mirror** via `syncConnectionToSQLite` / `removeConnectionFromSQLite`
    (`connection-store.ts:106`, `:138`) for cross-WebView-origin persistence.
  - Passwords live in **sessionStorage** via `@/lib/secure-storage`
    (`migratePasswordsFromLocalStorage`, `getSecureStorage`).
  - `onRehydrateStorage` clears live state + passwords on reload (`connection-store.ts:617`).
- **Init:** `initializeConnectionStore()` (`connection-store.ts:660`) restores connections from
  SQLite on startup (handles empty localStorage after origin change / v2→v3 migration), honoring
  `deletedConnectionIds`.
- **Global handle:** `window.__connectionStore` (`connection-store.ts:652`) for cross-store
  access without circular imports.

### 2.2 `src/store/connections-store.ts` — ORG / SHARING (≈496 lines)

Narrow store. **4 consumers** (see §6.3). Header comment: "managing database connections with
organization sharing."

- **Type:** `Connection` (snake_case) — `frontend/src/lib/api/connections.ts:20`.
  Fields: `id, user_id, organization_id?, name, description?, database_type, host, port,
  database_name, username, ssl_enabled, visibility ('personal'|'shared'), created_at,
  updated_at, created_by_email?, last_used?`. **Password is never returned by the server.**
- **Backend:** REST via `@/lib/api/connections` (`/api/connections*`): `getConnections,
  getOrganizationConnections, createConnection, updateConnection, deleteConnection,
  shareConnection, shareConnectionWithCredential, unshareConnection`, plus password/test helpers.
  > Note: this only functions when a **cloud server** is reachable (the desktop app is otherwise
  > Wails-only). Treat the remote source as **optional/absent** in pure-desktop mode.
- **Owns:** sharing metadata (`organization_id`, `visibility`, `created_by_email`),
  server timestamps. **No credentials, no live state.**
- **State:** `connections: Connection[]` (personal + accessible shared), `sharedConnections:
  Connection[]`, `loading`, `error`.
- **Actions** (`connections-store.ts:51`): `fetchConnections, fetchSharedConnections(orgId),
  createConnection, updateConnection, deleteConnection, shareConnection, unshareConnection,
  getConnectionsByOrg, getPersonalConnections, clearError`.

### 2.3 Why a naive flatten is wrong (already established)

- Different field naming (camelCase vs snake_case) and field sets.
- Server records carry **no password** → cannot be used to actually connect.
- `sessionId`/`isConnected` have **no server equivalent**.
- The two backends differ (Wails local vs REST cloud), and the remote source may be **absent**
  on desktop.

Hence: **canonical model + adapters + join by `id`**, with explicit ownership rules.

---

## 3. Target architecture

```
            ┌────────────────────────────────────────────┐
 UI ───────▶│  useConnections (facade hook/store)         │
            │   • connections: ConnectionView[]           │
            │   • operational actions (delegate → local)  │
            │   • sharing actions     (delegate → remote) │
            └───────────────┬───────────────┬─────────────┘
                            │               │
                 ┌──────────▼──────┐  ┌──────▼───────────┐
                 │ LocalSource     │  │ RemoteSource     │
                 │ (connection-    │  │ (connections-    │
                 │  store, Wails)  │  │  store, REST)    │
                 └─────────────────┘  └──────────────────┘
```

**Ownership rule (authoritative merge):** for a connection present in both sources (same `id`):

| Concern | Owner |
|---|---|
| `name, type, host, port, database, username` | Local if present, else Remote |
| `password`, SSH/VPC, `sessionId`, `isConnected`, `environments`, `lastUsed` | **Local only** |
| `organizationId`, `visibility`, `sharedByEmail`, server timestamps | **Remote only** |
| `source` provenance (`'local' | 'remote' | 'both'`) | computed |

A **remote-only** connection (shared *to* you, not present locally) appears in the list with
`isConnected:false`, no `sessionId`, `canConnect:false` until imported locally.

---

## 4. Detailed design

### 4.1 Canonical type — new file `src/store/connection-view.ts`

```ts
import type { DatabaseConnection, DatabaseTypeString } from './connection-store'
import type { Connection, ConnectionVisibility } from '@/lib/api/connections'

/** Single canonical view of a connection, joined from local + remote sources. */
export interface ConnectionView {
  id: string
  name: string
  type: DatabaseTypeString
  host?: string
  port?: number
  database: string
  username?: string

  // Operational (LOCAL only) ------------------------------------------------
  sessionId?: string
  isConnected: boolean
  lastUsed?: Date
  environments?: string[]
  /** True when this connection exists locally with usable credentials. */
  hasLocalCredentials: boolean
  useTunnel?: boolean
  useVpc?: boolean

  // Sharing (REMOTE only) ---------------------------------------------------
  organizationId?: string | null
  visibility?: ConnectionVisibility
  sharedByEmail?: string
  description?: string

  // Provenance --------------------------------------------------------------
  source: 'local' | 'remote' | 'both'
  /** Can the user actually open a session? (needs a local record + creds) */
  canConnect: boolean
}
```

### 4.2 Mappers (pure, unit-testable)

```ts
export function fromLocal(c: DatabaseConnection): ConnectionView {
  return {
    id: c.id, name: c.name, type: c.type, host: c.host, port: c.port,
    database: c.database, username: c.username,
    sessionId: c.sessionId, isConnected: c.isConnected, lastUsed: c.lastUsed,
    environments: c.environments,
    hasLocalCredentials: true,
    useTunnel: c.useTunnel, useVpc: c.useVpc,
    source: 'local', canConnect: true,
  }
}

const DB_TYPE_FROM_SERVER: Record<string, DatabaseTypeString> = {
  postgres: 'postgresql', postgresql: 'postgresql', mysql: 'mysql',
  mariadb: 'mariadb', sqlite: 'sqlite', mssql: 'mssql', mongodb: 'mongodb',
  clickhouse: 'clickhouse', elasticsearch: 'elasticsearch',
  opensearch: 'opensearch', tidb: 'tidb',
} // NOTE: confirm exact server `database_type` strings against the Go DTOs.

export function fromRemote(c: Connection): ConnectionView {
  return {
    id: c.id, name: c.name,
    type: DB_TYPE_FROM_SERVER[c.database_type] ?? (c.database_type as DatabaseTypeString),
    host: c.host, port: c.port, database: c.database_name, username: c.username,
    isConnected: false, hasLocalCredentials: false,
    organizationId: c.organization_id, visibility: c.visibility,
    sharedByEmail: c.created_by_email, description: c.description,
    lastUsed: c.last_used ? new Date(c.last_used) : undefined,
    source: 'remote', canConnect: false,
  }
}
```

### 4.3 Merge by id (the heart of Approach A)

```ts
/** Join local + remote by id. Local owns operational fields; remote overlays sharing. */
export function mergeConnections(
  local: DatabaseConnection[],
  remote: Connection[],
): ConnectionView[] {
  const byId = new Map<string, ConnectionView>()

  for (const l of local) byId.set(l.id, fromLocal(l))

  for (const r of remote) {
    const existing = byId.get(r.id)
    if (!existing) { byId.set(r.id, fromRemote(r)); continue }
    // Overlay remote sharing fields onto the local (operational) base.
    byId.set(r.id, {
      ...existing,
      organizationId: r.organization_id,
      visibility: r.visibility,
      sharedByEmail: r.created_by_email,
      description: existing.description ?? r.description,
      source: 'both',
    })
  }

  // Stable order: local order first (preserves the user's list), then remote-only.
  const order = [...local.map(l => l.id), ...remote.map(r => r.id)]
  const seen = new Set<string>()
  const out: ConnectionView[] = []
  for (const id of order) {
    if (seen.has(id)) continue
    seen.add(id)
    const v = byId.get(id)
    if (v) out.push(v)
  }
  return out
}
```

**Edge cases the merge must handle (write tests for each):**
1. id only local → `source:'local'`, `canConnect:true`.
2. id only remote → `source:'remote'`, `canConnect:false`, `hasLocalCredentials:false`.
3. id in both → operational from local, sharing from remote, `source:'both'`.
4. remote source empty/absent (desktop, no cloud) → result == local mapped 1:1.
5. duplicate ids within a source → first wins (and log a warning; shouldn't happen).
6. `database_type` value not in the map → fall through to raw string (and add it to the map).

### 4.4 Facade — new file `src/store/use-connections.ts`

Implement as a **hook that composes both stores** (NOT a third stateful store — avoids
cross-store subscription sync bugs). React re-renders when either underlying store changes.

```ts
import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useConnectionStore } from './connection-store'
import { useConnectionsStore } from './connections-store'
import { mergeConnections, type ConnectionView } from './connection-view'

export function useConnections() {
  const localConns = useConnectionStore(useShallow(s => s.connections))
  const remoteConns = useConnectionsStore(useShallow(s => s.connections))

  const connections: ConnectionView[] = useMemo(
    () => mergeConnections(localConns, remoteConns),
    [localConns, remoteConns],
  )

  // Operational actions (delegate to local store)
  const connect       = useConnectionStore(s => s.connectToDatabase)
  const disconnect    = useConnectionStore(s => s.disconnectFromDatabase)
  const addConnection = useConnectionStore(s => s.addConnection)
  const removeLocal   = useConnectionStore(s => s.removeConnection)
  const switchDatabase= useConnectionStore(s => s.switchDatabase)
  const fetchDatabases= useConnectionStore(s => s.fetchDatabases)

  // Sharing actions (delegate to remote store)
  const fetchRemote     = useConnectionsStore(s => s.fetchConnections)
  const fetchShared     = useConnectionsStore(s => s.fetchSharedConnections)
  const shareConnection = useConnectionsStore(s => s.shareConnection)
  const unshareConnection = useConnectionsStore(s => s.unshareConnection)

  return {
    connections,
    // operational
    connect, disconnect, addConnection, removeLocal, switchDatabase, fetchDatabases,
    // sharing
    fetchRemote, fetchShared, shareConnection, unshareConnection,
  }
}
```

Also add a **non-React accessor** for the ~9 `getState()` call sites (export/import, query-engine,
schema-visualizer):

```ts
export function getConnectionsSnapshot(): ConnectionView[] {
  return mergeConnections(
    useConnectionStore.getState().connections,
    useConnectionsStore.getState().connections,
  )
}
```

### 4.5 Provider/init wiring

- Keep `initializeConnectionStore()` as-is (local hydration from SQLite).
- Where the app currently triggers `useConnectionsStore().fetchConnections()` (cloud login),
  keep it — the facade reflects whatever the remote store has fetched. Ensure the remote fetch is
  **gracefully no-op when no cloud server** (it already throws → caught; confirm it doesn't spam
  errors on desktop, and gate it behind "is cloud/authed" via `useAuthStore`).

---

## 5. Phased migration plan (low-risk order)

> Each phase is independently shippable and reversible. **Run the app and smoke-test after each.**

**Phase 0 — Foundations (no consumer change).**
- Add `connection-view.ts` (types + mappers + `mergeConnections`) and `use-connections.ts`.
- Add unit tests for `mergeConnections` (see §8). `npm run typecheck` + `npm run test`.

**Phase 1 — Migrate the 4 remote consumers to the facade.** These are the dedup win.
- `src/components/reports/connection-picker.tsx` — currently reads `useConnectionsStore`
  (`:41-43`, list/loading/fetch). Switch to `useConnections()` → `connections` + `fetchRemote`.
  The picker now also shows local connections (desired: one unified list).
- `src/pages/settings.tsx`, `src/pages/connections-team-tab.tsx` — review each `useConnectionsStore`
  usage; replace list reads with the facade, keep sharing actions via facade's sharing delegates.
- `src/components/sharing/INTEGRATION_EXAMPLES.tsx` — appears to be example/doc code; update or
  leave (confirm it isn't mounted).

**Phase 2 — Point list-reading local consumers at the facade (optional, incremental).**
- Components that only *read the list* (pickers, selectors, indicators) can switch
  `useConnectionStore(s => s.connections)` → `useConnections().connections`. This gives them the
  org/sharing overlay (badges like "shared", `sharedByEmail`).
- Components that call **operational actions** (query-editor, sidebar, connection-manager) can
  keep using `useConnectionStore` directly OR use the facade's delegated actions — both are fine.
  Do NOT rush these; the facade re-exports the same action references.

**Phase 3 — Retire the raw stores (only after all consumers migrate).**
- Replace remaining direct `useConnectionsStore`/`useConnectionStore` reads.
- Keep `connection-store` internals (it's the LocalSource); the goal is that **UI no longer
  imports the raw stores directly** — it goes through `useConnections` / `getConnectionsSnapshot`.
- Optionally rename for clarity: `connection-store.ts` → keep (LocalSource),
  `connections-store.ts` → `connection-sharing-store.ts` (RemoteSource) to end the
  singular/plural confusion that caused this whole ticket.

---

## 6. Inventory (so nothing is missed)

### 6.1 Remote-store consumers (migrate first — small)
- `src/components/reports/connection-picker.tsx`
- `src/pages/connections-team-tab.tsx`
- `src/pages/settings.tsx`
- `src/components/sharing/INTEGRATION_EXAMPLES.tsx` (verify if mounted)

### 6.2 Non-React `getState()` access (use `getConnectionsSnapshot()` or keep local)
- `src/store/query-engine-store.ts:211,302`
- `src/components/schema-visualizer/schema-visualizer.tsx:1110`
- `src/components/query-editor.tsx:706,775`
- `src/components/layout/subnav/use-tab-actions.ts:88`
- `src/components/query-editor-parts/hooks/use-connection-databases.ts:44,68`
- `src/components/connection-manager/hooks/use-connection-actions.ts:92`
- `src/lib/export-import/export-service.ts:38,182`, `import-service.ts:86,103,154,307`
- `src/store/connection-store.ts` internal (leave)

> These mostly need **operational** data (sessionId, credentials) → they should keep using the
> **local** store/snapshot. Only switch them to the merged snapshot if they need sharing fields.

### 6.3 Local-store consumers (34 files — Phase 2/3, mostly read-only list usage)
`connection-database-picker, connection-manager/*, connection-schema-viewer,
debug/multi-db-diagnostics, environment-manager, environment-tag-picker, foreign-key-card,
layout/connection-row, layout/header, layout/sidebar, layout/subnav/use-tab-actions,
multi-db-connection-selector, query-editor-parts/hooks/use-connection-databases, query-editor,
query-results/hooks/use-database-selector, results-panel, schema-visualizer, tier-settings-panel,
usage-stats, value-indicators/connection-limit-indicator, examples/upgrade-integration-example,
hooks/use-ai-schema-context, hooks/use-query-mode, hooks/use-schema-introspection,
hooks/use-schema-refresh, lib/export-import/*, lib/query-engine/runtime, pages/data-catalog,
pages/schema-diff, store/query-engine-store`.

---

## 7. Backend correctness check (DO THIS FIRST)

The entire join hinges on **local id == server id for the same connection**. Verify:

1. **Create flow:** When a connection is created locally (`api.connections.create`, Wails) and
   later shared, does the server record reuse the **same `id`**? Inspect the Go side:
   - `connection_service.go` (Wails `CreateConnection`) — what id does it assign?
   - `internal/connections/*` and the REST `/api/connections` POST handler
     (`createConnection` in `@/lib/api/connections`) — does sharing **upload the existing local
     connection with its id**, or does the server **mint a new id**?
2. If the server mints a **new** id, then the merge key must be a **stable natural key** instead
   (e.g. normalized `type|host|port|database|username`), or the share response must return the
   server id to store back on the local record (add `serverId?` to `DatabaseConnection`). Pick
   one and update `mergeConnections` + the mappers accordingly. **Flag this to the product owner
   if it contradicts the "same record" assumption.**
3. **`database_type` strings:** confirm the exact server values (postgres vs postgresql, etc.) to
   finish `DB_TYPE_FROM_SERVER` in §4.2. Source: the Go DTO / proto for the REST connection.

---

## 8. Test plan

### 8.1 Unit (add `src/store/connection-view.test.ts`, runs under vitest)
- `mergeConnections`: each edge case in §4.3 (local-only, remote-only, both, empty remote,
  duplicate ids, unknown db type, order stability).
- `fromLocal`/`fromRemote`: field mapping + `database_name→database`, `ssl_enabled`, date parsing.

### 8.2 Manual smoke-test (the part that needs the running app — **the reason this is a handoff**)
Run the desktop app (`make dev` / `wails3 dev`) and verify:
1. **Desktop, no cloud login:** connections list looks identical to before (remote source empty).
   Connect / disconnect / switch DB / create / delete all work. No console error spam from the
   remote fetch.
2. **Cloud login:** shared/org connections appear; a connection that's both local and shared
   shows ONE row with sharing badge (not two rows). `sharedByEmail`/`visibility` render.
3. **Reports → connection picker:** shows the unified list; selecting a connection still works.
4. **Share / unshare** from settings/team tab updates the row's `visibility` without duplicating.
5. **Reload** (rehydrate): credentials/live-state cleared as before; `initializeConnectionStore`
   restores from SQLite; deleted connections stay deleted (`deletedConnectionIds`).
6. **Query editor**: auto-connect, multi-DB `@conn` autocomplete, and run still work
   (these read `useConnectionStore.getState()` — confirm unchanged).

### 8.3 Regression watch
- localStorage `connection-store` partialize still strips passwords (§2.1).
- sessionStorage password flow intact (`secure-storage`).
- `window.__connectionStore` still set.

---

## 9. Acceptance criteria
- [ ] §7 backend id-equality verified (or merge key adjusted + product owner informed).
- [ ] `connection-view.ts` + `use-connections.ts` added; unit tests green; `tsc --noEmit` clean.
- [ ] 4 remote consumers migrated to the facade; the picker shows a unified, de-duped list.
- [ ] Manual smoke-test §8.2 passes in both desktop-only and cloud-login modes.
- [ ] No duplicate rows for a connection that is both local and shared.
- [ ] No behavior change for operational flows (connect/run/multi-DB).
- [ ] (Stretch) raw stores no longer imported by UI; `connections-store` renamed to
      `connection-sharing-store`.

---

## Appendix A — Deferred: true batched schema endpoint

Separately deferred to a dev env with the Wails CLI. Context:

- `src/store/schema-store.ts` `getSchema` previously walked `databases → tables → columns`
  with the schemas loop **sequential**; the tech-debt pass made the schemas fetch **concurrent**
  (`Promise.all`), which captured most of the latency win **without new bindings**. (Merged.)
- The deeper fix is a **single backend call**: add `GetConnectionSchemaFull(connectionID)` to
  `QueryService` (Go) returning schemas+tables+columns+FKs in one payload (introspect server-side,
  parallelizing per-table like `pkg/schemadiff/comparator.go` now does with a bounded errgroup),
  then **regenerate Wails bindings** (`make bindings` / `wails3 generate bindings` — the CLI was
  absent in the originating env), and rewire `getSchema` to one call + a response mapper.
- Binding dispatch is **name-based** (`runtime.Call.ByName('main.QueryService.<Method>')`, see
  `frontend/bindings/.../app.ts` `invokeBinding`), so once bindings are regenerated the method is
  callable; a hand-written binding is possible but unverifiable without running the app.

## Appendix B — What shipped in the tech-debt pass (context)
PRs #50–#54 (all merged): data races + goroutine/timer leaks + hot-path (#50); N+1 introspection,
RAG memory bound, O(1) embedding LRU (#51); WebSocket reconnect/render storms + multi-DB schema
stale-race + dead code (#52); data-catalog virtualization + dead worker subsystem removal (#53);
getSchema concurrency (#54). This unification and the batched endpoint are the two items that
required a runnable dev environment and were documented instead.
