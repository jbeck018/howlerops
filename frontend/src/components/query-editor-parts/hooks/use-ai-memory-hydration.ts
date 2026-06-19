import { useEffect } from "react"

interface UseAIMemoryHydrationParams {
  aiEnabled: boolean
  syncMemories: boolean
  hydrateMemoriesFromBackend: () => Promise<unknown>
  memoriesHydrated: boolean
  agentHydrated: boolean
  syncAgentFromMemory: () => void
}

export function useAIMemoryHydration({
  aiEnabled,
  syncMemories,
  hydrateMemoriesFromBackend,
  memoriesHydrated,
  agentHydrated,
  syncAgentFromMemory,
}: UseAIMemoryHydrationParams): void {
  useEffect(() => {
    if (!aiEnabled || !syncMemories) {
      return
    }

    hydrateMemoriesFromBackend().catch(error => {
      console.error('Failed to hydrate AI memories:', error)
    })
  }, [aiEnabled, syncMemories, hydrateMemoriesFromBackend])

  useEffect(() => {
    if (memoriesHydrated && !agentHydrated) {
      syncAgentFromMemory()
    }
  }, [memoriesHydrated, agentHydrated, syncAgentFromMemory])
}
