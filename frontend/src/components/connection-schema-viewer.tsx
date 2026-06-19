import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Database,
  Folder,
  FolderOpen,
  Loader2,
  Network,
  RefreshCw,
  Table,
  X,
} from "lucide-react"
import { useCallback, useEffect, useMemo, useState } from "react"
import { createPortal } from "react-dom"

import { SchemaTree } from "@/components/layout/sidebar"
import { SchemaVisualizerWrapper } from "@/components/schema-visualizer/schema-visualizer"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { useConnectionStore } from "@/store/connection-store"

interface ConnectionSchemaViewerProps {
  connectionId: string | null
  onClose: () => void
}

interface SchemaNode {
  id: string
  name: string
  type: 'database' | 'schema' | 'table' | 'column'
  schema?: string
  children?: SchemaNode[]
  expanded?: boolean
}

// Build schema -> table tree nodes for one database from its tables, dropping
// migration/system noise and sorting. Ids are namespaced by db to stay unique.
function buildSchemaNodes(
  connectionId: string,
  dbName: string,
  tables: Array<{ name?: string; schema?: string }>
): SchemaNode[] {
  const bySchema = new Map<string, string[]>()
  for (const t of tables) {
    const name = t.name || ''
    const schemaName = t.schema || 'public'
    if (!name) continue
    if (
      name === 'schema_migrations' ||
      name === 'goose_db_version' ||
      name === '_prisma_migrations' ||
      name.startsWith('__drizzle') ||
      schemaName.startsWith('pg_temp') ||
      schemaName.startsWith('pg_toast')
    ) {
      continue
    }
    const arr = bySchema.get(schemaName) ?? []
    arr.push(name)
    bySchema.set(schemaName, arr)
  }

  return [...bySchema.entries()]
    .sort((a, b) => a[0].localeCompare(b[0]))
    .map(([schemaName, tableNames]) => ({
      id: `${connectionId}-${dbName}-${schemaName}`,
      name: schemaName,
      type: 'schema' as const,
      children: tableNames
        .sort((a, b) => a.localeCompare(b))
        .map((name) => ({
          id: `${connectionId}-${dbName}-${schemaName}-${name}`,
          name,
          type: 'table' as const,
          schema: schemaName,
          children: [],
        })),
    }))
}

export function ConnectionSchemaViewer({ connectionId, onClose }: ConnectionSchemaViewerProps) {
  const { connections } = useConnectionStore()
  const connection = connections.find(conn => conn.id === connectionId)
  // Backend DB calls are keyed by the live session id (set when the connection
  // is activated), not the persisted connection id. Resolve it here so the
  // schema lookups match the manager — passing connectionId yields
  // "connection not found". Node ids stay keyed by connectionId for stable keys.
  const sessionId = connection?.sessionId ?? null

  const [databases, setDatabases] = useState<string[]>([])
  const [loadingDatabases, setLoadingDatabases] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showVisualizer, setShowVisualizer] = useState(false)

  // Per-database lazy state: schema nodes / loading / error, keyed by db name.
  const [dbSchemas, setDbSchemas] = useState<Record<string, SchemaNode[]>>({})
  const [dbLoading, setDbLoading] = useState<Record<string, boolean>>({})
  const [dbError, setDbError] = useState<Record<string, string>>({})
  const [openDatabases, setOpenDatabases] = useState<Set<string>>(new Set())

  // Lazily load one database's schema (cached after first expand).
  const loadDatabaseSchema = useCallback(async (dbName: string) => {
    if (!connectionId || !sessionId) return
    setDbLoading(prev => ({ ...prev, [dbName]: true }))
    setDbError(prev => ({ ...prev, [dbName]: '' }))
    try {
      const { GetDatabaseSchema } = await import('../../bindings/github.com/jbeck018/howlerops/app')
      const res = await GetDatabaseSchema(sessionId, dbName)
      const tables = (res?.tables ?? []) as Array<{ name?: string; schema?: string }>
      setDbSchemas(prev => ({ ...prev, [dbName]: buildSchemaNodes(connectionId, dbName, tables) }))
    } catch (err) {
      setDbError(prev => ({ ...prev, [dbName]: err instanceof Error ? err.message : 'Failed to load schema' }))
    } finally {
      setDbLoading(prev => ({ ...prev, [dbName]: false }))
    }
  }, [connectionId, sessionId])

  const loadDatabases = useCallback(async () => {
    if (!connectionId || !sessionId) {
      setDatabases([])
      return
    }
    setLoadingDatabases(true)
    setError(null)
    setDbSchemas({})
    setDbError({})
    try {
      const { ListConnectionDatabases } = await import('../../bindings/github.com/jbeck018/howlerops/app')
      const res = await ListConnectionDatabases(sessionId)
      if (res && res.success === false) {
        throw new Error(res.message || 'Failed to list databases')
      }
      const list = (res?.databases ?? []) as string[]
      // Fall back to the connection's configured database if none were returned.
      const configuredDb = connection?.database
      const names = list.length > 0 ? list : (configuredDb ? [configuredDb] : [])
      setDatabases(names)

      // Auto-open and load the connection's current database (or the first).
      const initial = configuredDb && names.includes(configuredDb) ? configuredDb : names[0]
      if (initial) {
        setOpenDatabases(new Set([initial]))
        void loadDatabaseSchema(initial)
      } else {
        setOpenDatabases(new Set())
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to list databases')
    } finally {
      setLoadingDatabases(false)
    }
  }, [connectionId, sessionId, connection?.database, loadDatabaseSchema])

  const toggleDatabase = useCallback((dbName: string) => {
    setOpenDatabases(prev => {
      const next = new Set(prev)
      if (next.has(dbName)) {
        next.delete(dbName)
      } else {
        next.add(dbName)
        if (!dbSchemas[dbName] && !dbLoading[dbName]) {
          void loadDatabaseSchema(dbName)
        }
      }
      return next
    })
  }, [dbSchemas, dbLoading, loadDatabaseSchema])

  useEffect(() => {
    if (connectionId) {
      loadDatabases()
    }
  }, [connectionId, loadDatabases])

  // Aggregate of everything loaded so far (for the visualizer).
  const allLoadedNodes = useMemo(() => Object.values(dbSchemas).flat(), [dbSchemas])

  if (!connectionId || !connection) {
    return null
  }

  return createPortal(
    <div className="fixed inset-0 z-50 bg-black/50 flex items-center justify-center p-4">
      <Card className="w-full max-w-4xl h-[80vh] flex flex-col">
        <CardHeader className="flex flex-row items-center justify-between shrink-0 pb-2">
          <div className="flex items-center gap-2">
            <Database className="h-5 w-5" />
            <CardTitle className="text-lg">
              Schema Explorer - {connection.name}
            </CardTitle>
            <Badge variant="outline" className="text-xs">
              {connection.type}
            </Badge>
          </div>
          <div className="flex items-center gap-2">
            {allLoadedNodes.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setShowVisualizer(true)}
                className="h-8 px-2"
                title="Schema Visualizer"
              >
                <Network className="h-4 w-4" />
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={loadDatabases}
              disabled={loadingDatabases}
              className="h-8 px-2"
              title="Refresh"
            >
              <RefreshCw className={cn("h-4 w-4", loadingDatabases && "animate-spin")} />
            </Button>
            <Button variant="ghost" size="sm" onClick={onClose} className="h-8 w-8 p-0">
              <X className="h-4 w-4" />
            </Button>
          </div>
        </CardHeader>

        <CardContent className="flex-1 overflow-hidden pt-0">
          <ScrollArea className="h-full">
            {error ? (
              <div className="flex items-center justify-center h-32 text-destructive">
                <div className="flex items-center gap-2">
                  <AlertCircle className="h-4 w-4" />
                  <span>{error}</span>
                </div>
              </div>
            ) : loadingDatabases && databases.length === 0 ? (
              <div className="flex items-center justify-center h-32 text-muted-foreground">
                <div className="flex items-center gap-2">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Loading databases...</span>
                </div>
              </div>
            ) : databases.length === 0 ? (
              <div className="flex items-center justify-center h-32 text-muted-foreground">
                <div className="flex items-center gap-2">
                  <Table className="h-4 w-4" />
                  <span>No databases found</span>
                </div>
              </div>
            ) : (
              <div className="space-y-1">
                {databases.map((dbName) => {
                  const isOpen = openDatabases.has(dbName)
                  const nodes = dbSchemas[dbName]
                  const isLoading = dbLoading[dbName]
                  const dbErr = dbError[dbName]
                  return (
                    <div key={dbName}>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="w-full justify-start h-8 px-2"
                        onClick={() => toggleDatabase(dbName)}
                      >
                        <div className="mr-1">
                          {isOpen ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
                        </div>
                        <div className="mr-2">
                          {isOpen ? <FolderOpen className="h-4 w-4" /> : <Folder className="h-4 w-4" />}
                        </div>
                        <span className="text-sm truncate">{dbName}</span>
                        {connection.database === dbName && (
                          <Badge variant="secondary" className="ml-2 text-[10px]">current</Badge>
                        )}
                      </Button>

                      {isOpen && (
                        <div className="pl-4">
                          {isLoading ? (
                            <div className="flex items-center gap-2 px-2 py-2 text-xs text-muted-foreground">
                              <Loader2 className="h-3 w-3 animate-spin" />
                              <span>Loading schema...</span>
                            </div>
                          ) : dbErr ? (
                            <div className="flex items-center gap-2 px-2 py-2 text-xs text-destructive">
                              <AlertCircle className="h-3 w-3" />
                              <span>{dbErr}</span>
                            </div>
                          ) : nodes && nodes.length > 0 ? (
                            <SchemaTree key={`${connectionId}-${dbName}`} nodes={nodes} />
                          ) : (
                            <p className="px-2 py-2 text-xs text-muted-foreground">No tables in this database.</p>
                          )}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </ScrollArea>
        </CardContent>
      </Card>

      {/* Schema Visualizer Modal */}
      {showVisualizer && (
        <SchemaVisualizerWrapper
          schema={allLoadedNodes}
          connectionId={connectionId}
          onClose={() => setShowVisualizer(false)}
        />
      )}
    </div>,
    document.body
  )
}
