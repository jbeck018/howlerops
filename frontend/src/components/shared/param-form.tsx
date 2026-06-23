import { memo } from 'react'

import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import type { ParamInput, ParamValues } from '@/lib/param-types'

export interface ParamFormProps {
  inputs: ParamInput[]
  values: ParamValues
  onChange: (values: ParamValues) => void
  idPrefix?: string
}

/**
 * ParamForm renders a control per typed parameter definition (string, number,
 * boolean, date, enum, list) and reports value changes. Shared by the runbook
 * and notebook run dialogs.
 */
export const ParamForm = memo(function ParamForm({ inputs, values, onChange, idPrefix = 'param' }: ParamFormProps) {
  if (inputs.length === 0) {
    return <p className="text-xs text-muted-foreground">This item has no parameters.</p>
  }

  const set = (name: string, value: unknown) => onChange({ ...values, [name]: value })

  return (
    <div className="space-y-3">
      {inputs.map((input) => {
        const id = `${idPrefix}-${input.name}`
        const value = values[input.name]
        return (
          <div key={input.name} className="space-y-1">
            <Label htmlFor={id} className="text-xs font-medium">
              {input.label || input.name}
              {input.required && <span className="ml-0.5 text-destructive">*</span>}
            </Label>
            {renderControl(id, input, value, set)}
            {input.description && (
              <p className="text-xs text-muted-foreground">{input.description}</p>
            )}
          </div>
        )
      })}
    </div>
  )
})

function renderControl(
  id: string,
  input: ParamInput,
  value: unknown,
  set: (name: string, value: unknown) => void,
) {
  const type = input.type.toLowerCase()

  if (type === 'boolean') {
    return (
      <div>
        <Switch id={id} checked={Boolean(value)} onCheckedChange={(c) => set(input.name, c)} />
      </div>
    )
  }

  if (type === 'enum' && input.options && input.options.length > 0) {
    return (
      <Select value={value != null ? String(value) : undefined} onValueChange={(v) => set(input.name, v)}>
        <SelectTrigger id={id}>
          <SelectValue placeholder="Select…" />
        </SelectTrigger>
        <SelectContent>
          {input.options.map((opt) => (
            <SelectItem key={opt} value={opt}>
              {opt}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    )
  }

  if (type === 'number' || type === 'integer') {
    // Show '' for NaN so React keeps the input controlled without rendering "NaN".
    const numText = typeof value === 'number' && Number.isNaN(value) ? '' : value != null ? String(value) : ''
    return (
      <Input
        id={id}
        type="number"
        value={numText}
        step={type === 'integer' ? 1 : 'any'}
        onChange={(e) => {
          const raw = e.target.value
          if (raw === '') {
            set(input.name, undefined)
            return
          }
          const num = Number(raw)
          // Ignore intermediate/invalid input (e.g. "-", "1e") rather than storing NaN.
          set(input.name, Number.isNaN(num) ? undefined : num)
        }}
      />
    )
  }

  if (type === 'date' || type === 'timestamp') {
    return (
      <Input
        id={id}
        type="date"
        value={value != null ? String(value) : ''}
        onChange={(e) => set(input.name, e.target.value || undefined)}
      />
    )
  }

  if (type === 'list') {
    // Comma-separated entry; stored as an array for the IN-clause binding.
    const joined = Array.isArray(value) ? (value as unknown[]).join(', ') : value != null ? String(value) : ''
    return (
      <Input
        id={id}
        value={joined}
        placeholder="comma, separated, values"
        onChange={(e) => {
          const parts = e.target.value
            .split(',')
            .map((s) => s.trim())
            .filter((s) => s.length > 0)
          set(input.name, parts)
        }}
      />
    )
  }

  // string / enum-without-options / identifier / fallback
  return (
    <Input
      id={id}
      value={value != null ? String(value) : ''}
      onChange={(e) => set(input.name, e.target.value)}
    />
  )
}
