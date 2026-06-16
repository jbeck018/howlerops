import { useCallback } from "react"
import { useShallow } from "zustand/react/shallow"

import { useAIQueryAgentStore } from "@/store/ai-query-agent-store"
import { useAIConfig } from "@/store/ai-store"
import { useConnectionStore } from "@/store/connection-store"
import { useQueryEditorStore } from "@/store/query-editor-store"
import type { QueryTab } from "@/store/query-types"

/**
 * Tab lifecycle actions lifted out of the query editor so they can be driven
 * from anywhere — notably the vertical Open Tabs list in the ContextPanel, which
 * is a sibling of the editor and cannot receive props from it. Reads the stores
 * directly (query-editor, connection, AI agent/config) rather than re-running the
 * editor's useAIIntegration.
 */
export function useTabActions() {
  const { createTab, updateTab, setActiveTab } = useQueryEditorStore(
    useShallow((s) => ({
      createTab: s.createTab,
      updateTab: s.updateTab,
      setActiveTab: s.setActiveTab,
    }))
  )
  const { connections, activeConnection, connectToDatabase, setActiveConnection } = useConnectionStore(
    useShallow((s) => ({
      connections: s.connections,
      activeConnection: s.activeConnection,
      connectToDatabase: s.connectToDatabase,
      setActiveConnection: s.setActiveConnection,
    }))
  )
  const createSession = useAIQueryAgentStore((s) => s.createSession)
  const setActiveSession = useAIQueryAgentStore((s) => s.setActiveSession)
  const { config: aiConfig } = useAIConfig()

  const createSqlTab = useCallback(() => {
    const id = createTab("New Query", { type: "sql", connectionId: activeConnection?.id })
    setActiveTab(id)
    return id
  }, [createTab, setActiveTab, activeConnection?.id])

  const createAiTab = useCallback(() => {
    const sessionId = createSession({
      title: `AI Query ${new Date().toLocaleTimeString()}`,
      provider: aiConfig.provider,
      model: aiConfig.selectedModel,
    })
    const id = createTab("AI Query Agent", {
      type: "ai",
      connectionId: activeConnection?.id,
      aiSessionId: sessionId,
    })
    updateTab(id, {
      connectionId: activeConnection?.id,
      selectedConnectionIds: activeConnection?.id ? [activeConnection.id] : [],
    })
    setActiveTab(id)
    setActiveSession(sessionId)
    return id
  }, [
    createSession,
    aiConfig.provider,
    aiConfig.selectedModel,
    createTab,
    updateTab,
    setActiveTab,
    setActiveSession,
    activeConnection?.id,
  ])

  const changeTabConnection = useCallback(
    async (tabId: string, connectionId: string) => {
      updateTab(tabId, { connectionId, selectedConnectionIds: [connectionId] })
      const conn = connections.find((c) => c.id === connectionId)
      if (!conn) {
        setActiveConnection(null)
        return
      }
      if (!conn.isConnected) {
        try {
          await connectToDatabase(connectionId)
        } catch (error) {
          console.error("Failed to connect to database:", error)
          return
        }
      }
      const updated = useConnectionStore.getState().connections.find((c) => c.id === connectionId)
      if (updated?.isConnected) {
        setActiveConnection(updated)
      }
    },
    [updateTab, connections, connectToDatabase, setActiveConnection]
  )

  const getConnectionLabelForTab = useCallback(
    (tab: Pick<QueryTab, "connectionId">) => {
      if (!tab.connectionId) {
        return "Select DB"
      }
      return connections.find((c) => c.id === tab.connectionId)?.name ?? "Select DB"
    },
    [connections]
  )

  return { connections, createSqlTab, createAiTab, changeTabConnection, getConnectionLabelForTab }
}
