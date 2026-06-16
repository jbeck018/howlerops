import {
  ChevronDown,
  Database,
  Filter,
  Loader2,
  Tag,
} from "lucide-react"
import { lazy, Suspense, useCallback, useMemo, useState } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useShallow } from "zustand/react/shallow"

import { ConnectionSchemaViewer } from "@/components/connection-schema-viewer"
import { EnvironmentManager } from "@/components/environment-manager"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { cn } from "@/lib/utils"
import { type DatabaseConnection, useConnectionStore } from "@/store/connection-store"
import { useQueryEditorStore } from "@/store/query-editor-store"

import { ConnectionRow } from "./connection-row"

// Re-exported for consumers that render schema trees outside the sidebar.
export { SchemaTree } from "./schema-tree"

// Lazy-load the heavy schema visualizer (uses reactflow)
const SchemaVisualizerWrapper = lazy(() =>
  import("@/components/schema-visualizer/schema-visualizer").then(m => ({ default: m.SchemaVisualizerWrapper }))
)

export function Sidebar() {
  const navigate = useNavigate()
  const {
    connections,
    activeConnection,
    setActiveConnection,
    connectToDatabase,
    isConnecting,
    activeEnvironmentFilter,
    availableEnvironments,
    setEnvironmentFilter,
  } = useConnectionStore(useShallow((state) => ({
    connections: state.connections,
    activeConnection: state.activeConnection,
    setActiveConnection: state.setActiveConnection,
    connectToDatabase: state.connectToDatabase,
    isConnecting: state.isConnecting,
    activeEnvironmentFilter: state.activeEnvironmentFilter,
    availableEnvironments: state.availableEnvironments,
    setEnvironmentFilter: state.setEnvironmentFilter,
  })))
  const { tabs, activeTabId, updateTab } = useQueryEditorStore(useShallow((state) => ({
    tabs: state.tabs,
    activeTabId: state.activeTabId,
    updateTab: state.updateTab,
  })))

  const [connectingId, setConnectingId] = useState<string | null>(null)
  const [showEnvironmentManager, setShowEnvironmentManager] = useState(false)
  const [connectionsExpanded, setConnectionsExpanded] = useState(true)
  const [schemaViewConnectionId, setSchemaViewConnectionId] = useState<string | null>(null)
  const [diagramConnectionId, setDiagramConnectionId] = useState<string | null>(null)

  // Compute these once per render rather than per connection row.
  const activeTab = useMemo(
    () => tabs.find(tab => tab.id === activeTabId),
    [tabs, activeTabId]
  )
  const filteredConnections = useMemo(() => {
    if (!activeEnvironmentFilter) {
      return connections
    }
    return connections.filter((conn) =>
      !conn.environments || conn.environments.length === 0
        ? true
        : conn.environments.includes(activeEnvironmentFilter)
    )
  }, [connections, activeEnvironmentFilter])

  const handleConnectionSelect = useCallback(async (connection: DatabaseConnection) => {
    if (connection.sessionId) {
      setActiveConnection(connection)
      return
    }
    setConnectingId(connection.id)
    try {
      await connectToDatabase(connection.id)
    } catch (error) {
      console.error('Failed to activate connection:', error)
    } finally {
      setConnectingId(null)
    }
  }, [connectToDatabase, setActiveConnection])

  const handleAddToQueryTab = useCallback((connectionId: string) => {
    if (!activeTab) {
      return
    }
    const isAlreadyInTab = activeTab.connectionId === connectionId ||
      (activeTab.selectedConnectionIds?.includes(connectionId) ?? false)
    if (isAlreadyInTab) {
      return
    }
    if (activeTab.selectedConnectionIds) {
      updateTab(activeTab.id, {
        selectedConnectionIds: [...activeTab.selectedConnectionIds, connectionId],
      })
    } else {
      updateTab(activeTab.id, { connectionId, selectedConnectionIds: [connectionId] })
    }
  }, [activeTab, updateTab])

  const handleViewSchema = useCallback((connectionId: string) => {
    setSchemaViewConnectionId(connectionId)
  }, [])
  const handleViewDiagram = useCallback((connectionId: string) => {
    setDiagramConnectionId(connectionId)
  }, [])

  return (
    <div className="w-56 border-r bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60 flex-shrink-0 flex flex-col">
      <ScrollArea className="flex-1">
        <div className="flex flex-col h-full pt-2">
          {/* Connections Section */}
          <Collapsible open={connectionsExpanded} onOpenChange={setConnectionsExpanded} className="px-2">
            <CollapsibleTrigger asChild>
              <Button variant="ghost" size="sm" className="w-full justify-between px-3 py-2 h-auto">
                <div className="flex items-center gap-2">
                  <Database className="h-4 w-4" />
                  <span className="text-xs font-semibold uppercase tracking-wider">Active Connections</span>
                </div>
                <ChevronDown className={cn("h-4 w-4 transition-transform", !connectionsExpanded && "-rotate-90")} />
              </Button>
            </CollapsibleTrigger>

            <CollapsibleContent className="space-y-1 mt-1">
              {/* Environment Filter */}
              {availableEnvironments.length > 0 && (
                <div className="px-1 mb-2 flex gap-1">
                  <Select
                    value={activeEnvironmentFilter || "__all__"}
                    onValueChange={(value) => setEnvironmentFilter(value === "__all__" ? null : value)}
                  >
                    <SelectTrigger className="h-7 text-xs flex-1">
                      <div className="flex items-center gap-1">
                        <Filter className="h-3 w-3" />
                        <SelectValue placeholder="All Envs" />
                      </div>
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="__all__">All Environments</SelectItem>
                      {availableEnvironments.map((env) => (
                        <SelectItem key={env} value={env}>
                          {env}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0"
                    onClick={() => setShowEnvironmentManager(true)}
                    title="Manage environments"
                  >
                    <Tag className="h-3 w-3" />
                  </Button>
                </div>
              )}

              {/* Manage Environments button when no environments exist */}
              {availableEnvironments.length === 0 && connections.length > 0 && (
                <div className="px-1 mb-2">
                  <Button
                    variant="outline"
                    size="sm"
                    className="w-full h-7 text-xs"
                    onClick={() => setShowEnvironmentManager(true)}
                  >
                    <Tag className="h-3 w-3 mr-1" />
                    Add Environments
                  </Button>
                </div>
              )}

              {/* Connection List */}
              <div className="space-y-1 px-1">
                {filteredConnections.length === 0 && connections.length > 0 ? (
                  <div className="text-xs text-muted-foreground text-center py-3">
                    No connections for this environment
                  </div>
                ) : filteredConnections.length === 0 ? (
                  <div className="text-xs text-muted-foreground text-center py-3">
                    <p>No connections</p>
                    <Button
                      variant="link"
                      size="sm"
                      className="h-auto p-0 text-xs"
                      onClick={() => navigate('/connections')}
                    >
                      Add one
                    </Button>
                  </div>
                ) : (
                  filteredConnections.map((connection) => {
                    const isInActiveTab = Boolean(activeTab && (
                      activeTab.connectionId === connection.id ||
                      (activeTab.selectedConnectionIds?.includes(connection.id) ?? false)
                    ))
                    return (
                      <ConnectionRow
                        key={connection.id}
                        connection={connection}
                        isActive={activeConnection?.id === connection.id}
                        isPending={connectingId === connection.id}
                        isConnecting={isConnecting}
                        isInActiveTab={isInActiveTab}
                        hasActiveTab={Boolean(activeTab)}
                        onSelect={handleConnectionSelect}
                        onAddToQueryTab={handleAddToQueryTab}
                        onViewSchema={handleViewSchema}
                        onViewDiagram={handleViewDiagram}
                      />
                    )
                  })
                )}
              </div>
            </CollapsibleContent>
          </Collapsible>

          <div className="flex-1" />
        </div>
      </ScrollArea>

      {/* Connection Schema Viewer Modal */}
      {schemaViewConnectionId && (
        <ConnectionSchemaViewer
          connectionId={schemaViewConnectionId}
          onClose={() => setSchemaViewConnectionId(null)}
        />
      )}

      {/* Connection Diagram Modal */}
      {diagramConnectionId && createPortal(
        <Suspense fallback={
          <div className="fixed inset-0 bg-background/80 backdrop-blur-sm flex items-center justify-center z-50">
            <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
          </div>
        }>
          <SchemaVisualizerWrapper
            schema={[]}
            connectionId={diagramConnectionId}
            onClose={() => setDiagramConnectionId(null)}
          />
        </Suspense>,
        document.body
      )}

      {/* Environment Manager Modal */}
      {showEnvironmentManager && (
        <EnvironmentManager
          open={showEnvironmentManager}
          onClose={() => setShowEnvironmentManager(false)}
        />
      )}
    </div>
  )
}
