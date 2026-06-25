import { Pencil, Play, Plus, RotateCw } from 'lucide-react'
import { type ReactNode, useEffect, useMemo, useState } from 'react'
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  Cell as RChartCell,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'

import { ParamForm } from '@/components/shared/param-form'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useNotebookRun, useNotebooks } from '@/hooks/use-notebook'
import { type ChartSpec, type NotebookCellResult } from '@/lib/notebook-api'
import { defaultValues, type ParamValues } from '@/lib/param-types'

import { NotebookEditorDialog } from './notebook-editor-dialog'

const CHART_COLORS = ['#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed', '#0891b2', '#db2777', '#65a30d']

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

function kindBadge(kind: string) {
  const variant =
    kind === 'action' ? 'destructive' : kind === 'markdown' || kind === 'notify' ? 'secondary' : 'outline'
  return (
    <Badge variant={variant as 'destructive' | 'secondary' | 'outline'} className="text-[10px] uppercase">
      {kind}
    </Badge>
  )
}

/** ResultTable renders a query result with a bounded, scrollable grid. */
function ResultTable({ columns, rows }: { columns: string[]; rows: Record<string, unknown>[] }) {
  const shown = rows.slice(0, 200)
  if (columns.length === 0) return <p className="text-xs text-muted-foreground">No rows.</p>
  return (
    <div className="max-h-80 overflow-auto rounded border">
      <Table>
        <TableHeader className="sticky top-0 bg-background">
          <TableRow>
            {columns.map((c) => (
              <TableHead key={c}>{c}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((row, i) => (
            <TableRow key={i}>
              {columns.map((c) => (
                <TableCell key={c} className="font-mono text-xs">
                  {formatCell(row[c])}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {rows.length > shown.length && (
        <div className="px-2 py-1 text-[11px] text-muted-foreground">
          Showing {shown.length} of {rows.length} rows.
        </div>
      )}
    </div>
  )
}

/** NotebookChart renders a chart cell from its source cell's result. */
function NotebookChart({ spec, source }: { spec: ChartSpec; source?: NotebookCellResult }) {
  const rows = (source?.rows ?? []) as Record<string, unknown>[]
  const x = spec.x ?? source?.columns?.[0] ?? ''
  const ys = spec.y && spec.y.length > 0 ? spec.y : (source?.columns ?? []).filter((c) => c !== x).slice(0, 1)

  if (!source) {
    return <p className="text-xs text-muted-foreground">Chart source “{spec.source}” has no result yet.</p>
  }
  if (rows.length === 0 || ys.length === 0) {
    return <p className="text-xs text-muted-foreground">Nothing to chart.</p>
  }

  const data = rows.map((r) => {
    const point: Record<string, unknown> = { [x]: formatCell(r[x]) }
    for (const y of ys) point[y] = Number(r[y] ?? 0)
    return point
  })

  const common = (
    <>
      <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
      <XAxis dataKey={x} fontSize={11} />
      <YAxis fontSize={11} />
      <Tooltip />
      <Legend />
    </>
  )

  return (
    <div className="h-64 w-full">
      <ResponsiveContainer width="100%" height="100%">
        {spec.type === 'line' ? (
          <LineChart data={data}>
            {common}
            {ys.map((y, i) => (
              <Line key={y} type="monotone" dataKey={y} stroke={CHART_COLORS[i % CHART_COLORS.length]} dot={false} />
            ))}
          </LineChart>
        ) : spec.type === 'area' ? (
          <AreaChart data={data}>
            {common}
            {ys.map((y, i) => (
              <Area
                key={y}
                type="monotone"
                dataKey={y}
                stackId={spec.stacked ? '1' : undefined}
                stroke={CHART_COLORS[i % CHART_COLORS.length]}
                fill={CHART_COLORS[i % CHART_COLORS.length]}
                fillOpacity={0.25}
              />
            ))}
          </AreaChart>
        ) : spec.type === 'pie' ? (
          <PieChart>
            <Tooltip />
            <Legend />
            <Pie data={data} dataKey={ys[0]} nameKey={x} outerRadius={90} label>
              {data.map((_, i) => (
                <RChartCell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
              ))}
            </Pie>
          </PieChart>
        ) : (
          <BarChart data={data}>
            {common}
            {ys.map((y, i) => (
              <Bar key={y} dataKey={y} stackId={spec.stacked ? '1' : undefined} fill={CHART_COLORS[i % CHART_COLORS.length]} />
            ))}
          </BarChart>
        )}
      </ResponsiveContainer>
    </div>
  )
}

function statusBadges(cell: NotebookCellResult) {
  const badges: ReactNode[] = []
  if (cell.planned) badges.push(<Badge key="p" variant="secondary">planned (dry run)</Badge>)
  if (cell.status === 'preserved') badges.push(<Badge key="pr" variant="outline">unchanged</Badge>)
  if (cell.status === 'skipped') badges.push(<Badge key="s" variant="outline">skipped</Badge>)
  if (cell.status === 'error') badges.push(<Badge key="e" variant="destructive">error</Badge>)
  if (cell.notified) badges.push(<Badge key="n" variant="secondary">sent</Badge>)
  return badges
}

/** CellOutput renders one cell's output by kind. */
function CellOutput({ cell, source }: { cell: NotebookCellResult; source?: NotebookCellResult }) {
  if (cell.status === 'error') {
    return (
      <Alert variant="destructive">
        <AlertDescription>{cell.error}</AlertDescription>
      </Alert>
    )
  }
  if (cell.status === 'skipped') {
    return <p className="text-xs text-muted-foreground">Skipped{cell.skipped ? `: ${cell.skipped}` : '.'}</p>
  }
  if (cell.status === 'preserved') {
    return <p className="text-xs text-muted-foreground">Unchanged — re-run to refresh.</p>
  }

  switch (cell.kind) {
    case 'markdown':
      return <div className="whitespace-pre-wrap text-sm text-foreground">{cell.markdown}</div>
    case 'notify':
      return (
        <div className="text-sm">
          {cell.planned ? 'Would notify: ' : cell.notified ? 'Notified: ' : ''}
          <span className="text-muted-foreground">{cell.message}</span>
        </div>
      )
    case 'action':
      return (
        <div className="space-y-1">
          {cell.sql && <pre className="overflow-x-auto whitespace-pre-wrap text-xs text-muted-foreground">{cell.sql}</pre>}
          {cell.planned ? (
            <p className="text-xs text-muted-foreground">Planned (not executed).</p>
          ) : (
            <p className="text-xs">{cell.affected ?? 0} row(s) affected.</p>
          )}
        </div>
      )
    case 'chart':
      return cell.chart ? <NotebookChart spec={cell.chart} source={source} /> : null
    default: // sql
      return (
        <div className="space-y-1">
          {cell.sql && <pre className="overflow-x-auto whitespace-pre-wrap text-xs text-muted-foreground">{cell.sql}</pre>}
          <ResultTable columns={cell.columns ?? []} rows={cell.rows ?? []} />
        </div>
      )
  }
}

/**
 * NotebookPanel lists saved notebooks and runs the selected one as a reactive,
 * cell-based document: run all, or re-run a single cell (and its descendants)
 * in place. Action cells respect the dry-run / auto-approve guardrail.
 */
export function NotebookPanel() {
  const { notebooks, loading: listing, error: listError, refresh } = useNotebooks()
  const { definition, result, loading, running, error, load, run } = useNotebookRun()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [values, setValues] = useState<ParamValues>({})
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [dryRun, setDryRun] = useState(true)
  const [autoApprove, setAutoApprove] = useState(false)

  useEffect(() => {
    if (selectedId) void load(selectedId)
  }, [selectedId, load])

  useEffect(() => {
    setValues(definition?.inputs ? defaultValues(definition.inputs) : {})
  }, [definition])

  const hasActions = useMemo(
    () => (definition?.cells ?? []).some((c) => c.kind === 'action'),
    [definition],
  )

  const cellMeta = useMemo(() => {
    const map: Record<string, { title: string; name?: string }> = {}
    for (const c of definition?.cells ?? []) map[c.id] = { title: c.title || c.id, name: c.name }
    return map
  }, [definition])

  // Look up a cell result by handle (name) so chart cells can find their source.
  const resultByName = useMemo(() => {
    const map = new Map<string, NotebookCellResult>()
    for (const c of result?.cells ?? []) if (c.name) map.set(c.name, c)
    return map
  }, [result])

  const runAll = () =>
    selectedId && void run({ notebookId: selectedId, inputs: values, dryRun, autoApprove })

  const runCell = (cellId: string) =>
    selectedId && void run({ notebookId: selectedId, inputs: values, dryRun, autoApprove, only: [cellId] })

  const handleSaved = (id: string) => {
    void refresh()
    if (id === selectedId) void load(id)
    else setSelectedId(id)
  }

  return (
    <div className="flex h-full min-h-0 gap-4 p-3">
      <Card className="w-64 shrink-0 overflow-auto">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-sm">Notebooks</CardTitle>
          <div className="flex items-center gap-1">
            <Button size="sm" variant="ghost" onClick={() => void refresh()}>
              Refresh
            </Button>
            <Button
              size="sm"
              onClick={() => {
                setEditing(false)
                setEditorOpen(true)
              }}
            >
              <Plus className="mr-1 h-3.5 w-3.5" />
              New
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-1">
          {listing && <p className="text-xs text-muted-foreground">Loading…</p>}
          {listError && (
            <Alert variant="destructive">
              <AlertDescription>{listError}</AlertDescription>
            </Alert>
          )}
          {!listing && notebooks.length === 0 && <p className="text-xs text-muted-foreground">No notebooks yet.</p>}
          {notebooks.map((nb) => (
            <button
              key={nb.id}
              onClick={() => setSelectedId(nb.id)}
              className={`block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted ${
                selectedId === nb.id ? 'bg-muted font-medium' : ''
              }`}
            >
              <div>{nb.name}</div>
              {nb.lastRunStatus && (
                <div className="text-[10px] text-muted-foreground">last run: {nb.lastRunStatus}</div>
              )}
            </button>
          ))}
        </CardContent>
      </Card>

      <Card className="flex-1 overflow-auto">
        <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
          <CardTitle className="text-sm">{definition?.name ?? 'Select a notebook'}</CardTitle>
          {definition && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setEditing(true)
                setEditorOpen(true)
              }}
            >
              <Pencil className="mr-1 h-3.5 w-3.5" />
              Edit
            </Button>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {loading && <p className="text-xs text-muted-foreground">Loading…</p>}
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          {definition && (
            <>
              {definition.description && (
                <p className="text-sm text-muted-foreground">{definition.description}</p>
              )}
              <ParamForm inputs={definition.inputs ?? []} values={values} onChange={setValues} idPrefix="nb" />

              <div className="flex flex-wrap items-center gap-4">
                <Button onClick={runAll} disabled={running}>
                  <Play className="mr-1.5 h-4 w-4" />
                  {running ? 'Running…' : 'Run all'}
                </Button>
                <div className="flex items-center gap-2">
                  <Switch id="nb-dryrun" checked={dryRun} onCheckedChange={setDryRun} />
                  <Label htmlFor="nb-dryrun" className="text-xs">
                    Dry run
                  </Label>
                </div>
                {hasActions && !dryRun && (
                  <div className="flex items-center gap-2">
                    <Switch id="nb-approve" checked={autoApprove} onCheckedChange={setAutoApprove} />
                    <Label htmlFor="nb-approve" className="text-xs text-destructive">
                      Auto-approve writes
                    </Label>
                  </div>
                )}
                {result?.dryRun && <Badge variant="secondary">dry run</Badge>}
                {result?.failed && <Badge variant="destructive">some cells failed</Badge>}
              </div>

              <div className="space-y-4">
                {(result?.cells ?? []).map((cell) => {
                  const meta = cellMeta[cell.cellId]
                  const source = cell.chart?.source ? resultByName.get(cell.chart.source) : undefined
                  return (
                    <div key={cell.cellId} className="space-y-1.5 rounded-lg border p-3">
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2">
                          {kindBadge(cell.kind)}
                          <span className="text-sm font-medium">{meta?.title || cell.title || cell.cellId}</span>
                          {(meta?.name || cell.name) && (
                            <code className="rounded bg-muted px-1 text-[11px] text-muted-foreground">
                              {meta?.name || cell.name}
                            </code>
                          )}
                          {statusBadges(cell)}
                        </div>
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => runCell(cell.cellId)}
                          disabled={running}
                          title="Re-run this cell and its dependents"
                        >
                          <RotateCw className="mr-1 h-3.5 w-3.5" />
                          Run
                        </Button>
                      </div>
                      <CellOutput cell={cell} source={source} />
                    </div>
                  )
                })}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <NotebookEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        initial={editing ? definition : null}
        onSaved={handleSaved}
      />
    </div>
  )
}
