// Notebook API client (typed indirection over the Wails bindings).

import type { ParamInput, ParamValues } from '@/lib/param-types'

export type CellKind = 'sql' | 'markdown'

export interface NotebookCell {
  id: string
  title?: string
  kind: CellKind
  connectionId?: string
  sql?: string
  markdown?: string
}

export interface NotebookDefinition {
  id?: string
  name: string
  description?: string
  inputs?: ParamInput[]
  cells?: NotebookCell[]
}

export interface NotebookSummary {
  id: string
  name: string
  description: string
  updatedAt: string
}

export interface NotebookCellResult {
  cellId: string
  title?: string
  kind: string
  status: string
  markdown?: string
  sql?: string
  columns?: string[]
  rows?: Record<string, unknown>[]
  rowCount?: number
  error?: string
}

export interface NotebookRunResult {
  failed: boolean
  cells: NotebookCellResult[]
}

export interface NotebookRunRequest {
  notebookId: string
  inputs: ParamValues
  stopOnError?: boolean
}

type NotebookBindings = {
  SaveNotebook?: (def: NotebookDefinition) => Promise<string>
  ListNotebooks?: () => Promise<NotebookSummary[] | null>
  GetNotebook?: (id: string) => Promise<NotebookDefinition | null>
  DeleteNotebook?: (id: string) => Promise<void>
  RunNotebook?: (req: NotebookRunRequest) => Promise<NotebookRunResult | null>
}

async function bindings(): Promise<NotebookBindings> {
  return (await import(
    '../../bindings/github.com/jbeck018/howlerops/app'
  )) as unknown as NotebookBindings
}

function unavailable(): never {
  throw new Error('Notebooks are unavailable in this build. Rebuild the desktop app to regenerate bindings.')
}

export async function saveNotebook(def: NotebookDefinition): Promise<string> {
  const mod = await bindings()
  if (!mod.SaveNotebook) unavailable()
  return mod.SaveNotebook(def)
}

export async function listNotebooks(): Promise<NotebookSummary[]> {
  const mod = await bindings()
  if (!mod.ListNotebooks) unavailable()
  return (await mod.ListNotebooks()) ?? []
}

export async function getNotebook(id: string): Promise<NotebookDefinition> {
  const mod = await bindings()
  if (!mod.GetNotebook) unavailable()
  const def = await mod.GetNotebook(id)
  if (!def) throw new Error('Notebook not found')
  return def
}

export async function deleteNotebook(id: string): Promise<void> {
  const mod = await bindings()
  if (!mod.DeleteNotebook) unavailable()
  return mod.DeleteNotebook(id)
}

export async function runNotebook(req: NotebookRunRequest): Promise<NotebookRunResult> {
  const mod = await bindings()
  if (!mod.RunNotebook) unavailable()
  const res = await mod.RunNotebook(req)
  if (!res) throw new Error('No run result returned')
  return res
}
