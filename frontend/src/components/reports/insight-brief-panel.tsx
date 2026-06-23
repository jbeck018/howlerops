import { AlertTriangle, Lightbulb, TrendingUp } from 'lucide-react'
import { memo, useMemo } from 'react'
import {
  Area,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import type { InsightBriefResponse } from '@/lib/insight-api'

export interface InsightBriefPanelProps {
  brief: InsightBriefResponse | null
  loading?: boolean
  error?: string | null
  className?: string
}

interface ForecastChartDatum {
  label: string
  value: number
  // Recharts renders a band as [lower, upper] tuples on a single Area series.
  band: [number, number]
}

function shortDate(iso: string): string {
  // Inputs are RFC3339; fall back to the raw string if parsing fails.
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toISOString().slice(0, 10)
}

/**
 * InsightBriefPanel renders an Auto Insight Brief: the narrative, an optional
 * forecast chart with a confidence band, and any anomaly callouts. It is purely
 * presentational — callers supply state via useInsightBrief.
 */
export const InsightBriefPanel = memo(function InsightBriefPanel({
  brief,
  loading = false,
  error = null,
  className,
}: InsightBriefPanelProps) {
  const chartData = useMemo<ForecastChartDatum[]>(() => {
    if (!brief?.predictions?.length) return []
    return brief.predictions.map((p) => ({
      label: shortDate(p.time),
      value: p.value,
      band: [p.lower, p.upper],
    }))
  }, [brief])

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Lightbulb className="h-4 w-4 text-amber-500" />
          Insight Brief
          {brief?.forecastMethod && (
            <Badge variant="secondary" className="ml-auto gap-1 text-xs font-normal">
              <TrendingUp className="h-3 w-3" />
              {brief.forecastMethod}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>

      <CardContent className="space-y-4">
        {loading && (
          <div className="space-y-2" data-testid="insight-loading">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-5/6" />
            <Skeleton className="h-4 w-2/3" />
          </div>
        )}

        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {!loading && !error && brief && (
          <>
            <p className="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
              {brief.brief}
            </p>

            {chartData.length > 0 && (
              <div className="h-48 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <ComposedChart data={chartData} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
                    <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
                    <XAxis dataKey="label" tick={{ fontSize: 11 }} />
                    <YAxis tick={{ fontSize: 11 }} width={48} />
                    <Tooltip />
                    <Area
                      type="monotone"
                      dataKey="band"
                      stroke="none"
                      fill="hsl(var(--primary))"
                      fillOpacity={0.15}
                      name="Confidence"
                      isAnimationActive={false}
                    />
                    <Line
                      type="monotone"
                      dataKey="value"
                      stroke="hsl(var(--primary))"
                      strokeWidth={2}
                      dot={false}
                      name="Forecast"
                      isAnimationActive={false}
                    />
                  </ComposedChart>
                </ResponsiveContainer>
              </div>
            )}

            {brief.forecastError && (
              <p className="text-xs text-muted-foreground">
                Forecast skipped: {brief.forecastError}
              </p>
            )}

            {brief.anomalies && brief.anomalies.length > 0 && (
              <div className="space-y-1">
                <div className="flex items-center gap-1.5 text-xs font-medium text-foreground">
                  <AlertTriangle className="h-3.5 w-3.5 text-amber-500" />
                  Anomalies ({brief.anomalies.length})
                </div>
                <ul className="space-y-0.5">
                  {brief.anomalies.slice(0, 5).map((a, i) => (
                    <li key={`${a.time}-${i}`} className="text-xs text-muted-foreground">
                      {shortDate(a.time)}: observed {formatNumber(a.value)} vs expected{' '}
                      {formatNumber(a.expected)}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <p className="text-xs text-muted-foreground">Based on {brief.rowCount} rows.</p>
          </>
        )}

        {!loading && !error && !brief && (
          <p className="text-sm text-muted-foreground">
            Run a query and generate a brief to see narrative insights here.
          </p>
        )}
      </CardContent>
    </Card>
  )
})

function formatNumber(n: number): string {
  return Number.isInteger(n) ? String(n) : n.toFixed(2)
}
