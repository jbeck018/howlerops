import { Plus, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { ParamInput } from '@/lib/param-types'

/** Parameter types the run-time ParamForm knows how to render. */
const PARAM_TYPES = ['string', 'number', 'integer', 'boolean', 'date', 'enum', 'list'] as const

export interface ParamInputsEditorProps {
  inputs: ParamInput[]
  onChange: (next: ParamInput[]) => void
  idPrefix?: string
}

/**
 * ParamInputsEditor authors the typed parameter definitions a runbook or
 * notebook accepts. It is the design-time counterpart to ParamForm, which
 * renders the same definitions at run time. Enum types collect their choices as
 * a comma-separated list.
 */
export function ParamInputsEditor({ inputs, onChange, idPrefix = 'pi' }: ParamInputsEditorProps) {
  const update = (index: number, patch: Partial<ParamInput>) =>
    onChange(inputs.map((inp, i) => (i === index ? { ...inp, ...patch } : inp)))

  const add = () => onChange([...inputs, { name: '', type: 'string' }])
  const remove = (index: number) => onChange(inputs.filter((_, i) => i !== index))

  return (
    <div className="space-y-2">
      {inputs.length === 0 && (
        <p className="text-xs text-muted-foreground">No parameters. Add one to prompt for input at run time.</p>
      )}

      {inputs.map((input, i) => {
        const id = `${idPrefix}-${i}`
        return (
          <div key={i} className="space-y-2 rounded border p-2">
            <div className="flex items-center gap-2">
              <Input
                id={`${id}-name`}
                value={input.name}
                placeholder="name (used as {{name}} in SQL)"
                onChange={(e) => update(i, { name: e.target.value })}
                className="h-8 flex-1"
              />
              <Select value={input.type} onValueChange={(v) => update(i, { type: v })}>
                <SelectTrigger id={`${id}-type`} className="h-8 w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {PARAM_TYPES.map((t) => (
                    <SelectItem key={t} value={t}>
                      {t}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-8 w-8 flex-shrink-0"
                onClick={() => remove(i)}
                aria-label="Remove parameter"
              >
                <Trash2 className="h-4 w-4" />
              </Button>
            </div>

            <div className="flex items-center gap-2">
              <Input
                id={`${id}-label`}
                value={input.label ?? ''}
                placeholder="label (optional)"
                onChange={(e) => update(i, { label: e.target.value || undefined })}
                className="h-8 flex-1"
              />
              <Label
                htmlFor={`${id}-required`}
                className="flex items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground"
              >
                <Checkbox
                  id={`${id}-required`}
                  checked={Boolean(input.required)}
                  onCheckedChange={(c) => update(i, { required: c === true })}
                />
                Required
              </Label>
            </div>

            {input.type === 'enum' && (
              <Input
                id={`${id}-options`}
                value={(input.options ?? []).join(', ')}
                placeholder="enum options: a, b, c"
                onChange={(e) =>
                  update(i, {
                    options: e.target.value
                      .split(',')
                      .map((s) => s.trim())
                      .filter((s) => s.length > 0),
                  })
                }
                className="h-8"
              />
            )}
          </div>
        )
      })}

      <Button type="button" size="sm" variant="outline" onClick={add}>
        <Plus className="mr-1.5 h-3.5 w-3.5" />
        Add parameter
      </Button>
    </div>
  )
}
