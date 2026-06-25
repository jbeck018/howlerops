// Notebook API client (typed indirection over the Wails bindings).
//
// A notebook is the unified, reactive, cell-based document: markdown, sql
// (read), action (write, gated), notify, and chart cells that form a dependency
// DAG. Cells reference each other by handle (name); the backend stages each SQL
// cell's result so downstream cells compose (the DuckDB-UI / marimo model).

import type { ParamInput, ParamValues } from '@/lib/param-types'

export type CellKind = 'markdown' | 'sql' | 'action' | 'notify' | 'chart'

export interface ChartSpec {
  source: string
  type?: string // bar | line | area | pie | scatter
  x?: string
  y?: string[]
  stacked?: boolean
}

export interface NotebookCell {
  id: string
  name?: string // stable handle used for cross-cell references
  title?: string
  kind: CellKind
  dependsOn?: string[]
  connectionId?: string
  sql?: string
  markdown?: string
  channel?: string
  message?: string
  chart?: ChartSpec
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
  lastRunAt?: string
  lastRunStatus?: string
  updatedAt: string
}

export interface NotebookCellResult {
  cellId: string
  name?: string
  title?: string
  kind: string
  status: string // success | error | skipped | preserved
  markdown?: string
  sql?: string
  columns?: string[]
  rows?: Record<string, unknown>[]
  rowCount?: number
  affected?: number // rows affected (action cells)
  message?: string // rendered notify message
  notified?: boolean
  planned?: boolean // dry-run: would have executed
  chart?: ChartSpec
  error?: string
  skipped?: string
}

export interface NotebookRunResult {
  failed: boolean
  dryRun: boolean
  cells: NotebookCellResult[]
}

export interface NotebookRunRequest {
  notebookId: string
  inputs: ParamValues
  stopOnError?: boolean
  /** Plan writes/notifications without performing them. */
  dryRun?: boolean
  /** Permit action (write) cells without an interactive prompt. */
  autoApprove?: boolean
  /** Reactive re-run: restrict to these cell IDs plus their descendants. */
  only?: string[]
}

export interface NotebookRunRecord {
  id: string
  notebookId: string
  startedAt: string
  finishedAt?: string
  status: string
  dryRun: boolean
}

type NotebookBindings = {
  SaveNotebook?: (def: NotebookDefinition) => Promise<string>
  ListNotebooks?: () => Promise<NotebookSummary[] | null>
  GetNotebook?: (id: string) => Promise<NotebookDefinition | null>
  DeleteNotebook?: (id: string) => Promise<void>
  RunNotebook?: (req: NotebookRunRequest) => Promise<NotebookRunResult | null>
  NotebookHistory?: (id: string, limit: number) => Promise<NotebookRunRecord[] | null>
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

export async function notebookHistory(id: string, limit = 20): Promise<NotebookRunRecord[]> {
  const mod = await bindings()
  if (!mod.NotebookHistory) return []
  return (await mod.NotebookHistory(id, limit)) ?? []
}
