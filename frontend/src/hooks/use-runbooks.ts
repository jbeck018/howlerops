import { useCallback, useEffect, useRef, useState } from 'react'

import {
  deleteRunbook as apiDelete,
  getRunbook as apiGet,
  listRunbooks as apiList,
  type RunbookDefinition,
  type RunbookRunRequest,
  type RunbookRunResult,
  type RunbookSummary,
  runRunbook as apiRun,
} from '@/lib/runbook-api'

export interface UseRunbooksResult {
  runbooks: RunbookSummary[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
  remove: (id: string) => Promise<void>
}

/** useRunbooks loads and manages the list of saved runbooks. */
export function useRunbooks(): UseRunbooksResult {
  const [runbooks, setRunbooks] = useState<RunbookSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setRunbooks(await apiList())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load runbooks')
    } finally {
      setLoading(false)
    }
  }, [])

  const remove = useCallback(
    async (id: string) => {
      await apiDelete(id)
      await refresh()
    },
    [refresh],
  )

  useEffect(() => {
    void refresh()
  }, [refresh])

  return { runbooks, loading, error, refresh, remove }
}

export interface UseRunbookRunResult {
  definition: RunbookDefinition | null
  result: RunbookRunResult | null
  loading: boolean
  running: boolean
  error: string | null
  load: (id: string) => Promise<void>
  run: (req: RunbookRunRequest) => Promise<void>
  reset: () => void
}

/** useRunbookRun loads a single runbook's definition and runs it. */
export function useRunbookRun(): UseRunbookRunResult {
  const [definition, setDefinition] = useState<RunbookDefinition | null>(null)
  const [result, setResult] = useState<RunbookRunResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // Guards against an earlier load() resolving after a later one (stale response).
  const loadSeq = useRef(0)

  const load = useCallback(async (id: string) => {
    const seq = ++loadSeq.current
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const def = await apiGet(id)
      if (seq !== loadSeq.current) return
      setDefinition(def)
    } catch (err) {
      if (seq !== loadSeq.current) return
      setError(err instanceof Error ? err.message : 'Failed to load runbook')
      setDefinition(null)
    } finally {
      if (seq === loadSeq.current) setLoading(false)
    }
  }, [])

  const run = useCallback(async (req: RunbookRunRequest) => {
    setRunning(true)
    setError(null)
    try {
      setResult(await apiRun(req))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to run runbook')
    } finally {
      setRunning(false)
    }
  }, [])

  const reset = useCallback(() => {
    // Invalidate any in-flight load so its response can't repopulate after reset.
    loadSeq.current++
    setDefinition(null)
    setResult(null)
    setError(null)
    setLoading(false)
  }, [])

  return { definition, result, loading, running, error, load, run, reset }
}
