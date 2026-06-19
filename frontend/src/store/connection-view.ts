/**
 * Canonical connection view + join logic (Approach A).
 *
 * The app historically had two independent stores for "a database connection":
 *  - `connection-store` (LOCAL / operational, Wails) — owns credentials + live state.
 *  - `connections-store` (REMOTE / org sharing, REST) — owns sharing metadata.
 *
 * This module defines the single canonical `ConnectionView` consumed by the UI
 * and the pure join (`mergeConnections`) that overlays remote sharing fields onto
 * the local operational record, keyed by `id`. The mappers and merge are pure and
 * unit-tested (see `connection-view.test.ts`).
 *
 * @module store/connection-view
 */

import type { Connection, ConnectionVisibility } from '@/lib/api/connections'

import type { DatabaseConnection, DatabaseTypeString } from './connection-store'

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

/**
 * Map the server's `database_type` string onto the frontend `DatabaseTypeString`.
 * Confirmed against the Go drivers (`pkg/database`): the canonical type strings
 * are postgresql/mysql/sqlite/mssql/mariadb/elasticsearch/opensearch/clickhouse/
 * mongodb/tidb. `postgres` is accepted as an alias for robustness.
 */
const DB_TYPE_FROM_SERVER: Record<string, DatabaseTypeString> = {
  postgres: 'postgresql',
  postgresql: 'postgresql',
  mysql: 'mysql',
  mariadb: 'mariadb',
  sqlite: 'sqlite',
  mssql: 'mssql',
  mongodb: 'mongodb',
  clickhouse: 'clickhouse',
  elasticsearch: 'elasticsearch',
  opensearch: 'opensearch',
  tidb: 'tidb',
}

/** Map a LOCAL (Wails) connection to the canonical view. */
export function fromLocal(c: DatabaseConnection): ConnectionView {
  return {
    id: c.id,
    name: c.name,
    type: c.type,
    host: c.host,
    port: c.port,
    database: c.database,
    username: c.username,
    sessionId: c.sessionId,
    isConnected: c.isConnected,
    lastUsed: c.lastUsed,
    environments: c.environments,
    hasLocalCredentials: true,
    useTunnel: c.useTunnel,
    useVpc: c.useVpc,
    source: 'local',
    canConnect: true,
  }
}

/** Map a REMOTE (REST/org) connection to the canonical view. */
export function fromRemote(c: Connection): ConnectionView {
  return {
    id: c.id,
    name: c.name,
    type: DB_TYPE_FROM_SERVER[c.database_type] ?? (c.database_type as DatabaseTypeString),
    host: c.host,
    port: c.port,
    database: c.database_name,
    username: c.username,
    isConnected: false,
    hasLocalCredentials: false,
    organizationId: c.organization_id,
    visibility: c.visibility,
    sharedByEmail: c.created_by_email,
    description: c.description,
    lastUsed: c.last_used ? new Date(c.last_used) : undefined,
    source: 'remote',
    canConnect: false,
  }
}

/**
 * Join local + remote connections by `id`. Local owns operational fields
 * (credentials, live state); remote overlays sharing metadata. Order is stable:
 * local order first (preserves the user's list), then remote-only connections.
 */
export function mergeConnections(
  local: DatabaseConnection[],
  remote: Connection[]
): ConnectionView[] {
  const byId = new Map<string, ConnectionView>()

  for (const l of local) {
    if (byId.has(l.id)) {
      console.warn(`[connection-view] duplicate local connection id: ${l.id} — keeping first`)
      continue
    }
    byId.set(l.id, fromLocal(l))
  }

  for (const r of remote) {
    const existing = byId.get(r.id)
    if (!existing) {
      byId.set(r.id, fromRemote(r))
      continue
    }
    if (existing.source === 'remote') {
      // Duplicate remote id (shouldn't happen) — first wins.
      console.warn(`[connection-view] duplicate remote connection id: ${r.id} — keeping first`)
      continue
    }
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

  // Stable order: local order first, then remote-only.
  const order = [...local.map((l) => l.id), ...remote.map((r) => r.id)]
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
