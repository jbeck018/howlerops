import { useCallback, useEffect, useRef, useState } from "react"

import { type SchemaNode } from "@/hooks/use-schema-introspection"
import { type ColumnLoader } from "@/lib/codemirror-sql"
import { waitForWails } from "@/lib/wails-runtime"
import { type DatabaseConnection } from "@/store/connection-store"

interface UseMultiDBSchemasParams {
  mode: 'single' | 'multi'
  connections: DatabaseConnection[]
  connectToDatabase: (connectionId: string) => Promise<void>
}

interface UseMultiDBSchemasResult {
  multiDBSchemas: Map<string, SchemaNode[]>
  multiDBSchemasRef: React.MutableRefObject<Map<string, SchemaNode[]>>
  columnCacheRef: React.MutableRefObject<Map<string, SchemaNode[]>>
  loadMultiDBSchemas: () => Promise<void>
  columnLoader: ColumnLoader
}

export function useMultiDBSchemas({
  mode,
  connections,
  connectToDatabase,
}: UseMultiDBSchemasParams): UseMultiDBSchemasResult {
  // Multi-DB state - schemas for all connections
  const [multiDBSchemas, setMultiDBSchemas] = useState<Map<string, SchemaNode[]>>(new Map())
  const multiDBSchemasRef = useRef<Map<string, SchemaNode[]>>(new Map())

  // Column cache for lazy loading (sessionId-schema-table -> columns)
  const columnCacheRef = useRef<Map<string, SchemaNode[]>>(new Map())

  useEffect(() => {
    multiDBSchemasRef.current = multiDBSchemas
  }, [multiDBSchemas])

  const loadMultiDBSchemas = useCallback(async () => {
    // For @ symbol autocomplete, we need ALL connected connections, not just filtered ones
    // The environment filter should only affect the UI, not the autocomplete functionality
    const relevantConnections = mode === 'multi' ? connections.filter(c => c.isConnected) : connections

    try {
      // Step 1: Ensure ALL filtered connections are connected (auto-connect)
      const disconnected = relevantConnections.filter(c => !c.isConnected)

      if (disconnected.length > 0) {
        await Promise.allSettled(
          disconnected.map(async (conn) => {
            await connectToDatabase(conn.id)
          })
        )

        // Wait a bit for state to update after connections
        await new Promise(resolve => setTimeout(resolve, 100))
      }

      // Step 2: Get session IDs for backend (backend uses sessionId as map key!)
      // Filter to only connected connections (from filtered set) that have sessionIds
      const connectedWithSessions = relevantConnections.filter(c => c.isConnected && c.sessionId)
      const sessionIds = connectedWithSessions.map(c => c.sessionId!)

      if (sessionIds.length === 0) {
        setMultiDBSchemas(new Map())
        return
      }

      // Step 3: Load schemas using GetMultiConnectionSchema (uses cache!)
      try {
        const { GetMultiConnectionSchema } = await import('../../../../bindings/github.com/jbeck018/howlerops/app')
        const combined = await GetMultiConnectionSchema(sessionIds)

          if (!combined || !combined.connections) {
            setMultiDBSchemas(new Map())
            return
          }

      // Convert to SchemaNode format and load columns for each table
      const schemasMap = new Map<string, SchemaNode[]>()

      // Process each connection (connId here is sessionId from backend)
      for (const [sessionId, rawConnSchema] of Object.entries(combined.connections || {})) {
        const schemaNodes: SchemaNode[] = []
        const connSchema = rawConnSchema as {
          schemas?: string[]
          tables?: Array<{ name: string; schema: string }>
        }

        // Find the connection by sessionId to get its name
        const connection = connectedWithSessions.find(c => c.sessionId === sessionId)

        const schemaNames = (connSchema.schemas as string[]) || []
        const tables = (connSchema.tables as Array<{ name: string; schema: string }>) || []

        // Process each schema
        for (const schemaName of schemaNames) {
          const schemaTables = tables.filter(t => t.schema === schemaName)

          // Skip migration table and internal postgres tables
          const nonMigrationTables = schemaTables.filter(t =>
            t.name !== 'schema_migrations' &&
            t.name !== 'goose_db_version' &&
            t.name !== '_prisma_migrations' &&
            !t.name.startsWith('__drizzle') &&
            !schemaName.startsWith('pg_temp') &&
            !schemaName.startsWith('pg_toast')
          )

          // Skip empty schemas (like pg_temp_*, pg_toast_*)
          if (nonMigrationTables.length === 0) {
            continue
          }

          // ✅ DON'T load columns upfront - too slow and hits localStorage quota!
          // Columns will be loaded lazily when user accesses a table in autocomplete
          const tablesWithColumns: SchemaNode[] = nonMigrationTables.map(table => ({
            id: `${sessionId}-${schemaName}-${table.name}`,
            name: table.name,
            type: 'table' as const,
            schema: table.schema,
            sessionId,  // Store for lazy loading
            children: []  // Empty initially, loaded on-demand
          }))

          schemaNodes.push({
            id: `${sessionId}-${schemaName}`,
            name: schemaName,
            type: 'schema' as const,
            children: tablesWithColumns
          })
        }

        // Store by connection ID (not sessionId!) and name for lookup
        if (connection) {
          const keys = new Set<string>([connection.id])

          if (connection.name) {
            keys.add(connection.name)

            const slug = connection.name.replace(/[^\w-]/g, '-')
            if (slug && slug !== connection.name) {
              keys.add(slug)
            }
          }

          keys.forEach(key => {
            schemasMap.set(key, schemaNodes)
          })

          // ✅ UPDATE BOTH STATE AND REF! Don't wait for useEffect - update ref immediately
          const newMap = new Map(schemasMap)
          setMultiDBSchemas(newMap)
          multiDBSchemasRef.current = newMap  // Direct ref update for immediate availability!
        }
      }

      // Final update with complete schema
      setMultiDBSchemas(schemasMap)
      multiDBSchemasRef.current = schemasMap  // Final ref sync
      } catch {
      setMultiDBSchemas(new Map())
      return
      }
    } catch {
      // Set empty map on error so autocomplete still works (without multi-DB)
      setMultiDBSchemas(new Map())
    }
  }, [mode, connections, connectToDatabase])

  // Load schemas for all connections when in multi-DB mode
  useEffect(() => {
    if (mode !== 'multi') {
      return
    }

    const connectedConnections = connections.filter(c => c.isConnected)
    if (connectedConnections.length === 0) {
      const emptyMap = new Map<string, SchemaNode[]>()
      setMultiDBSchemas(emptyMap)
      multiDBSchemasRef.current = emptyMap
      return
    }

    loadMultiDBSchemas()
  }, [mode, connections, loadMultiDBSchemas])

  const columnLoader: ColumnLoader = useCallback(async (sessionId: string, schema: string, tableName: string) => {
    try {
      // Wait for Wails runtime to be ready
      const isReady = await waitForWails(2000)

      if (!isReady) {
        console.warn('[ColumnLoader] Wails runtime not ready')
        return []
      }

      const { GetTableStructure } = await import('../../../../bindings/github.com/jbeck018/howlerops/app')
      const structure = await GetTableStructure(sessionId, schema, tableName)

      if (!structure || !structure.columns || structure.columns.length === 0) {
        return []
      }

      // Convert to Column format
      return structure.columns.map((col: { name: string; data_type?: string; nullable?: boolean; primary_key?: boolean }) => ({
        name: col.name,
        dataType: col.data_type || 'unknown',
        nullable: col.nullable,
        primaryKey: col.primary_key
      }))
    } catch (error) {
      console.error('[ColumnLoader] Failed to load columns:', {
        sessionId,
        schema,
        tableName,
        error: error instanceof Error ? error.message : String(error)
      })
      return []
    }
  }, [])

  return {
    multiDBSchemas,
    multiDBSchemasRef,
    columnCacheRef,
    loadMultiDBSchemas,
    columnLoader,
  }
}
