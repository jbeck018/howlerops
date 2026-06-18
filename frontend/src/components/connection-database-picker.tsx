import { Check, ChevronsUpDown, Database, Network, Search } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { useShallow } from "zustand/react/shallow"

import { UNASSIGNED_ENVIRONMENT_LABEL } from "@/components/connection-manager/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { toast } from "@/hooks/use-toast"
import { groupConnectionsByEnvironment } from "@/lib/group-connections-by-environment"
import { cn } from "@/lib/utils"
import { type DatabaseConnection, useConnectionStore } from "@/store/connection-store"
import { useActiveTab, useQueryEditorActions } from "@/store/query-editor-store"

// A single federated query can reference at most this many connections (matches
// the backend MaxConcurrentConns). Guard the selection so users hit a clear
// message here instead of a cryptic error at execution time.
const MAX_FEDERATION_CONNECTIONS = 10

/**
 * Unified per-tab connection / database picker for the query editor toolbar.
 * One control to: switch the active connection, switch its database, or flip to
 * multi-DB and choose which connections participate (the @conn.table set).
 * Reads the active tab + connection store directly and drives the per-tab store
 * actions, so it stays self-contained.
 */
export function ConnectionDatabasePicker() {
  const activeTab = useActiveTab()
  const { setTabConnection, setTabDatabase, setTabSelectedConnections, setTabMode } = useQueryEditorActions()

  const {
    connections,
    setActiveConnection,
    connectToDatabase,
    fetchDatabases,
    getFilteredConnections,
    availableEnvironments,
    // Subscribed so the picker re-renders when the env filter changes.
    activeEnvironmentFilter: _envFilter,
  } = useConnectionStore(
    useShallow((s) => ({
      connections: s.connections,
      setActiveConnection: s.setActiveConnection,
      connectToDatabase: s.connectToDatabase,
      fetchDatabases: s.fetchDatabases,
      getFilteredConnections: s.getFilteredConnections,
      availableEnvironments: s.availableEnvironments,
      activeEnvironmentFilter: s.activeEnvironmentFilter,
    }))
  )

  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState("")
  const [databases, setDatabases] = useState<string[]>([])
  const [dbLoading, setDbLoading] = useState(false)

  const envConnections = getFilteredConnections()
  // Default mode and the "Multi-DB" toggle both key off the environment-filtered
  // set so they stay consistent (otherwise the trigger can show "Multi-DB" while
  // the toggle is disabled because only one connection is in the environment).
  const mode = activeTab?.mode ?? (envConnections.length > 1 ? "multi" : "single")
  const canMulti = envConnections.length >= 2
  const tabConn = connections.find((c) => c.id === activeTab?.connectionId)
  // The database the tab targets: the per-tab override if set, else the
  // connection's globally-active database.
  const activeDatabase = activeTab?.database || tabConn?.database
  const selectedIds = activeTab?.selectedConnectionIds ?? []
  const tabConnId = tabConn?.id
  const tabConnConnected = tabConn?.isConnected

  // Reset the filter each time the popover opens so it doesn't carry over.
  useEffect(() => {
    if (!open) setFilter("")
  }, [open])

  // Lazily load databases for the active connection while the popover is open.
  useEffect(() => {
    if (!open || mode !== "single" || !tabConnId || !tabConnConnected) return
    let cancelled = false
    setDbLoading(true)
    fetchDatabases(tabConnId)
      .then((dbs) => { if (!cancelled) setDatabases(dbs) })
      .catch(() => { if (!cancelled) setDatabases([]) })
      .finally(() => { if (!cancelled) setDbLoading(false) })
    return () => { cancelled = true }
  }, [open, mode, tabConnId, tabConnConnected, fetchDatabases])

  // The source list for the active mode: all connections in single mode, the
  // environment-filtered set in multi mode. Deduplicated by id (the store can
  // momentarily hold repeat entries) and filtered by the search box, then
  // grouped into environment folders like the sidebar.
  const sourceConnections = mode === "multi" ? envConnections : connections
  const groups = useMemo(() => {
    const byId = new Map<string, DatabaseConnection>()
    for (const c of sourceConnections) {
      if (!byId.has(c.id)) byId.set(c.id, c)
    }
    let list = Array.from(byId.values())

    const q = filter.trim().toLowerCase()
    if (q) {
      list = list.filter((c) =>
        c.name.toLowerCase().includes(q) ||
        c.type.toLowerCase().includes(q) ||
        (c.database ?? "").toLowerCase().includes(q)
      )
    }

    return groupConnectionsByEnvironment(list, availableEnvironments, UNASSIGNED_ENVIRONMENT_LABEL)
  }, [sourceConnections, filter, availableEnvironments])

  // Only show folder headers when environments actually exist; otherwise a flat
  // list reads cleaner.
  const showFolders = availableEnvironments.length > 0
  const hasMatches = groups.some((g) => g.connections.length > 0)

  if (!activeTab) return null

  const handleSelectConnection = async (conn: DatabaseConnection) => {
    setTabConnection(activeTab.id, conn.id)
    setActiveConnection(conn) // keep autocomplete/global active in sync
    if (!conn.isConnected) {
      try { await connectToDatabase(conn.id) } catch { /* surfaced elsewhere */ }
    }
  }

  // Per-tab database targeting: set the database on the tab only, WITHOUT
  // globally switching the connection's active database. This lets two tabs on
  // the same connection target different databases. Execution threads
  // tab.database into the query request.
  const handleSwitchDatabase = (db: string) => {
    if (!tabConn || !activeTab) return
    setTabDatabase(activeTab.id, db)
    setOpen(false)
  }

  const toggleMulti = (conn: DatabaseConnection) => {
    if (selectedIds.includes(conn.id)) {
      if (selectedIds.length <= 1) return // keep at least one
      setTabSelectedConnections(activeTab.id, selectedIds.filter((id) => id !== conn.id))
    } else {
      if (selectedIds.length >= MAX_FEDERATION_CONNECTIONS) {
        toast({
          title: "Connection limit reached",
          description: `A single query can use at most ${MAX_FEDERATION_CONNECTIONS} connections. Deselect one to add another.`,
          variant: "destructive",
        })
        return
      }
      setTabSelectedConnections(activeTab.id, [...selectedIds, conn.id])
      if (!conn.isConnected) void connectToDatabase(conn.id)
    }
  }

  const triggerLabel =
    mode === "multi"
      ? `Multi-DB · ${(selectedIds.length || envConnections.length)} DBs`
      : tabConn
        ? `${tabConn.name}${activeDatabase ? ` · ${activeDatabase}` : ""}`
        : "Select database"

  const connectedCount = envConnections.filter((c) => c.isConnected).length

  const renderConnectionRow = (conn: DatabaseConnection) => {
    if (mode === "multi") {
      const checked = selectedIds.includes(conn.id)
      return (
        <button
          key={conn.id}
          type="button"
          className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-muted"
          onClick={() => toggleMulti(conn)}
        >
          <span className={cn(
            "flex h-3.5 w-3.5 flex-shrink-0 items-center justify-center rounded-sm border",
            checked ? "border-primary bg-primary text-primary-foreground" : "border-muted-foreground/40"
          )}>
            {checked && <Check className="h-2.5 w-2.5" />}
          </span>
          <span className={cn("h-1.5 w-1.5 flex-shrink-0 rounded-full", conn.isConnected ? "bg-green-500" : "bg-muted-foreground/40")} />
          <span className="flex-1 truncate">{conn.name}</span>
          <Badge variant="secondary" className="text-[9px]">{conn.type}</Badge>
        </button>
      )
    }

    return (
      <button
        key={conn.id}
        type="button"
        className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-muted"
        onClick={() => void handleSelectConnection(conn)}
      >
        <span className={cn("h-1.5 w-1.5 flex-shrink-0 rounded-full", conn.isConnected ? "bg-green-500" : "bg-muted-foreground/40")} />
        <span className="flex-1 truncate">{conn.name}</span>
        <Badge variant="secondary" className="text-[9px]">{conn.type}</Badge>
        {activeTab.connectionId === conn.id && <Check className="h-3 w-3 flex-shrink-0" />}
      </button>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="h-8 max-w-[280px] gap-2" title="Connection & database">
          {mode === "multi" ? <Network className="h-4 w-4 flex-shrink-0" /> : <Database className="h-4 w-4 flex-shrink-0" />}
          <span className="truncate">{triggerLabel}</span>
          <ChevronsUpDown className="h-3.5 w-3.5 flex-shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80 p-0">
        {/* Bounded flex column: toggle + filter pinned, list scrolls, footer pinned. */}
        <div className="flex max-h-[min(70vh,32rem)] flex-col">
          {/* Mode toggle */}
          <div className="flex gap-1 border-b p-2">
            <Button
              variant={mode === "single" ? "default" : "ghost"}
              size="sm"
              className="h-7 flex-1 text-xs"
              onClick={() => setTabMode(activeTab.id, "single")}
            >
              <Database className="mr-1 h-3.5 w-3.5" />
              Single
            </Button>
            <Button
              variant={mode === "multi" ? "default" : "ghost"}
              size="sm"
              className="h-7 flex-1 text-xs"
              disabled={!canMulti}
              title={canMulti ? undefined : "Needs at least 2 connections in the same environment"}
              onClick={() => canMulti && setTabMode(activeTab.id, "multi")}
            >
              <Network className="mr-1 h-3.5 w-3.5" />
              Multi-DB
            </Button>
          </div>

          {/* Filter */}
          <div className="border-b p-2">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="Filter connections…"
                className="h-7 pl-7 text-xs"
                autoFocus
              />
            </div>
          </div>

          <ScrollArea className="flex-1">
            <div className="px-1 py-1">
              <div className="px-2 pb-1 pt-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                {mode === "multi" ? "Connections in query" : "Connection"}
              </div>

              {!hasMatches ? (
                <p className="px-2 py-2 text-xs text-muted-foreground">
                  {sourceConnections.length === 0 ? "No connections." : "No matching connections."}
                </p>
              ) : (
                groups.map((group) =>
                  group.connections.length === 0 ? null : (
                    <div key={group.key} className="pb-1">
                      {showFolders && (
                        <div className="flex items-center justify-between px-2 pb-0.5 pt-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground/80">
                          <span className="truncate">{group.label}</span>
                          <span className="ml-1 font-normal">{group.connections.length}</span>
                        </div>
                      )}
                      {group.connections.map(renderConnectionRow)}
                    </div>
                  )
                )
              )}

              {/* Single-mode database list for the active connection. */}
              {mode === "single" && tabConn?.isConnected && (
                <div className="mt-1 border-t pt-1">
                  <div className="px-2 pb-1 pt-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    Database
                  </div>
                  {dbLoading ? (
                    <p className="px-2 py-2 text-xs text-muted-foreground">Loading…</p>
                  ) : databases.length === 0 ? (
                    <p className="px-2 py-1.5 text-xs text-muted-foreground">{activeDatabase || "—"}</p>
                  ) : (
                    databases.map((db) => (
                      <button
                        key={db}
                        type="button"
                        className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs hover:bg-muted"
                        onClick={() => handleSwitchDatabase(db)}
                      >
                        <span className="flex-1 truncate">{db}</span>
                        {activeDatabase === db && <Check className="h-3 w-3 flex-shrink-0" />}
                      </button>
                    ))
                  )}
                </div>
              )}
            </div>
          </ScrollArea>

          {mode === "multi" && (
            <p className="border-t px-3 py-2 text-[10px] text-muted-foreground">
              Selected connections power <code>@connection.table</code> autocomplete.
            </p>
          )}

          <div className="flex items-center justify-between gap-2 border-t px-3 py-2 text-[10px] text-muted-foreground">
            <span>{connectedCount}/{envConnections.length} connected</span>
            {availableEnvironments.length > 0 && activeTab.environmentSnapshot && (
              <Badge variant="outline" className="text-[9px]">{activeTab.environmentSnapshot}</Badge>
            )}
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}
