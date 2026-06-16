import { useMemo, useState } from "react"

import { groupConnectionsByEnvironment } from "@/lib/group-connections-by-environment"

import type { ConnectionGroup, DatabaseConnection } from "../types"
import { UNASSIGNED_ENVIRONMENT_LABEL } from "../types"

interface UseConnectionListReturn {
  filteredConnections: DatabaseConnection[]
  groupedConnections: ConnectionGroup[]
  groupByEnvironment: boolean
  setGroupByEnvironment: (grouped: boolean) => void
}

interface UseConnectionListOptions {
  connections: DatabaseConnection[]
  activeEnvironmentFilter: string | null
  availableEnvironments: string[]
}

/**
 * Hook for filtering and grouping connections
 */
export function useConnectionList({
  connections,
  activeEnvironmentFilter,
  availableEnvironments,
}: UseConnectionListOptions): UseConnectionListReturn {
  // Folders on by default so the connections page shows the environment
  // structure; users can still flatten via the toggle.
  const [groupByEnvironment, setGroupByEnvironment] = useState(true)

  // Filter connections by active environment
  const filteredConnections = useMemo(() => {
    if (!activeEnvironmentFilter) {
      return connections
    }

    return connections.filter((conn) => {
      if (!conn.environments || conn.environments.length === 0) {
        return false
      }
      return conn.environments.includes(activeEnvironmentFilter)
    })
  }, [connections, activeEnvironmentFilter])

  // Group connections by environment (shared with the sidebar folders).
  const groupedConnections = useMemo<ConnectionGroup[]>(() => {
    if (!groupByEnvironment) {
      return []
    }
    return groupConnectionsByEnvironment(
      filteredConnections,
      availableEnvironments,
      UNASSIGNED_ENVIRONMENT_LABEL
    )
  }, [filteredConnections, groupByEnvironment, availableEnvironments])

  return {
    filteredConnections,
    groupedConnections,
    groupByEnvironment,
    setGroupByEnvironment,
  }
}
