import {
  ChevronDown,
  ChevronRight,
  Database,
  Play,
  Plus,
  RotateCw,
  Trash2,
} from 'lucide-react'
import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react'
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
import { useShallow } from 'zustand/react/shallow'

import { CodeMirrorEditor } from '@/components/codemirror-editor'
import { ParamForm } from '@/components/shared/param-form'
import { ParamInputsEditor } from '@/components/shared/param-inputs-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { useNotebookRun, useNotebooks } from '@/hooks/use-notebook'
import {
  type CellKind,
  type ChartSpec,
  deleteNotebook as apiDelete,
  type NotebookCell,
  type NotebookCellResult,
  type NotebookDefinition,
  saveNotebook,
} from '@/lib/notebook-api'
import { defaultValues, type ParamInput, type ParamValues } from '@/lib/param-types'
import { useConnectionStore } from '@/store/connection-store'

const CHART_COLORS = ['#2563eb', '#16a34a', '#d97706', '#dc2626', '#7c3aed', '#0891b2', '#db2777', '#65a30d']

const CELL_KINDS: { value: CellKind; label: string }[] = [
  { value: 'sql', label: 'SQL' },
  { value: 'markdown', label: 'Markdown' },
  { value: 'action', label: 'Action (write)' },
  { value: 'notify', label: 'Notify' },
  { value: 'chart', label: 'Chart' },
]

let cellSeq = 0
function newCellId(): string {
  cellSeq += 1
  return `cell-${Date.now().toString(36)}-${cellSeq}`
}

function blankCell(kind: CellKind): NotebookCell {
  return { id: newCellId(), kind, title: '', name: '', sql: '', markdown: '', connectionId: '', channel: '', message: '' }
}

function blankNotebook(): NotebookDefinition {
  return { name: 'Untitled notebook', cells: [blankCell('sql')] }
}

function isDark(): boolean {
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

// --- output rendering --------------------------------------------------------

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

function NotebookChart({ spec, source }: { spec: ChartSpec; source?: NotebookCellResult }) {
  const rows = (source?.rows ?? []) as Record<string, unknown>[]
  const x = spec.x ?? source?.columns?.[0] ?? ''
  const ys = spec.y && spec.y.length > 0 ? spec.y : (source?.columns ?? []).filter((c) => c !== x).slice(0, 1)

  if (!source) return <p className="text-xs text-muted-foreground">Chart source “{spec.source}” has no result yet — run it first.</p>
  if (rows.length === 0 || ys.length === 0) return <p className="text-xs text-muted-foreground">Nothing to chart.</p>

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
              <Area key={y} type="monotone" dataKey={y} stackId={spec.stacked ? '1' : undefined} stroke={CHART_COLORS[i % CHART_COLORS.length]} fill={CHART_COLORS[i % CHART_COLORS.length]} fillOpacity={0.25} />
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

function CellOutput({ cell, source }: { cell: NotebookCellResult; source?: NotebookCellResult }) {
  if (cell.status === 'error') {
    return (
      <Alert variant="destructive">
        <AlertDescription>{cell.error}</AlertDescription>
      </Alert>
    )
  }
  if (cell.status === 'skipped') return <p className="text-xs text-muted-foreground">Skipped{cell.skipped ? `: ${cell.skipped}` : '.'}</p>
  if (cell.status === 'preserved') return null

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
      return cell.planned ? (
        <p className="text-xs text-muted-foreground">Planned (not executed).</p>
      ) : (
        <p className="text-xs">{cell.affected ?? 0} row(s) affected.</p>
      )
    case 'chart':
      return cell.chart ? <NotebookChart spec={cell.chart} source={source} /> : null
    default:
      return <ResultTable columns={cell.columns ?? []} rows={cell.rows ?? []} />
  }
}

function statusBadges(out?: NotebookCellResult): ReactNode[] {
  if (!out) return []
  const b: ReactNode[] = []
  if (out.planned) b.push(<Badge key="p" variant="secondary">planned</Badge>)
  if (out.status === 'error') b.push(<Badge key="e" variant="destructive">error</Badge>)
  if (out.status === 'skipped') b.push(<Badge key="s" variant="outline">skipped</Badge>)
  if (out.notified) b.push(<Badge key="n" variant="secondary">sent</Badge>)
  if (out.status === 'success' && (out.kind === 'sql' || out.chart)) b.push(<Badge key="ok" variant="outline">{out.rowCount ?? out.rows?.length ?? 0} rows</Badge>)
  return b
}

// --- inline cell editor ------------------------------------------------------

interface CellEditorProps {
  cell: NotebookCell
  index: number
  total: number
  connections: { id: string; name: string }[]
  output?: NotebookCellResult
  chartSource?: NotebookCellResult
  running: boolean
  onPatch: (patch: Partial<NotebookCell>) => void
  onPatchChart: (patch: Partial<ChartSpec>) => void
  onRemove: () => void
  onMove: (dir: -1 | 1) => void
  onRun: () => void
  onAddBelow: () => void
}

function CellEditor({
  cell, index, total, connections, output, chartSource, running, onPatch, onPatchChart, onRemove, onMove, onRun, onAddBelow,
}: CellEditorProps) {
  const isSqlish = cell.kind === 'sql' || cell.kind === 'action'
  return (
    <div className="group relative rounded-lg border bg-card">
      {/* cell toolbar */}
      <div className="flex flex-wrap items-center gap-2 border-b px-2 py-1.5">
        <Select value={cell.kind} onValueChange={(v) => onPatch({ kind: v as CellKind })}>
          <SelectTrigger className="h-7 w-[140px] text-xs"><SelectValue /></SelectTrigger>
          <SelectContent>
            {CELL_KINDS.map((k) => (
              <SelectItem key={k.value} value={k.value}>{k.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          value={cell.title ?? ''}
          placeholder="Title (optional)"
          onChange={(e) => onPatch({ title: e.target.value })}
          className="h-7 flex-1 min-w-[120px] text-xs"
        />
        {isSqlish && (
          <Input
            value={cell.name ?? ''}
            placeholder="handle"
            onChange={(e) => onPatch({ name: e.target.value })}
            className="h-7 w-28 font-mono text-xs"
            title="Handle other cells reference: SELECT * FROM this_handle"
          />
        )}
        {isSqlish && (
          <Select value={cell.connectionId || undefined} onValueChange={(v) => onPatch({ connectionId: v })}>
            <SelectTrigger className="h-7 w-[160px] text-xs">
              <SelectValue placeholder={cell.kind === 'sql' ? 'Connection / compose' : 'Connection'} />
            </SelectTrigger>
            <SelectContent>
              {connections.length === 0 ? (
                <SelectItem value="__none" disabled>No connections</SelectItem>
              ) : (
                connections.map((c) => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)
              )}
            </SelectContent>
          </Select>
        )}
        <div className="ml-auto flex items-center gap-0.5">
          {statusBadges(output)}
          <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => onMove(-1)} disabled={index === 0} aria-label="Move up">
            <ChevronRight className="h-3.5 w-3.5 -rotate-90" />
          </Button>
          <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => onMove(1)} disabled={index === total - 1} aria-label="Move down">
            <ChevronRight className="h-3.5 w-3.5 rotate-90" />
          </Button>
          <Button size="sm" variant="ghost" className="h-7 px-2" onClick={onRun} disabled={running} title="Run cell (⌘/Ctrl+Enter)">
            <Play className="mr-1 h-3.5 w-3.5" />Run
          </Button>
          <Button size="icon" variant="ghost" className="h-7 w-7 text-muted-foreground hover:text-destructive" onClick={onRemove} aria-label="Delete cell">
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* cell body editor */}
      <div className="p-2">
        {isSqlish && (
          <div className="overflow-hidden rounded border">
            <CodeMirrorEditor
              value={cell.sql ?? ''}
              onChange={(v) => onPatch({ sql: v })}
              onExecute={onRun}
              height="140px"
              theme={isDark() ? 'dark' : 'light'}
              placeholder={cell.kind === 'action' ? 'UPDATE ... / DELETE ... ({{param}} to bind)' : 'SELECT ...  ({{param}}, or FROM <other_handle> to compose)'}
            />
          </div>
        )}
        {cell.kind === 'markdown' && (
          <Textarea
            value={cell.markdown ?? ''}
            placeholder="## Notes, rendered as markdown ({{param}} substituted)"
            onChange={(e) => onPatch({ markdown: e.target.value })}
            className="min-h-[70px]"
          />
        )}
        {cell.kind === 'notify' && (
          <div className="space-y-2">
            <Input value={cell.channel ?? ''} placeholder="Channel (e.g. #alerts)" onChange={(e) => onPatch({ channel: e.target.value })} className="h-8" />
            <Textarea value={cell.message ?? ''} placeholder="Message ({{param}} substituted)" onChange={(e) => onPatch({ message: e.target.value })} className="min-h-[50px]" />
          </div>
        )}
        {cell.kind === 'chart' && (
          <div className="grid grid-cols-2 gap-2">
            <Input value={cell.chart?.source ?? ''} placeholder="Source cell handle" onChange={(e) => onPatchChart({ source: e.target.value })} className="h-8 font-mono text-xs" />
            <Select value={cell.chart?.type || 'bar'} onValueChange={(v) => onPatchChart({ type: v })}>
              <SelectTrigger className="h-8"><SelectValue placeholder="Type" /></SelectTrigger>
              <SelectContent>
                {['bar', 'line', 'area', 'pie'].map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
              </SelectContent>
            </Select>
            <Input value={cell.chart?.x ?? ''} placeholder="X column" onChange={(e) => onPatchChart({ x: e.target.value })} className="h-8 font-mono text-xs" />
            <Input
              value={(cell.chart?.y ?? []).join(', ')}
              placeholder="Y columns (comma-separated)"
              onChange={(e) => onPatchChart({ y: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })}
              className="h-8 font-mono text-xs"
            />
          </div>
        )}

        {/* cell output */}
        {output && output.status !== 'preserved' && (
          <div className="mt-2 border-t pt-2">
            <CellOutput cell={output} source={chartSource} />
          </div>
        )}
      </div>

      {/* hover "add cell below" */}
      <div className="pointer-events-none absolute -bottom-3 left-0 right-0 flex justify-center opacity-0 transition-opacity group-hover:opacity-100">
        <Button size="sm" variant="outline" className="pointer-events-auto h-6 rounded-full px-2 text-[11px]" onClick={onAddBelow}>
          <Plus className="mr-1 h-3 w-3" />cell
        </Button>
      </div>
    </div>
  )
}

// --- panel -------------------------------------------------------------------

/**
 * NotebookPanel is an inline, Marimo/Jupyter-style notebook editor: the selected
 * notebook is edited in place — cells are authored, reordered, run, and removed
 * directly in the document (no modal). Edits autosave; running a cell saves first
 * so the engine runs the latest. ⌘/Ctrl+Enter runs a cell; ⌘/Ctrl+S saves.
 */
export function NotebookPanel() {
  const { notebooks, loading: listing, error: listError, refresh } = useNotebooks()
  const { definition, result, running, error, load, run } = useNotebookRun()
  const { connections } = useConnectionStore(useShallow((s) => ({ connections: s.connections })))

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [draft, setDraft] = useState<NotebookDefinition | null>(null)
  const [values, setValues] = useState<ParamValues>({})
  const [dryRun, setDryRun] = useState(true)
  const [autoApprove, setAutoApprove] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [showParams, setShowParams] = useState(false)
  // After we save, the assigned id flows into selectedId; skip the reload it would
  // otherwise trigger so in-progress edits aren't clobbered.
  const skipLoadRef = useRef<string | null>(null)
  // Always-fresh handle on the draft so the (stable) persist callback never reads
  // a stale closure.
  const draftRef = useRef<NotebookDefinition | null>(draft)
  draftRef.current = draft

  // Load when the user selects a different notebook from the sidebar.
  useEffect(() => {
    if (!selectedId) return
    if (skipLoadRef.current === selectedId) {
      skipLoadRef.current = null
      return
    }
    void load(selectedId)
  }, [selectedId, load])

  // Adopt a freshly loaded definition as the editable draft.
  useEffect(() => {
    if (!definition) return
    setDraft(definition)
    setValues(definition.inputs ? defaultValues(definition.inputs) : {})
    setDirty(false)
  }, [definition])

  const mutate = useCallback((fn: (d: NotebookDefinition) => NotebookDefinition) => {
    setDraft((prev) => (prev ? fn(prev) : prev))
    setDirty(true)
  }, [])

  const patchCell = (id: string, patch: Partial<NotebookCell>) =>
    mutate((d) => ({ ...d, cells: (d.cells ?? []).map((c) => (c.id === id ? { ...c, ...patch } : c)) }))
  const patchChart = (id: string, patch: Partial<ChartSpec>) =>
    mutate((d) => ({ ...d, cells: (d.cells ?? []).map((c) => (c.id === id ? { ...c, chart: { source: '', ...c.chart, ...patch } } : c)) }))
  const removeCell = (id: string) => mutate((d) => ({ ...d, cells: (d.cells ?? []).filter((c) => c.id !== id) }))
  const moveCell = (id: string, dir: -1 | 1) =>
    mutate((d) => {
      const cells = [...(d.cells ?? [])]
      const i = cells.findIndex((c) => c.id === id)
      const j = i + dir
      if (i < 0 || j < 0 || j >= cells.length) return d
      ;[cells[i], cells[j]] = [cells[j], cells[i]]
      return { ...d, cells }
    })
  const addCell = (kind: CellKind, atIndex?: number) =>
    mutate((d) => {
      const cells = [...(d.cells ?? [])]
      const cell = blankCell(kind)
      if (atIndex === undefined) cells.push(cell)
      else cells.splice(atIndex, 0, cell)
      return { ...d, cells }
    })
  const setInputs = (inputs: ParamInput[]) => mutate((d) => ({ ...d, inputs }))

  // Persist the draft. Returns the (possibly newly assigned) id, or null on error.
  const persist = useCallback(
    async (silent = false): Promise<string | null> => {
      const d = draftRef.current
      if (!d || !d.name?.trim()) {
        if (!silent) setSaveError('Notebook name is required.')
        return null
      }
      setSaving(true)
      setSaveError(null)
      try {
        const id = await saveNotebook(d)
        skipLoadRef.current = id
        setDraft((prev) => (prev ? { ...prev, id } : prev))
        setSelectedId(id)
        setDirty(false)
        void refresh()
        return id
      } catch (e) {
        if (!silent) setSaveError(e instanceof Error ? e.message : 'Failed to save')
        return null
      } finally {
        setSaving(false)
      }
    },
    [refresh],
  )

  // Debounced autosave on edits (silent: in-progress invalid states just stay dirty).
  useEffect(() => {
    if (!dirty || !draft?.name?.trim()) return
    const t = setTimeout(() => void persist(true), 1000)
    return () => clearTimeout(t)
  }, [dirty, draft, persist])

  // ⌘/Ctrl+S saves.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
        e.preventDefault()
        void persist()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [persist])

  const runAll = async () => {
    const id = dirty || !draft?.id ? await persist() : draft.id
    if (id) await run({ notebookId: id, inputs: values, dryRun, autoApprove })
  }
  const runCell = async (cellId: string) => {
    const id = dirty || !draft?.id ? await persist() : draft.id
    if (id) await run({ notebookId: id, inputs: values, dryRun, autoApprove, only: [cellId] })
  }

  const openNotebook = (id: string) => setSelectedId(id)
  const newNotebook = () => {
    skipLoadRef.current = null
    setSelectedId(null)
    setDraft(blankNotebook())
    setValues({})
    setDirty(false)
    setSaveError(null)
  }
  const deleteCurrent = async () => {
    if (draft?.id) {
      await apiDelete(draft.id)
      void refresh()
    }
    setDraft(null)
    setSelectedId(null)
  }

  const hasActions = useMemo(() => (draft?.cells ?? []).some((c) => c.kind === 'action'), [draft])
  const resultByCellId = useMemo(() => {
    const m = new Map<string, NotebookCellResult>()
    for (const c of result?.cells ?? []) m.set(c.cellId, c)
    return m
  }, [result])
  const resultByName = useMemo(() => {
    const m = new Map<string, NotebookCellResult>()
    for (const c of result?.cells ?? []) if (c.name) m.set(c.name, c)
    return m
  }, [result])

  const saveLabel = saving ? 'Saving…' : dirty ? 'Unsaved' : draft?.id ? 'Saved' : 'New'

  return (
    <div className="flex h-full min-h-0 gap-4 p-3">
      {/* sidebar */}
      <Card className="w-60 shrink-0 overflow-auto">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-sm">Notebooks</CardTitle>
          <Button size="sm" onClick={newNotebook}>
            <Plus className="mr-1 h-3.5 w-3.5" />New
          </Button>
        </CardHeader>
        <CardContent className="space-y-1">
          {listing && <p className="text-xs text-muted-foreground">Loading…</p>}
          {listError && <Alert variant="destructive"><AlertDescription>{listError}</AlertDescription></Alert>}
          {!listing && notebooks.length === 0 && <p className="text-xs text-muted-foreground">No notebooks yet.</p>}
          {notebooks.map((nb) => (
            <button
              key={nb.id}
              onClick={() => openNotebook(nb.id)}
              className={`block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted ${selectedId === nb.id ? 'bg-muted font-medium' : ''}`}
            >
              <div className="truncate">{nb.name}</div>
              {nb.lastRunStatus && <div className="text-[10px] text-muted-foreground">last run: {nb.lastRunStatus}</div>}
            </button>
          ))}
        </CardContent>
      </Card>

      {/* document */}
      <Card className="flex-1 overflow-auto">
        <CardContent className="space-y-4 p-4">
          {!draft && <p className="text-sm text-muted-foreground">Select a notebook, or create a new one.</p>}

          {draft && (
            <>
              {/* title + meta */}
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Input
                    value={draft.name}
                    onChange={(e) => mutate((d) => ({ ...d, name: e.target.value }))}
                    placeholder="Notebook name"
                    className="h-9 border-0 px-0 text-lg font-semibold shadow-none focus-visible:ring-0"
                  />
                  <Badge variant={dirty ? 'secondary' : 'outline'} className="shrink-0 text-[10px]">{saveLabel}</Badge>
                  <Button size="icon" variant="ghost" className="h-8 w-8 text-muted-foreground hover:text-destructive" onClick={deleteCurrent} aria-label="Delete notebook">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
                <Input
                  value={draft.description ?? ''}
                  onChange={(e) => mutate((d) => ({ ...d, description: e.target.value }))}
                  placeholder="Description (optional)"
                  className="h-7 border-0 px-0 text-sm text-muted-foreground shadow-none focus-visible:ring-0"
                />
              </div>

              {/* parameters (collapsible) */}
              <div className="rounded border">
                <button className="flex w-full items-center gap-1 px-2 py-1.5 text-xs font-medium" onClick={() => setShowParams((s) => !s)}>
                  {showParams ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                  Parameters {draft.inputs?.length ? `(${draft.inputs.length})` : ''}
                </button>
                {showParams && (
                  <div className="border-t p-2">
                    <ParamInputsEditor inputs={draft.inputs ?? []} onChange={setInputs} idPrefix="nb-input" />
                  </div>
                )}
              </div>

              {/* run controls */}
              <div className="flex flex-wrap items-center gap-3 rounded border bg-muted/30 p-2">
                <Button onClick={runAll} disabled={running} size="sm">
                  <Play className="mr-1.5 h-4 w-4" />{running ? 'Running…' : 'Run all'}
                </Button>
                <div className="flex items-center gap-1.5">
                  <Switch id="nb-dryrun" checked={dryRun} onCheckedChange={setDryRun} />
                  <Label htmlFor="nb-dryrun" className="text-xs">Dry run</Label>
                </div>
                {hasActions && !dryRun && (
                  <div className="flex items-center gap-1.5">
                    <Switch id="nb-approve" checked={autoApprove} onCheckedChange={setAutoApprove} />
                    <Label htmlFor="nb-approve" className="text-xs text-destructive">Auto-approve writes</Label>
                  </div>
                )}
                {(draft.inputs?.length ?? 0) > 0 && (
                  <div className="flex-1 min-w-[200px]">
                    <ParamForm inputs={draft.inputs ?? []} values={values} onChange={setValues} idPrefix="nb-val" />
                  </div>
                )}
                {result?.failed && <Badge variant="destructive">some cells failed</Badge>}
              </div>

              {(error || saveError) && (
                <Alert variant="destructive"><AlertDescription>{error || saveError}</AlertDescription></Alert>
              )}

              {/* cells */}
              <div className="space-y-5">
                {(draft.cells ?? []).map((cell, i) => (
                  <CellEditor
                    key={cell.id}
                    cell={cell}
                    index={i}
                    total={draft.cells?.length ?? 0}
                    connections={connections}
                    output={resultByCellId.get(cell.id)}
                    chartSource={cell.chart?.source ? resultByName.get(cell.chart.source) : undefined}
                    running={running}
                    onPatch={(patch) => patchCell(cell.id, patch)}
                    onPatchChart={(patch) => patchChart(cell.id, patch)}
                    onRemove={() => removeCell(cell.id)}
                    onMove={(dir) => moveCell(cell.id, dir)}
                    onRun={() => void runCell(cell.id)}
                    onAddBelow={() => addCell('sql', i + 1)}
                  />
                ))}
              </div>

              {/* add cell */}
              <div className="flex flex-wrap items-center gap-1.5 pt-1">
                <span className="text-xs text-muted-foreground">Add cell:</span>
                {CELL_KINDS.map((k) => (
                  <Button key={k.value} size="sm" variant="outline" className="h-7" onClick={() => addCell(k.value)}>
                    {k.value === 'sql' ? <Database className="mr-1 h-3.5 w-3.5" /> : <Plus className="mr-1 h-3.5 w-3.5" />}
                    {k.label}
                  </Button>
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
