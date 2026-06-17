import { Check, ChevronsUpDown, Database, Network } from "lucide-react"
import { useEffect, useState } from "react"
import { useShallow } from "zustand/react/shallow"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { type DatabaseConnection, useConnectionStore } from "@/store/connection-store"
import { useActiveTab, useQueryEditorActions } from "@/store/query-editor-store"

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
  const [databases, setDatabases] = useState<string[]>([])
  const [dbLoading, setDbLoading] = useState(false)

  const envConnections = getFilteredConnections()
  const mode = activeTab?.mode ?? (connections.length > 1 ? "multi" : "single")
  const canMulti = envConnections.length >= 2
  const tabConn = connections.find((c) => c.id === activeTab?.connectionId)
  // The database the tab targets: the per-tab override if set, else the
  // connection's globally-active database.
  const activeDatabase = activeTab?.database || tabConn?.database
  const selectedIds = activeTab?.selectedConnectionIds ?? []
  const tabConnId = tabConn?.id
  const tabConnConnected = tabConn?.isConnected

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

        {mode === "single" ? (
          <>
            <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Connection
            </div>
            <ScrollArea className="max-h-52">
              <div className="px-1 pb-1">
                {connections.length === 0 ? (
                  <p className="px-2 py-2 text-xs text-muted-foreground">No connections.</p>
                ) : (
                  connections.map((conn) => (
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
                  ))
                )}
              </div>
            </ScrollArea>

            {tabConn?.isConnected && (
              <div className="border-t">
                <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  Database
                </div>
                <ScrollArea className="max-h-40">
                  <div className="px-1 pb-2">
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
                </ScrollArea>
              </div>
            )}
          </>
        ) : (
          <>
            <div className="px-3 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              Connections in query
            </div>
            <ScrollArea className="max-h-60">
              <div className="px-1 pb-1">
                {envConnections.map((conn) => {
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
                })}
              </div>
            </ScrollArea>
            <p className="border-t px-3 py-2 text-[10px] text-muted-foreground">
              Selected connections power <code>@connection.table</code> autocomplete.
            </p>
          </>
        )}

        <div className="flex items-center justify-between gap-2 border-t px-3 py-2 text-[10px] text-muted-foreground">
          <span>{connectedCount}/{envConnections.length} connected</span>
          {availableEnvironments.length > 0 && activeTab.environmentSnapshot && (
            <Badge variant="outline" className="text-[9px]">{activeTab.environmentSnapshot}</Badge>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}
