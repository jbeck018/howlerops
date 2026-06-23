import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'

import { toast } from '@/hooks/use-toast'
import {
  type AlertCheckResponse,
  type AlertFiredEvent,
  checkAlertNow as apiCheck,
  deleteAlert as apiDelete,
  listAlerts as apiList,
  setAlertEnabled as apiSetEnabled,
  type TimeSeriesAlert,
} from '@/lib/alert-api'

export interface UseAlertsResult {
  alerts: TimeSeriesAlert[]
  loading: boolean
  error: string | null
  refresh: () => Promise<void>
  toggle: (id: string, enabled: boolean) => Promise<void>
  remove: (id: string) => Promise<void>
  check: (id: string) => Promise<AlertCheckResponse>
}

/** useAlerts loads and manages standalone time-series alerts. */
export function useAlerts(): UseAlertsResult {
  const [alerts, setAlerts] = useState<TimeSeriesAlert[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setAlerts(await apiList())
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load alerts')
    } finally {
      setLoading(false)
    }
  }, [])

  const toggle = useCallback(
    async (id: string, enabled: boolean) => {
      try {
        await apiSetEnabled(id, enabled)
      } catch (err) {
        toast({
          title: enabled ? 'Failed to enable alert' : 'Failed to disable alert',
          description: err instanceof Error ? err.message : 'Error',
          variant: 'destructive',
        })
      }
      await refresh()
    },
    [refresh],
  )

  const remove = useCallback(
    async (id: string) => {
      try {
        await apiDelete(id)
      } catch (err) {
        toast({
          title: 'Failed to delete alert',
          description: err instanceof Error ? err.message : 'Error',
          variant: 'destructive',
        })
      }
      await refresh()
    },
    [refresh],
  )

  const check = useCallback((id: string) => apiCheck(id), [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return { alerts, loading, error, refresh, toggle, remove, check }
}

/**
 * useAlertNotifications subscribes to the backend "alert:fired" event and raises
 * a toast for each firing. Call once near the app root.
 */
export function useAlertNotifications(): void {
  useEffect(() => {
    const off = Events.On('alert:fired', (event: { data?: AlertFiredEvent } | AlertFiredEvent) => {
      // Wails v3 wraps the payload in { data }; tolerate both shapes.
      const payload = (event && 'data' in event ? event.data : event) as AlertFiredEvent | undefined
      if (!payload) return
      toast({
        title: `Alert: ${payload.name}`,
        description: payload.message,
        variant: 'destructive',
      })
    })
    return () => {
      if (typeof off === 'function') off()
    }
  }, [])
}
