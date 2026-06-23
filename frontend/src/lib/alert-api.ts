// Time-series alert API client (typed indirection over the Wails bindings).

export type Comparator = 'gt' | 'gte' | 'lt' | 'lte'

export interface ThresholdRule {
  comparator: Comparator
  value: number
}

export interface AnomalyRule {
  seasonLength?: number
  lookback?: number
  minScore?: number
}

export interface ForecastRule {
  horizon?: number
  seasonLength?: number
  comparator: Comparator
  value: number
}

export interface AlertRule {
  name?: string
  anomaly?: AnomalyRule
  threshold?: ThresholdRule
  forecast?: ForecastRule
}

export interface AlertRequest {
  id?: string
  name: string
  connectionId: string
  sql: string
  timeColumn?: string
  valueColumn?: string
  channel?: string
  intervalSeconds?: number
  enabled: boolean
  rule: AlertRule
}

export interface TimeSeriesAlert {
  id: string
  name: string
  connectionId: string
  sql: string
  timeColumn?: string
  valueColumn?: string
  rule: AlertRule
  intervalSeconds: number
  channel?: string
  enabled: boolean
  lastFiredAt?: string
  createdAt: string
  updatedAt: string
}

export interface AlertCheckResponse {
  fired: boolean
  kind?: string
  message?: string
  value?: number
}

/** Payload of the "alert:fired" Wails event. */
export interface AlertFiredEvent {
  alertId: string
  name: string
  kind: string
  message: string
  value: number
  at: string
}

type AlertBindings = {
  SaveAlert?: (req: AlertRequest) => Promise<string>
  ListAlerts?: () => Promise<TimeSeriesAlert[] | null>
  GetAlert?: (id: string) => Promise<TimeSeriesAlert | null>
  DeleteAlert?: (id: string) => Promise<void>
  SetAlertEnabled?: (id: string, enabled: boolean) => Promise<void>
  CheckAlertNow?: (id: string) => Promise<AlertCheckResponse | null>
}

async function bindings(): Promise<AlertBindings> {
  return (await import(
    '../../bindings/github.com/jbeck018/howlerops/app'
  )) as unknown as AlertBindings
}

function unavailable(): never {
  throw new Error('Alerts are unavailable in this build. Rebuild the desktop app to regenerate bindings.')
}

export async function saveAlert(req: AlertRequest): Promise<string> {
  const mod = await bindings()
  if (!mod.SaveAlert) unavailable()
  return mod.SaveAlert(req)
}

export async function listAlerts(): Promise<TimeSeriesAlert[]> {
  const mod = await bindings()
  if (!mod.ListAlerts) unavailable()
  return (await mod.ListAlerts()) ?? []
}

export async function deleteAlert(id: string): Promise<void> {
  const mod = await bindings()
  if (!mod.DeleteAlert) unavailable()
  return mod.DeleteAlert(id)
}

export async function setAlertEnabled(id: string, enabled: boolean): Promise<void> {
  const mod = await bindings()
  if (!mod.SetAlertEnabled) unavailable()
  return mod.SetAlertEnabled(id, enabled)
}

export async function checkAlertNow(id: string): Promise<AlertCheckResponse> {
  const mod = await bindings()
  if (!mod.CheckAlertNow) unavailable()
  const res = await mod.CheckAlertNow(id)
  if (!res) throw new Error('No check result returned')
  return res
}
