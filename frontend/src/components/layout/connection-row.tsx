import { ChevronDown, Loader2, Network, Plus, Table } from "lucide-react"
import { memo, useCallback, useEffect, useRef, useState } from "react"

import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { toast } from "@/hooks/use-toast"
import { preloadComponent } from "@/lib/component-preload"
import { cn } from "@/lib/utils"
import { type DatabaseConnection, useConnectionStore } from "@/store/connection-store"

const preloadSchemaVisualizer = () =>
  import("@/components/schema-visualizer/schema-visualizer").then(m => ({
    default: m.SchemaVisualizerWrapper as React.ComponentType<unknown>,
  }))

interface DbState {
  options: string[]
  loading: boolean
  switching: boolean
  error?: string
}

const INITIAL_DB_STATE: DbState = { options: [], loading: false, switching: false }

export interface ConnectionRowProps {
  connection: DatabaseConnection
  isActive: boolean
  isPending: boolean
  isConnecting: boolean
  isInActiveTab: boolean
  hasActiveTab: boolean
  onSelect: (connection: DatabaseConnection) => void
  onAddToQueryTab: (connectionId: string) => void
  onViewSchema: (connectionId: string) => void
  onViewDiagram: (connectionId: string) => void
}

/**
 * A single connection entry: activation button, hover actions, and the database
 * selector. Memoized and self-contained — hover, accordion, and database state
 * live here so interacting with one row never re-renders its siblings.
 */
function ConnectionRowImpl({
  connection,
  isActive,
  isPending,
  isConnecting,
  isInActiveTab,
  hasActiveTab,
  onSelect,
  onAddToQueryTab,
  onViewSchema,
  onViewDiagram,
}: ConnectionRowProps) {
  const fetchDatabases = useConnectionStore((state) => state.fetchDatabases)
  const switchDatabase = useConnectionStore((state) => state.switchDatabase)

  const [hovered, setHovered] = useState(false)
  const [accordionOpen, setAccordionOpen] = useState(isActive)
  const [dbState, setDbState] = useState<DbState>(INITIAL_DB_STATE)
  const lastErrorToast = useRef<string | undefined>(undefined)

  const connectionId = connection.id
  const { isConnected } = connection

  const loadDatabases = useCallback(async () => {
    setDbState((prev) => (prev.loading ? prev : { ...prev, loading: true, error: undefined }))
    try {
      const dbs = await fetchDatabases(connectionId)
      setDbState((prev) => ({
        ...prev,
        options: dbs,
        loading: false,
        error: dbs.length === 0 ? 'No databases available' : undefined,
      }))
      lastErrorToast.current = undefined
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load databases'
      setDbState((prev) => ({ ...prev, loading: false, error: message }))
      if (lastErrorToast.current !== message) {
        toast({ title: 'Unable to load databases', description: message, variant: 'destructive' })
        lastErrorToast.current = message
      }
    }
  }, [connectionId, fetchDatabases])

  // Auto-load databases once the connection is active and connected.
  useEffect(() => {
    if (isActive && isConnected) {
      void loadDatabases()
    }
  }, [isActive, isConnected, loadDatabases])

  const handleDatabaseSelect = useCallback(
    async (database: string) => {
      if (!database || database === connection.database) {
        return
      }
      setDbState((prev) => ({ ...prev, switching: true }))
      try {
        await switchDatabase(connectionId, database)
        toast({ title: 'Database switched', description: `${connection.name} is now using ${database}.` })
      } catch (error) {
        toast({
          title: 'Failed to switch database',
          description: error instanceof Error ? error.message : 'Unable to switch database',
          variant: 'destructive',
        })
      } finally {
        setDbState((prev) => ({ ...prev, switching: false }))
      }
    },
    [connectionId, connection.database, connection.name, switchDatabase]
  )

  const selectedDatabase =
    connection.database && dbState.options.includes(connection.database)
      ? connection.database
      : undefined

  return (
    <Collapsible open={accordionOpen} onOpenChange={setAccordionOpen} className="space-y-1">
      <div
        className="flex items-center gap-1 group"
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
      >
        <Button
          variant={isActive || isPending ? "secondary" : "ghost"}
          size="sm"
          className="h-7 flex-1 justify-start overflow-hidden text-xs"
          disabled={isConnecting}
          onClick={() => onSelect(connection)}
        >
          <span className="truncate flex-1 text-left">{connection.name}</span>
          <span className="ml-1 inline-flex items-center flex-shrink-0">
            {isPending ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : isConnected ? (
              <span className="h-1.5 w-1.5 rounded-full bg-green-500" />
            ) : null}
          </span>
        </Button>

        {isConnected && (
          <>
            {hovered && (
              <div className="flex items-center">
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0"
                  onClick={() => onViewSchema(connectionId)}
                  title="View Tables"
                >
                  <Table className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0"
                  onClick={() => onViewDiagram(connectionId)}
                  onMouseEnter={() => void preloadComponent(preloadSchemaVisualizer)}
                  title="View Schema Diagram"
                >
                  <Network className="h-3 w-3" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 w-5 p-0"
                  onClick={() => onAddToQueryTab(connectionId)}
                  disabled={!hasActiveTab || isInActiveTab}
                  title={!hasActiveTab ? "No active query tab" : isInActiveTab ? "Already in query tab" : "Add to Query Tab"}
                >
                  <Plus className="h-3 w-3" />
                </Button>
              </div>
            )}
            <CollapsibleTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0"
                title={accordionOpen ? "Hide database selector" : "Show database selector"}
              >
                <ChevronDown className={cn("h-3 w-3 transition-transform", accordionOpen && "rotate-180")} />
              </Button>
            </CollapsibleTrigger>
          </>
        )}
      </div>

      {isConnected && (
        <CollapsibleContent className="pl-4 pr-1">
          {dbState.options.length > 0 ? (
            <Select
              value={selectedDatabase}
              onValueChange={(value) => void handleDatabaseSelect(value)}
              disabled={dbState.switching}
            >
              <SelectTrigger className="h-7 text-xs justify-between">
                <SelectValue placeholder="Select database" />
              </SelectTrigger>
              <SelectContent>
                {dbState.options.map((db) => (
                  <SelectItem key={db} value={db}>
                    {db}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs"
              onClick={() => void loadDatabases()}
              disabled={dbState.loading}
            >
              {dbState.loading ? (
                <>
                  <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                  Loading...
                </>
              ) : (
                'Load databases'
              )}
            </Button>
          )}
          {dbState.error && <p className="text-[10px] text-destructive mt-1">{dbState.error}</p>}
        </CollapsibleContent>
      )}
    </Collapsible>
  )
}

export const ConnectionRow = memo(ConnectionRowImpl)
