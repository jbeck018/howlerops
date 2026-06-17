import { ChevronDown, Database, Folder, FolderOpen, Plus } from "lucide-react"
import { useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { cn } from "@/lib/utils"

import type { ConnectionGroup, DatabaseConnection } from "../types"
import { ConnectionCard } from "./connection-card"

interface ConnectionListProps {
  connections: DatabaseConnection[]
  groupedConnections: ConnectionGroup[]
  groupByEnvironment: boolean
  isConnecting: boolean
  hasConnections: boolean
  activeEnvironmentFilter: string | null
  onAddConnection: () => void
  onEditConnection: (connection: DatabaseConnection) => void
  onDeleteConnection: (connection: DatabaseConnection) => void
  onConnectConnection: (connection: DatabaseConnection) => void
  onDiagnosticsConnection: (connection: DatabaseConnection) => void
  onClearEnvironmentFilter: () => void
}

/**
 * Component for displaying the list/grid of connections
 */
export function ConnectionList({
  connections,
  groupedConnections,
  groupByEnvironment,
  isConnecting,
  hasConnections,
  activeEnvironmentFilter,
  onAddConnection,
  onEditConnection,
  onDeleteConnection,
  onConnectConnection,
  onDiagnosticsConnection,
  onClearEnvironmentFilter,
}: ConnectionListProps) {
  const renderConnectionCard = (connection: DatabaseConnection) => (
    <ConnectionCard
      key={connection.id}
      connection={connection}
      isConnecting={isConnecting}
      onEdit={onEditConnection}
      onDelete={onDeleteConnection}
      onConnect={onConnectConnection}
      onDiagnostics={onDiagnosticsConnection}
    />
  )

  const renderEmptyState = (message: string) => (
    <Card className="col-span-full">
      <CardContent className="flex flex-col items-center justify-center py-12 text-center space-y-4">
        <Database className="h-12 w-12 text-muted-foreground" />
        <div>
          <CardTitle className="mb-2">No connections</CardTitle>
          <CardDescription>{message}</CardDescription>
        </div>
        {!hasConnections ? (
          <Button onClick={onAddConnection}>
            <Plus className="h-4 w-4 mr-2" />
            Add Connection
          </Button>
        ) : activeEnvironmentFilter ? (
          <Button variant="outline" onClick={onClearEnvironmentFilter}>
            Clear Environment Filter
          </Button>
        ) : (
          <Button onClick={onAddConnection}>
            <Plus className="h-4 w-4 mr-2" />
            Add Connection
          </Button>
        )}
      </CardContent>
    </Card>
  )

  // Grouped view — each environment is a collapsible folder.
  if (groupByEnvironment) {
    if (groupedConnections.length > 0) {
      return (
        <div className="space-y-4">
          {groupedConnections.map((group) => (
            <ConnectionFolderSection key={group.key} group={group} renderCard={renderConnectionCard} />
          ))}
        </div>
      )
    }

    return (
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {renderEmptyState(
          activeEnvironmentFilter
            ? 'No connections match this environment filter.'
            : 'Add your first database connection to get started.'
        )}
      </div>
    )
  }

  // Flat view
  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      {connections.length > 0
        ? connections.map(renderConnectionCard)
        : renderEmptyState(
            !hasConnections
              ? 'Add your first database connection to get started.'
              : 'No connections match this environment filter.'
          )}
    </div>
  )
}

/**
 * A single collapsible environment folder on the connections page. Defaults to
 * open; collapse state is local so users can tuck away environments.
 */
function ConnectionFolderSection({
  group,
  renderCard,
}: {
  group: ConnectionGroup
  renderCard: (connection: DatabaseConnection) => React.ReactNode
}) {
  const [open, setOpen] = useState(true)
  const count = group.connections.length

  return (
    <Collapsible open={open} onOpenChange={setOpen} className="rounded-lg border bg-card/40">
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex w-full items-center justify-between gap-2 rounded-t-lg px-4 py-3 hover:bg-muted/40"
        >
          <div className="flex items-center gap-2">
            <ChevronDown className={cn("h-4 w-4 text-muted-foreground transition-transform", !open && "-rotate-90")} />
            {open ? (
              <FolderOpen className="h-4 w-4 text-muted-foreground" />
            ) : (
              <Folder className="h-4 w-4 text-muted-foreground" />
            )}
            <h3 className="text-sm font-semibold">{group.label}</h3>
          </div>
          <Badge variant="outline" className="text-xs">
            {count} {count === 1 ? "connection" : "connections"}
          </Badge>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="grid gap-4 p-4 pt-1 md:grid-cols-2 lg:grid-cols-3">
          {group.connections.map(renderCard)}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}
