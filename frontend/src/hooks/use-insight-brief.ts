import { useCallback, useState } from 'react'

import {
  generateInsightBrief,
  type InsightBriefRequest,
  type InsightBriefResponse,
} from '@/lib/insight-api'

export interface UseInsightBriefResult {
  brief: InsightBriefResponse | null
  loading: boolean
  error: string | null
  run: (req: InsightBriefRequest) => Promise<void>
  reset: () => void
}

/**
 * useInsightBrief manages the request lifecycle for an Auto Insight Brief:
 * loading and error state plus the latest result. The actual provider call is
 * delegated to the Wails backend via generateInsightBrief.
 */
export function useInsightBrief(): UseInsightBriefResult {
  const [brief, setBrief] = useState<InsightBriefResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const run = useCallback(async (req: InsightBriefRequest) => {
    setLoading(true)
    setError(null)
    try {
      const result = await generateInsightBrief(req)
      setBrief(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate insight brief')
      setBrief(null)
    } finally {
      setLoading(false)
    }
  }, [])

  const reset = useCallback(() => {
    setBrief(null)
    setError(null)
    setLoading(false)
  }, [])

  return { brief, loading, error, run, reset }
}
