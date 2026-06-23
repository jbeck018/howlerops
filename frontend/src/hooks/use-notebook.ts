import { useCallback, useEffect, useRef, useState } from 'react'

import {
  getNotebook as apiGet,
  listNotebooks as apiList,
  type NotebookDefinition,
  type NotebookRunRequest,
  type NotebookRunResult,
  type NotebookSummary,
  runNotebook as apiRun,
} from '@/lib/notebook-api'

export interface UseNotebooksResult {
  notebooks: NotebookSummary[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
}

/** useNotebooks loads the list of saved notebooks. */
export function useNotebooks(): UseNotebooksResult {
  const [notebooks, setNotebooks] = useState<NotebookSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setNotebooks(await apiList())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load notebooks')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return { notebooks, loading, error, refresh }
}

export interface UseNotebookRunResult {
  definition: NotebookDefinition | null
  result: NotebookRunResult | null
  loading: boolean
  running: boolean
  error: string | null
  load: (id: string) => Promise<void>
  run: (req: NotebookRunRequest) => Promise<void>
}

/** useNotebookRun loads a single notebook and runs it. */
export function useNotebookRun(): UseNotebookRunResult {
  const [definition, setDefinition] = useState<NotebookDefinition | null>(null)
  const [result, setResult] = useState<NotebookRunResult | null>(null)
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
      setError(err instanceof Error ? err.message : 'Failed to load notebook')
      setDefinition(null)
    } finally {
      if (seq === loadSeq.current) setLoading(false)
    }
  }, [])

  const run = useCallback(async (req: NotebookRunRequest) => {
    setRunning(true)
    setError(null)
    try {
      setResult(await apiRun(req))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to run notebook')
    } finally {
      setRunning(false)
    }
  }, [])

  return { definition, result, loading, running, error, load, run }
}
