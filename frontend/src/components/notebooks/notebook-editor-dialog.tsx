import { ArrowDown, ArrowUp, Database, FileText, Plus, Trash2 } from 'lucide-react'
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
import { type NotebookCell, type NotebookDefinition, saveNotebook } from '@/lib/notebook-api'
import type { ParamInput } from '@/lib/param-types'
import { useConnectionStore } from '@/store/connection-store'

let cellSeq = 0
function newCellId(): string {
  cellSeq += 1
  return `cell-${Date.now().toString(36)}-${cellSeq}`
}

function blankCell(kind: NotebookCell['kind']): NotebookCell {
  return { id: newCellId(), kind, title: '', sql: '', markdown: '', connectionId: '' }
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
 * parameters, and an ordered list of markdown / SQL cells — and persists it via
 * SaveNotebook. It backs both "New notebook" and "Edit" in the NotebookPanel.
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
        title: c.title ?? '',
        sql: c.sql ?? '',
        markdown: c.markdown ?? '',
        connectionId: c.connectionId ?? '',
      })),
    )
    setError(null)
  }, [open, initial])

  const updateCell = (index: number, patch: Partial<NotebookCell>) =>
    setCells((prev) => prev.map((c, i) => (i === index ? { ...c, ...patch } : c)))

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
    for (const c of cells) {
      if (c.kind === 'sql') {
        if (!c.connectionId) {
          setError('Every SQL cell needs a connection.')
          return
        }
        if (!c.sql?.trim()) {
          setError('Every SQL cell needs a query.')
          return
        }
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
        title: c.title?.trim() || undefined,
        connectionId: c.kind === 'sql' ? c.connectionId : undefined,
        sql: c.kind === 'sql' ? c.sql : undefined,
        markdown: c.kind === 'markdown' ? c.markdown : undefined,
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit notebook' : 'New notebook'}</DialogTitle>
          <DialogDescription>
            A notebook runs its cells top to bottom. SQL cells run read-only against a connection.
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
              <div className="flex gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setCells((prev) => [...prev, blankCell('markdown')])}
                >
                  <FileText className="mr-1.5 h-3.5 w-3.5" />
                  Markdown
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setCells((prev) => [...prev, blankCell('sql')])}
                >
                  <Database className="mr-1.5 h-3.5 w-3.5" />
                  SQL
                </Button>
              </div>
            </div>

            {cells.length === 0 && (
              <p className="text-xs text-muted-foreground">No cells yet. Add a markdown or SQL cell.</p>
            )}

            {cells.map((cell, i) => (
              <div key={cell.id} className="space-y-2 rounded border p-2">
                <div className="flex items-center gap-2">
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase text-muted-foreground">
                    {cell.kind}
                  </span>
                  <Input
                    value={cell.title ?? ''}
                    placeholder="Cell title (optional)"
                    onChange={(e) => updateCell(i, { title: e.target.value })}
                    className="h-8 flex-1"
                  />
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => moveCell(i, -1)}
                    disabled={i === 0}
                    aria-label="Move cell up"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => moveCell(i, 1)}
                    disabled={i === cells.length - 1}
                    aria-label="Move cell down"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => removeCell(i)}
                    aria-label="Remove cell"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>

                {cell.kind === 'sql' ? (
                  <>
                    <Select
                      value={cell.connectionId || undefined}
                      onValueChange={(v) => updateCell(i, { connectionId: v })}
                    >
                      <SelectTrigger className="h-8">
                        <SelectValue placeholder="Select a connection" />
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
                      placeholder="SELECT ... (use {{param}} to bind parameters)"
                      onChange={(e) => updateCell(i, { sql: e.target.value })}
                      className="min-h-[80px] font-mono text-xs"
                    />
                  </>
                ) : (
                  <Textarea
                    value={cell.markdown ?? ''}
                    placeholder="## Notes, rendered as markdown"
                    onChange={(e) => updateCell(i, { markdown: e.target.value })}
                    className="min-h-[60px]"
                  />
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
