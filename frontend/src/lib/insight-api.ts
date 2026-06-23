// Insight Brief API client.
//
// The Wails binding `GenerateInsightBrief` is generated into
// frontend/bindings/... by the Wails toolchain at app build time. To keep
// `tsc` green before that regeneration (the binding is committed and only
// refreshed by a desktop build), we access it through a typed indirection and
// surface a clear error if a stale build lacks it. Once the app is rebuilt the
// call resolves normally.

export interface InsightBriefRequest {
  connectionId: string
  sql: string
  title?: string
  question?: string
  provider?: string
  model?: string
  forecast?: boolean
  timeColumn?: string
  valueColumn?: string
  horizon?: number
  seasonLength?: number
}

export interface InsightForecastPoint {
  time: string
  value: number
  lower: number
  upper: number
}

export interface InsightAnomaly {
  time: string
  value: number
  expected: number
  score: number
}

export interface InsightBriefResponse {
  brief: string
  rowCount: number
  forecastMethod?: string
  predictions?: InsightForecastPoint[]
  anomalies?: InsightAnomaly[]
  forecastError?: string
}

// Shape of the generated bindings module we depend on. Declared locally so this
// file type-checks against bindings that predate GenerateInsightBrief.
type InsightBindings = {
  GenerateInsightBrief?: (req: InsightBriefRequest) => Promise<InsightBriefResponse | null>
}

/**
 * Generate an Auto Insight Brief for a query: an executive narrative plus an
 * optional forecast and anomaly callouts, produced by the user's configured AI
 * provider. Only aggregates are sent to the model — never raw rows.
 */
export async function generateInsightBrief(req: InsightBriefRequest): Promise<InsightBriefResponse> {
  const mod = (await import(
    '../../bindings/github.com/jbeck018/howlerops/app'
  )) as unknown as InsightBindings

  if (typeof mod.GenerateInsightBrief !== 'function') {
    throw new Error(
      'Insight Brief is unavailable in this build. Rebuild the desktop app to regenerate bindings.',
    )
  }

  const res = await mod.GenerateInsightBrief(req)
  if (!res) {
    throw new Error('No insight brief was returned.')
  }
  return res
}
