// Runbook API client.
//
// The Wails bindings (SaveRunbook/RunRunbook/…) are generated at desktop build
// time, so — as with insight-api — they are accessed through a typed indirection
// that keeps `tsc` green before regeneration and surfaces a clear error in a
// stale build.

import type { ParamInput, ParamValues } from '@/lib/param-types'

export type StepKind = 'query' | 'action' | 'notify'

export interface RunbookStep {
  id: string
  name?: string
  kind: StepKind
  dependsOn?: string[]
  connectionId?: string
  sql?: string
  channel?: string
  message?: string
}

export interface RunbookDefinition {
  id?: string
  name: string
  description?: string
  inputs?: ParamInput[]
  steps?: RunbookStep[]
}

export interface RunbookSummary {
  id: string
  name: string
  description: string
  lastRunAt?: string
  lastRunStatus?: string
  updatedAt: string
}

export interface RunbookOutcome {
  stepId: string
  name?: string
  kind?: string
  status: string
  sql?: string
  rowsAffected?: number
  message?: string
  notified?: boolean
  planned?: boolean
  error?: string
  skipped?: string
}

export interface RunbookRunResult {
  failed: boolean
  dryRun: boolean
  outcomes: RunbookOutcome[]
}

export interface RunbookRunRequest {
  runbookId: string
  inputs: ParamValues
  dryRun?: boolean
  autoApprove?: boolean
}

export interface RunbookRunRecord {
  id: string
  runbookId: string
  startedAt: string
  finishedAt?: string
  status: string
  dryRun: boolean
}

type RunbookBindings = {
  SaveRunbook?: (def: RunbookDefinition) => Promise<string>
  ListRunbooks?: () => Promise<RunbookSummary[] | null>
  GetRunbook?: (id: string) => Promise<RunbookDefinition | null>
  DeleteRunbook?: (id: string) => Promise<void>
  RunRunbook?: (req: RunbookRunRequest) => Promise<RunbookRunResult | null>
  RunbookHistory?: (id: string, limit: number) => Promise<RunbookRunRecord[] | null>
}

async function bindings(): Promise<RunbookBindings> {
  return (await import(
    '../../bindings/github.com/jbeck018/howlerops/app'
  )) as unknown as RunbookBindings
}

function unavailable(): never {
  throw new Error('Runbooks are unavailable in this build. Rebuild the desktop app to regenerate bindings.')
}

export async function saveRunbook(def: RunbookDefinition): Promise<string> {
  const mod = await bindings()
  if (!mod.SaveRunbook) unavailable()
  return mod.SaveRunbook(def)
}

export async function listRunbooks(): Promise<RunbookSummary[]> {
  const mod = await bindings()
  if (!mod.ListRunbooks) unavailable()
  return (await mod.ListRunbooks()) ?? []
}

export async function getRunbook(id: string): Promise<RunbookDefinition> {
  const mod = await bindings()
  if (!mod.GetRunbook) unavailable()
  const def = await mod.GetRunbook(id)
  if (!def) throw new Error('Runbook not found')
  return def
}

export async function deleteRunbook(id: string): Promise<void> {
  const mod = await bindings()
  if (!mod.DeleteRunbook) unavailable()
  return mod.DeleteRunbook(id)
}

export async function runRunbook(req: RunbookRunRequest): Promise<RunbookRunResult> {
  const mod = await bindings()
  if (!mod.RunRunbook) unavailable()
  const res = await mod.RunRunbook(req)
  if (!res) throw new Error('No run result returned')
  return res
}

export async function runbookHistory(id: string, limit = 20): Promise<RunbookRunRecord[]> {
  const mod = await bindings()
  if (!mod.RunbookHistory) unavailable()
  return (await mod.RunbookHistory(id, limit)) ?? []
}
