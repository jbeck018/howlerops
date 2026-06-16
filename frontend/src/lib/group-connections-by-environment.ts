/**
 * Group connections into environment "folders" (Prod / Dev / Local / …).
 *
 * Shared by the connections sidebar and the connections page so both present
 * the same structure. A connection tagged with multiple environments appears
 * under each matching folder; untagged connections fall into a single
 * "unassigned" folder that always sorts last. Folders are otherwise ordered by
 * `availableEnvironments`, then alphabetically.
 */
export interface ConnectionEnvGroup<T> {
  key: string
  label: string
  connections: T[]
}

export function groupConnectionsByEnvironment<T extends { environments?: string[] }>(
  connections: T[],
  availableEnvironments: string[],
  unassignedLabel: string
): Array<ConnectionEnvGroup<T>> {
  const envOrder = new Map<string, number>()
  availableEnvironments.forEach((env, idx) => envOrder.set(env, idx))

  const groupMap = new Map<string, T[]>()
  for (const conn of connections) {
    const envs =
      conn.environments && conn.environments.length > 0 ? conn.environments : [unassignedLabel]
    for (const env of envs) {
      const bucket = groupMap.get(env)
      if (bucket) {
        bucket.push(conn)
      } else {
        groupMap.set(env, [conn])
      }
    }
  }

  return Array.from(groupMap.entries())
    .map(([key, items]) => ({ key, label: key, connections: items }))
    .sort((a, b) => {
      // Unassigned always last.
      if (a.key === unassignedLabel) return 1
      if (b.key === unassignedLabel) return -1

      const orderA = envOrder.get(a.key) ?? Number.MAX_SAFE_INTEGER
      const orderB = envOrder.get(b.key) ?? Number.MAX_SAFE_INTEGER
      return orderA === orderB ? a.label.localeCompare(b.label) : orderA - orderB
    })
}
