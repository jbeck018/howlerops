import { ArrowDown, ArrowUp, BarChart3, Bell, Database, FileText, Plus, Trash2, Wrench } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'

import { ParamInputsEditor } from '@/components/shared/param-inputs-editor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { type ChartSpec, type NotebookCell, type NotebookDefinition, saveNotebook } from '@/lib/notebook-api'
import type { ParamInput } from '@/lib/param-types'
import { useConnectionStore } from '@/store/connection-store'

let cellSeq = 0
function newCellId(): string {
  cellSeq += 1
  return `cell-${Date.now().toString(36)}-${cellSeq}`
}

function blankCell(kind: NotebookCell['kind']): NotebookCell {
  return { id: newCellId(), kind, title: '', name: '', sql: '', markdown: '', connectionId: '', channel: '', message: '' }
}

export interface NotebookEditorDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When provided, the dialog edits this notebook; otherwise it creates a new one. */
  initial?: NotebookDefinition | null
  /** Called with the saved notebook's ID after a successful save. */
  onSaved: (id: string) => void
}

/**
 * NotebookEditorDialog authors a notebook definition — name, description, typed
 * parameters, and a set of cells (markdown / sql / action / notify / chart) — and
 * persists it via SaveNotebook. SQL/action cells can carry a handle (name) so
 * other cells reference their result (`SELECT ... FROM <handle>`). It backs both
 * "New notebook" and "Edit" in the NotebookPanel.
 */
export function NotebookEditorDialog({ open, onOpenChange, initial, onSaved }: NotebookEditorDialogProps) {
  const { connections } = useConnectionStore(useShallow((s) => ({ connections: s.connections })))

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [inputs, setInputs] = useState<ParamInput[]>([])
  const [cells, setCells] = useState<NotebookCell[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // Reseed the form each time the dialog opens so a prior session never leaks in.
  useEffect(() => {
    if (!open) return
    setName(initial?.name ?? '')
    setDescription(initial?.description ?? '')
    setInputs(initial?.inputs ?? [])
    setCells(
      (initial?.cells ?? []).map((c) => ({
        id: c.id || newCellId(),
        kind: c.kind,
        name: c.name ?? '',
        title: c.title ?? '',
        sql: c.sql ?? '',
        markdown: c.markdown ?? '',
        connectionId: c.connectionId ?? '',
        channel: c.channel ?? '',
        message: c.message ?? '',
        chart: c.chart,
      })),
    )
    setError(null)
  }, [open, initial])

  const updateCell = (index: number, patch: Partial<NotebookCell>) =>
    setCells((prev) => prev.map((c, i) => (i === index ? { ...c, ...patch } : c)))

  const updateChart = (index: number, patch: Partial<ChartSpec>) =>
    setCells((prev) =>
      prev.map((c, i) => (i === index ? { ...c, chart: { source: '', ...c.chart, ...patch } } : c)),
    )

  const removeCell = (index: number) => setCells((prev) => prev.filter((_, i) => i !== index))

  const moveCell = (index: number, dir: -1 | 1) =>
    setCells((prev) => {
      const next = [...prev]
      const target = index + dir
      if (target < 0 || target >= next.length) return prev
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })

  const handleSave = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) {
      setError('Notebook name is required.')
      return
    }
    const handleRe = /^[A-Za-z_][A-Za-z0-9_]*$/
    for (const c of cells) {
      if (c.name && !handleRe.test(c.name)) {
        setError(`Handle "${c.name}" must be a valid identifier (letters, digits, underscore).`)
        return
      }
      if (c.kind === 'sql' || c.kind === 'action') {
        if (!c.sql?.trim()) {
          setError(`Every ${c.kind} cell needs a query.`)
          return
        }
        // A read cell that references another cell composes on DuckDB and needs no
        // connection; otherwise a connection is required.
        const composes = c.kind === 'sql' && cells.some((o) => o.name && o.name !== c.name && new RegExp(`\\b${o.name}\\b`).test(c.sql ?? ''))
        if (c.kind === 'action' && !c.connectionId) {
          setError('Every action (write) cell needs a connection.')
          return
        }
        if (c.kind === 'sql' && !c.connectionId && !composes) {
          setError('A SQL cell needs a connection (unless it references another cell).')
          return
        }
      }
      if (c.kind === 'notify' && !c.message?.trim()) {
        setError('Every notify cell needs a message.')
        return
      }
      if (c.kind === 'chart' && !c.chart?.source?.trim()) {
        setError('Every chart cell needs a source cell handle.')
        return
      }
    }

    const def: NotebookDefinition = {
      id: initial?.id,
      name: trimmedName,
      description: description.trim() || undefined,
      inputs: inputs.length > 0 ? inputs : undefined,
      cells: cells.map((c) => ({
        id: c.id,
        kind: c.kind,
        name: c.name?.trim() || undefined,
        title: c.title?.trim() || undefined,
        connectionId: c.kind === 'sql' || c.kind === 'action' ? c.connectionId || undefined : undefined,
        sql: c.kind === 'sql' || c.kind === 'action' ? c.sql : undefined,
        markdown: c.kind === 'markdown' ? c.markdown : undefined,
        channel: c.kind === 'notify' ? c.channel || undefined : undefined,
        message: c.kind === 'notify' ? c.message : undefined,
        chart: c.kind === 'chart' ? c.chart : undefined,
      })),
    }

    setSaving(true)
    setError(null)
    try {
      const id = await saveNotebook(def)
      onSaved(id)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save notebook')
    } finally {
      setSaving(false)
    }
  }

  const addBtns: { kind: NotebookCell['kind']; label: string; icon: typeof Database }[] = [
    { kind: 'markdown', label: 'Markdown', icon: FileText },
    { kind: 'sql', label: 'SQL', icon: Database },
    { kind: 'action', label: 'Action', icon: Wrench },
    { kind: 'notify', label: 'Notify', icon: Bell },
    { kind: 'chart', label: 'Chart', icon: BarChart3 },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit notebook' : 'New notebook'}</DialogTitle>
          <DialogDescription>
            Cells form a reactive graph. Give a SQL cell a handle and other cells can query its result by name
            (composed on DuckDB). Action cells write and are gated by the dry-run / approval guardrail.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="nb-name" className="text-xs font-medium">
              Name<span className="ml-0.5 text-destructive">*</span>
            </Label>
            <Input id="nb-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Daily checks" />
          </div>

          <div className="space-y-1">
            <Label htmlFor="nb-desc" className="text-xs font-medium">
              Description
            </Label>
            <Textarea
              id="nb-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What this notebook is for (optional)"
              className="min-h-[60px]"
            />
          </div>

          <div className="space-y-2">
            <Label className="text-xs font-medium">Parameters</Label>
            <ParamInputsEditor inputs={inputs} onChange={setInputs} idPrefix="nb-input" />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium">Cells</Label>
              <div className="flex flex-wrap gap-1">
                {addBtns.map(({ kind, label, icon: Icon }) => (
                  <Button
                    key={kind}
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setCells((prev) => [...prev, blankCell(kind)])}
                  >
                    <Icon className="mr-1.5 h-3.5 w-3.5" />
                    {label}
                  </Button>
                ))}
              </div>
            </div>

            {cells.length === 0 && <p className="text-xs text-muted-foreground">No cells yet. Add one above.</p>}

            {cells.map((cell, i) => (
              <div key={cell.id} className="space-y-2 rounded border p-2">
                <div className="flex items-center gap-2">
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase text-muted-foreground">
                    {cell.kind}
                  </span>
                  <Input
                    value={cell.title ?? ''}
                    placeholder="Title (optional)"
                    onChange={(e) => updateCell(i, { title: e.target.value })}
                    className="h-8 flex-1"
                  />
                  {(cell.kind === 'sql' || cell.kind === 'action') && (
                    <Input
                      value={cell.name ?? ''}
                      placeholder="handle"
                      onChange={(e) => updateCell(i, { name: e.target.value })}
                      className="h-8 w-28 font-mono text-xs"
                      title="Handle other cells reference (e.g. SELECT * FROM this_handle)"
                    />
                  )}
                  <Button type="button" size="icon" variant="ghost" className="h-8 w-8" onClick={() => moveCell(i, -1)} disabled={i === 0} aria-label="Move cell up">
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button type="button" size="icon" variant="ghost" className="h-8 w-8" onClick={() => moveCell(i, 1)} disabled={i === cells.length - 1} aria-label="Move cell down">
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button type="button" size="icon" variant="ghost" className="h-8 w-8" onClick={() => removeCell(i)} aria-label="Remove cell">
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>

                {(cell.kind === 'sql' || cell.kind === 'action') && (
                  <>
                    <Select value={cell.connectionId || undefined} onValueChange={(v) => updateCell(i, { connectionId: v })}>
                      <SelectTrigger className="h-8">
                        <SelectValue placeholder={cell.kind === 'sql' ? 'Connection (or leave empty to compose other cells)' : 'Connection'} />
                      </SelectTrigger>
                      <SelectContent>
                        {connections.length === 0 ? (
                          <SelectItem value="__none" disabled>
                            No connections
                          </SelectItem>
                        ) : (
                          connections.map((conn) => (
                            <SelectItem key={conn.id} value={conn.id}>
                              {conn.name}
                            </SelectItem>
                          ))
                        )}
                      </SelectContent>
                    </Select>
                    <Textarea
                      value={cell.sql ?? ''}
                      placeholder={
                        cell.kind === 'action'
                          ? 'UPDATE ... / DELETE ... (use {{param}} to bind)'
                          : 'SELECT ... (use {{param}}, or FROM <other_handle> to compose)'
                      }
                      onChange={(e) => updateCell(i, { sql: e.target.value })}
                      className="min-h-[80px] font-mono text-xs"
                    />
                  </>
                )}

                {cell.kind === 'markdown' && (
                  <Textarea
                    value={cell.markdown ?? ''}
                    placeholder="## Notes, rendered as markdown ({{param}} is substituted)"
                    onChange={(e) => updateCell(i, { markdown: e.target.value })}
                    className="min-h-[60px]"
                  />
                )}

                {cell.kind === 'notify' && (
                  <>
                    <Input
                      value={cell.channel ?? ''}
                      placeholder="Channel (e.g. #alerts)"
                      onChange={(e) => updateCell(i, { channel: e.target.value })}
                      className="h-8"
                    />
                    <Textarea
                      value={cell.message ?? ''}
                      placeholder="Message ({{param}} is substituted)"
                      onChange={(e) => updateCell(i, { message: e.target.value })}
                      className="min-h-[50px]"
                    />
                  </>
                )}

                {cell.kind === 'chart' && (
                  <div className="grid grid-cols-2 gap-2">
                    <Input
                      value={cell.chart?.source ?? ''}
                      placeholder="Source cell handle"
                      onChange={(e) => updateChart(i, { source: e.target.value })}
                      className="h-8 font-mono text-xs"
                    />
                    <Select value={cell.chart?.type || 'bar'} onValueChange={(v) => updateChart(i, { type: v })}>
                      <SelectTrigger className="h-8">
                        <SelectValue placeholder="Type" />
                      </SelectTrigger>
                      <SelectContent>
                        {['bar', 'line', 'area', 'pie'].map((t) => (
                          <SelectItem key={t} value={t}>
                            {t}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Input
                      value={cell.chart?.x ?? ''}
                      placeholder="X column"
                      onChange={(e) => updateChart(i, { x: e.target.value })}
                      className="h-8 font-mono text-xs"
                    />
                    <Input
                      value={(cell.chart?.y ?? []).join(', ')}
                      placeholder="Y columns (comma-separated)"
                      onChange={(e) => updateChart(i, { y: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })}
                      className="h-8 font-mono text-xs"
                    />
                  </div>
                )}
              </div>
            ))}
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            Cancel
          </Button>
          <Button onClick={handleSave} disabled={saving}>
            <Plus className="mr-1.5 h-4 w-4" />
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Create notebook'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
