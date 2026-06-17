import { useCallback, useEffect, useState } from "react"

import { toast } from "@/hooks/use-toast"
import { type DatabaseConnection, useConnectionStore } from "@/store/connection-store"

interface UseConnectionDatabasesParams {
  mode: 'single' | 'multi'
  connectionMap: Map<string, DatabaseConnection>
  activeTabConnectionId: string | undefined
}

interface UseConnectionDatabasesResult {
  connectionDatabases: Record<string, string[]>
  connectionDbLoading: Record<string, boolean>
  connectionDbSwitching: Record<string, boolean>
  ensureConnectionDatabases: (connectionId: string) => Promise<void>
  handleConnectionDatabaseChange: (connectionId: string, database: string) => Promise<void>
}

export function useConnectionDatabases({
  mode,
  connectionMap,
  activeTabConnectionId,
}: UseConnectionDatabasesParams): UseConnectionDatabasesResult {
  const [connectionDatabases, setConnectionDatabases] = useState<Record<string, string[]>>({})
  const [connectionDbLoading, setConnectionDbLoading] = useState<Record<string, boolean>>({})
  const [connectionDbSwitching, setConnectionDbSwitching] = useState<Record<string, boolean>>({})

  const ensureConnectionDatabases = useCallback(async (connectionId: string) => {
    if (!connectionId) {
      return
    }
    if (connectionDatabases[connectionId] || connectionDbLoading[connectionId]) {
      return
    }

    const connection = connectionMap.get(connectionId)
    if (!connection || !connection.sessionId || !connection.isConnected) {
      return
    }

    setConnectionDbLoading((prev) => ({ ...prev, [connectionId]: true }))
    try {
      const dbs = await useConnectionStore.getState().fetchDatabases(connectionId)
      setConnectionDatabases((prev) => ({ ...prev, [connectionId]: dbs }))
    } catch (error) {
      toast({
        title: 'Unable to load databases',
        description: error instanceof Error ? error.message : 'Unknown error occurred',
        variant: 'destructive',
      })
    } finally {
      setConnectionDbLoading((prev) => ({ ...prev, [connectionId]: false }))
    }
  }, [connectionDatabases, connectionDbLoading, connectionMap])

  const handleConnectionDatabaseChange = useCallback(async (connectionId: string, database: string) => {
    if (!connectionId || !database) {
      return
    }
    const connection = connectionMap.get(connectionId)
    if (!connection || connection.database === database) {
      return
    }

    setConnectionDbSwitching((prev) => ({ ...prev, [connectionId]: true }))
    try {
      await useConnectionStore.getState().switchDatabase(connectionId, database)
      toast({
        title: 'Database switched',
        description: `${connection.name || 'Connection'} is now using ${database}.`,
      })
    } catch (error) {
      toast({
        title: 'Failed to switch database',
        description: error instanceof Error ? error.message : 'Unable to switch database',
        variant: 'destructive',
      })
    } finally {
      setConnectionDbSwitching((prev) => ({ ...prev, [connectionId]: false }))
    }
  }, [connectionMap])

  useEffect(() => {
    if (mode !== 'single') {
      return
    }
    if (!activeTabConnectionId) {
      return
    }
    void ensureConnectionDatabases(activeTabConnectionId)
  }, [activeTabConnectionId, ensureConnectionDatabases, mode])

  return {
    connectionDatabases,
    connectionDbLoading,
    connectionDbSwitching,
    ensureConnectionDatabases,
    handleConnectionDatabaseChange,
  }
}
