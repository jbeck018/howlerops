import { describe, expect, it } from 'vitest'

import type { Connection } from '@/lib/api/connections'

import type { DatabaseConnection } from './connection-store'
import { fromLocal, fromRemote, mergeConnections } from './connection-view'

const localConn = (overrides: Partial<DatabaseConnection> = {}): DatabaseConnection => ({
  id: 'local-1',
  name: 'Local DB',
  type: 'postgresql',
  host: 'localhost',
  port: 5432,
  database: 'app',
  username: 'admin',
  isConnected: false,
  ...overrides,
})

const remoteConn = (overrides: Partial<Connection> = {}): Connection => ({
  id: 'remote-1',
  user_id: 'user-1',
  organization_id: 'org-1',
  name: 'Shared DB',
  description: 'team db',
  database_type: 'postgres',
  host: 'db.example.com',
  port: 5432,
  database_name: 'shared',
  username: 'reader',
  ssl_enabled: true,
  visibility: 'shared',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-02T00:00:00Z',
  created_by_email: 'owner@example.com',
  last_used: '2024-03-01T00:00:00Z',
  ...overrides,
})

describe('fromLocal', () => {
  it('maps operational fields and marks local provenance', () => {
    const v = fromLocal(
      localConn({ sessionId: 'sess-9', isConnected: true, environments: ['dev'], useTunnel: true })
    )
    expect(v).toMatchObject({
      id: 'local-1',
      database: 'app',
      sessionId: 'sess-9',
      isConnected: true,
      hasLocalCredentials: true,
      environments: ['dev'],
      useTunnel: true,
      source: 'local',
      canConnect: true,
    })
  })
})

describe('fromRemote', () => {
  it('maps snake_case server fields, database_name->database, and parses last_used', () => {
    const v = fromRemote(remoteConn())
    expect(v).toMatchObject({
      id: 'remote-1',
      type: 'postgresql', // database_type 'postgres' normalized
      database: 'shared',
      username: 'reader',
      organizationId: 'org-1',
      visibility: 'shared',
      sharedByEmail: 'owner@example.com',
      description: 'team db',
      hasLocalCredentials: false,
      isConnected: false,
      source: 'remote',
      canConnect: false,
    })
    expect(v.lastUsed).toBeInstanceOf(Date)
  })

  it('falls through unknown database_type to the raw string', () => {
    const v = fromRemote(remoteConn({ database_type: 'cockroachdb' }))
    expect(v.type).toBe('cockroachdb')
  })

  it('handles absent last_used', () => {
    const v = fromRemote(remoteConn({ last_used: undefined }))
    expect(v.lastUsed).toBeUndefined()
  })
})

describe('mergeConnections', () => {
  it('id only local -> source local, canConnect true', () => {
    const out = mergeConnections([localConn({ id: 'a' })], [])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({ id: 'a', source: 'local', canConnect: true })
  })

  it('id only remote -> source remote, canConnect false, no local creds', () => {
    const out = mergeConnections([], [remoteConn({ id: 'b' })])
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({
      id: 'b',
      source: 'remote',
      canConnect: false,
      hasLocalCredentials: false,
    })
  })

  it('id in both -> operational from local, sharing from remote, source both', () => {
    const out = mergeConnections(
      [localConn({ id: 'x', sessionId: 'sess-1', isConnected: true })],
      [remoteConn({ id: 'x', visibility: 'shared', organization_id: 'org-9' })]
    )
    expect(out).toHaveLength(1)
    expect(out[0]).toMatchObject({
      id: 'x',
      source: 'both',
      // operational (local)
      sessionId: 'sess-1',
      isConnected: true,
      hasLocalCredentials: true,
      canConnect: true,
      // sharing (remote)
      visibility: 'shared',
      organizationId: 'org-9',
    })
  })

  it('empty remote -> result equals local mapped 1:1', () => {
    const locals = [localConn({ id: 'a' }), localConn({ id: 'b', name: 'B' })]
    const out = mergeConnections(locals, [])
    expect(out.map((c) => c.id)).toEqual(['a', 'b'])
    expect(out.every((c) => c.source === 'local')).toBe(true)
  })

  it('duplicate ids within local -> first wins', () => {
    const out = mergeConnections(
      [localConn({ id: 'dup', name: 'First' }), localConn({ id: 'dup', name: 'Second' })],
      []
    )
    expect(out).toHaveLength(1)
    expect(out[0].name).toBe('First')
  })

  it('preserves stable order: local first, then remote-only', () => {
    const locals = [localConn({ id: 'l1' }), localConn({ id: 'l2' })]
    const remotes = [
      remoteConn({ id: 'l2' }), // overlaps local -> stays in local position
      remoteConn({ id: 'r1' }), // remote-only -> appended
    ]
    const out = mergeConnections(locals, remotes)
    expect(out.map((c) => c.id)).toEqual(['l1', 'l2', 'r1'])
    expect(out.find((c) => c.id === 'l2')?.source).toBe('both')
    expect(out.find((c) => c.id === 'r1')?.source).toBe('remote')
  })
})
