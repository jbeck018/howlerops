import { ArrowDown, ArrowUp, Bell, Database, PencilLine, Plus, Trash2 } from 'lucide-react'
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
import type { ParamInput } from '@/lib/param-types'
import { type RunbookDefinition, type RunbookStep, saveRunbook } from '@/lib/runbook-api'
import { useConnectionStore } from '@/store/connection-store'

let stepSeq = 0
function newStepId(): string {
  stepSeq += 1
  return `step-${Date.now().toString(36)}-${stepSeq}`
}

function blankStep(kind: RunbookStep['kind']): RunbookStep {
  return { id: newStepId(), kind, name: '', sql: '', connectionId: '', channel: '', message: '' }
}

const STEP_LABEL: Record<RunbookStep['kind'], string> = {
  query: 'Query (read-only)',
  action: 'Action (write)',
  notify: 'Notify',
}

export interface RunbookEditorDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When provided, the dialog edits this runbook; otherwise it creates a new one. */
  initial?: RunbookDefinition | null
  /** Called with the saved runbook's ID after a successful save. */
  onSaved: (id: string) => void
}

/**
 * RunbookEditorDialog authors a runbook definition — name, description, typed
 * parameters, and an ordered list of query / action / notify steps — and
 * persists it via SaveRunbook. It backs both "New runbook" and "Edit" in the
 * RunbookRunnerPanel.
 */
export function RunbookEditorDialog({ open, onOpenChange, initial, onSaved }: RunbookEditorDialogProps) {
  const { connections } = useConnectionStore(useShallow((s) => ({ connections: s.connections })))

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [inputs, setInputs] = useState<ParamInput[]>([])
  const [steps, setSteps] = useState<RunbookStep[]>([])
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  // Reseed the form each time the dialog opens so a prior session never leaks in.
  useEffect(() => {
    if (!open) return
    setName(initial?.name ?? '')
    setDescription(initial?.description ?? '')
    setInputs(initial?.inputs ?? [])
    setSteps(
      (initial?.steps ?? []).map((s) => ({
        id: s.id || newStepId(),
        kind: s.kind,
        name: s.name ?? '',
        sql: s.sql ?? '',
        connectionId: s.connectionId ?? '',
        channel: s.channel ?? '',
        message: s.message ?? '',
      })),
    )
    setError(null)
  }, [open, initial])

  const updateStep = (index: number, patch: Partial<RunbookStep>) =>
    setSteps((prev) => prev.map((s, i) => (i === index ? { ...s, ...patch } : s)))

  const removeStep = (index: number) => setSteps((prev) => prev.filter((_, i) => i !== index))

  const moveStep = (index: number, dir: -1 | 1) =>
    setSteps((prev) => {
      const next = [...prev]
      const target = index + dir
      if (target < 0 || target >= next.length) return prev
      ;[next[index], next[target]] = [next[target], next[index]]
      return next
    })

  const handleSave = async () => {
    const trimmedName = name.trim()
    if (!trimmedName) {
      setError('Runbook name is required.')
      return
    }
    for (const s of steps) {
      if (s.kind === 'notify') {
        if (!s.message?.trim()) {
          setError('Every notify step needs a message.')
          return
        }
      } else {
        if (!s.connectionId) {
          setError('Every query/action step needs a connection.')
          return
        }
        if (!s.sql?.trim()) {
          setError('Every query/action step needs SQL.')
          return
        }
      }
    }

    const def: RunbookDefinition = {
      id: initial?.id,
      name: trimmedName,
      description: description.trim() || undefined,
      inputs: inputs.length > 0 ? inputs : undefined,
      steps: steps.map((s) => ({
        id: s.id,
        kind: s.kind,
        name: s.name?.trim() || undefined,
        connectionId: s.kind === 'notify' ? undefined : s.connectionId,
        sql: s.kind === 'notify' ? undefined : s.sql,
        channel: s.kind === 'notify' ? s.channel?.trim() || undefined : undefined,
        message: s.kind === 'notify' ? s.message : undefined,
      })),
    }

    setSaving(true)
    setError(null)
    try {
      const id = await saveRunbook(def)
      onSaved(id)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save runbook')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit runbook' : 'New runbook'}</DialogTitle>
          <DialogDescription>
            A runbook runs its steps in order. Query steps are read-only; action steps may write and require approval.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1">
            <Label htmlFor="rb-name" className="text-xs font-medium">
              Name<span className="ml-0.5 text-destructive">*</span>
            </Label>
            <Input id="rb-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Rotate API keys" />
          </div>

          <div className="space-y-1">
            <Label htmlFor="rb-desc" className="text-xs font-medium">
              Description
            </Label>
            <Textarea
              id="rb-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What this runbook does (optional)"
              className="min-h-[60px]"
            />
          </div>

          <div className="space-y-2">
            <Label className="text-xs font-medium">Parameters</Label>
            <ParamInputsEditor inputs={inputs} onChange={setInputs} idPrefix="rb-input" />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label className="text-xs font-medium">Steps</Label>
              <div className="flex gap-1">
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setSteps((prev) => [...prev, blankStep('query')])}
                >
                  <Database className="mr-1.5 h-3.5 w-3.5" />
                  Query
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setSteps((prev) => [...prev, blankStep('action')])}
                >
                  <PencilLine className="mr-1.5 h-3.5 w-3.5" />
                  Action
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setSteps((prev) => [...prev, blankStep('notify')])}
                >
                  <Bell className="mr-1.5 h-3.5 w-3.5" />
                  Notify
                </Button>
              </div>
            </div>

            {steps.length === 0 && (
              <p className="text-xs text-muted-foreground">No steps yet. Add a query, action, or notify step.</p>
            )}

            {steps.map((step, i) => (
              <div key={step.id} className="space-y-2 rounded border p-2">
                <div className="flex items-center gap-2">
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium uppercase text-muted-foreground">
                    {STEP_LABEL[step.kind]}
                  </span>
                  <Input
                    value={step.name ?? ''}
                    placeholder="Step name (optional)"
                    onChange={(e) => updateStep(i, { name: e.target.value })}
                    className="h-8 flex-1"
                  />
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => moveStep(i, -1)}
                    disabled={i === 0}
                    aria-label="Move step up"
                  >
                    <ArrowUp className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => moveStep(i, 1)}
                    disabled={i === steps.length - 1}
                    aria-label="Move step down"
                  >
                    <ArrowDown className="h-4 w-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => removeStep(i)}
                    aria-label="Remove step"
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>

                {step.kind === 'notify' ? (
                  <>
                    <Input
                      value={step.channel ?? ''}
                      placeholder="Channel (optional, e.g. #ops)"
                      onChange={(e) => updateStep(i, { channel: e.target.value })}
                      className="h-8"
                    />
                    <Textarea
                      value={step.message ?? ''}
                      placeholder="Message (use {{param}} to bind parameters)"
                      onChange={(e) => updateStep(i, { message: e.target.value })}
                      className="min-h-[60px]"
                    />
                  </>
                ) : (
                  <>
                    <Select
                      value={step.connectionId || undefined}
                      onValueChange={(v) => updateStep(i, { connectionId: v })}
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
                      value={step.sql ?? ''}
                      placeholder={
                        step.kind === 'action'
                          ? 'UPDATE ... (write — requires approval at run time)'
                          : 'SELECT ... (read-only)'
                      }
                      onChange={(e) => updateStep(i, { sql: e.target.value })}
                      className="min-h-[80px] font-mono text-xs"
                    />
                  </>
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
            {saving ? 'Saving…' : initial ? 'Save changes' : 'Create runbook'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
