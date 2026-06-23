// Shared types for typed parameter inputs (mirrors the Go params.Definition /
// InputDTO across the runbook, notebook, and alert surfaces).

export interface ParamInput {
  name: string
  type: string
  label?: string
  description?: string
  required?: boolean
  default?: unknown
  options?: string[]
}

export type ParamValues = Record<string, unknown>

/** Seed a values map from the inputs' defaults. */
export function defaultValues(inputs: ParamInput[]): ParamValues {
  const out: ParamValues = {}
  for (const input of inputs) {
    if (input.default !== undefined && input.default !== null) {
      out[input.name] = input.default
    }
  }
  return out
}
