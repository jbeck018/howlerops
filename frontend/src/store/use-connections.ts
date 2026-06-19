/**
 * useConnections — the canonical connection facade (Approach A).
 *
 * Composes the two underlying stores (local/operational + remote/sharing) into a
 * single unified, de-duped `ConnectionView[]` and re-exports both operational and
 * sharing actions. Implemented as a hook (NOT a third stateful store) so it always
 * reflects whatever the underlying stores hold and React re-renders when either
 * changes — avoiding cross-store subscription sync bugs.
 *
 * @module store/use-connections
 */

import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'

import { useConnectionStore } from './connection-store'
import { type ConnectionView,mergeConnections } from './connection-view'
import { useConnectionsStore } from './connections-store'

export function useConnections() {
  const localConns = useConnectionStore(useShallow((s) => s.connections))
  const remoteConns = useConnectionsStore(useShallow((s) => s.connections))

  const connections: ConnectionView[] = useMemo(
    () => mergeConnections(localConns, remoteConns),
    [localConns, remoteConns]
  )

  // Operational actions (delegate to the local store).
  const connect = useConnectionStore((s) => s.connectToDatabase)
  const disconnect = useConnectionStore((s) => s.disconnectFromDatabase)
  const addConnection = useConnectionStore((s) => s.addConnection)
  const removeLocal = useConnectionStore((s) => s.removeConnection)
  const switchDatabase = useConnectionStore((s) => s.switchDatabase)
  const fetchDatabases = useConnectionStore((s) => s.fetchDatabases)

  // Sharing actions (delegate to the remote store).
  const fetchRemote = useConnectionsStore((s) => s.fetchConnections)
  const fetchShared = useConnectionsStore((s) => s.fetchSharedConnections)
  const shareConnection = useConnectionsStore((s) => s.shareConnection)
  const unshareConnection = useConnectionsStore((s) => s.unshareConnection)
  const deleteRemote = useConnectionsStore((s) => s.deleteConnection)

  // Org-shared connections (a remote-only concept distinct from the merged list:
  // it can include connections shared by other org members).
  const sharedConnections = useConnectionsStore((s) => s.sharedConnections)

  // Remote loading/error surfaced for consumers that previously read them.
  const remoteLoading = useConnectionsStore((s) => s.loading)
  const remoteError = useConnectionsStore((s) => s.error)

  return {
    connections,
    sharedConnections,
    // operational
    connect,
    disconnect,
    addConnection,
    removeLocal,
    switchDatabase,
    fetchDatabases,
    // sharing
    fetchRemote,
    fetchShared,
    shareConnection,
    unshareConnection,
    deleteRemote,
    // remote status
    remoteLoading,
    remoteError,
  }
}

/**
 * Non-React snapshot of the merged connections, for the `getState()` call sites
 * (export/import, query-engine, schema-visualizer) that need sharing-aware data
 * outside a component render.
 */
export function getConnectionsSnapshot(): ConnectionView[] {
  return mergeConnections(
    useConnectionStore.getState().connections,
    useConnectionsStore.getState().connections
  )
}
