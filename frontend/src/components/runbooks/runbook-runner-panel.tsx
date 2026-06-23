import { AlertTriangle, Play, ShieldCheck } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { ParamForm } from '@/components/shared/param-form'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useRunbookRun, useRunbooks } from '@/hooks/use-runbooks'
import { defaultValues, type ParamValues } from '@/lib/param-types'
import type { RunbookOutcome } from '@/lib/runbook-api'

function statusVariant(status: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'success':
      return 'default'
    case 'failed':
    case 'error':
      return 'destructive'
    case 'skipped':
      return 'secondary'
    default:
      return 'outline'
  }
}

function OutcomeRow({ outcome }: { outcome: RunbookOutcome }) {
  return (
    <div className="rounded border p-2 text-xs">
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{outcome.name || outcome.stepId}</span>
        <div className="flex items-center gap-1">
          {outcome.planned && <Badge variant="outline">planned</Badge>}
          <Badge variant={statusVariant(outcome.status)}>{outcome.status}</Badge>
        </div>
      </div>
      {outcome.sql && <pre className="mt-1 overflow-x-auto whitespace-pre-wrap text-muted-foreground">{outcome.sql}</pre>}
      {outcome.message && <p className="mt-1 text-muted-foreground">{outcome.message}</p>}
      {typeof outcome.rowsAffected === 'number' && outcome.rowsAffected > 0 && (
        <p className="mt-1 text-muted-foreground">{outcome.rowsAffected} rows affected</p>
      )}
      {outcome.error && <p className="mt-1 text-destructive">{outcome.error}</p>}
      {outcome.skipped && <p className="mt-1 text-muted-foreground">Skipped: {outcome.skipped}</p>}
    </div>
  )
}

/**
 * RunbookRunnerPanel lists saved runbooks and lets the user fill a runbook's
 * parameter form and run it — as a dry run (default, safe) or for real with
 * explicit approval of writes.
 */
export function RunbookRunnerPanel() {
  const { runbooks, loading: listing, error: listError, refresh } = useRunbooks()
  const { definition, result, loading, running, error, load, run, reset } = useRunbookRun()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [values, setValues] = useState<ParamValues>({})
  const [dryRun, setDryRun] = useState(true)
  const [autoApprove, setAutoApprove] = useState(false)

  // Use a single hook instance for load+run.
  useEffect(() => {
    if (selectedId) {
      void load(selectedId)
    } else {
      reset()
    }
  }, [selectedId, load, reset])

  useEffect(() => {
    // Reseed values whenever the loaded definition changes; clear stale values
    // when the new runbook has no inputs so we never submit a prior runbook's data.
    setValues(definition?.inputs ? defaultValues(definition.inputs) : {})
  }, [definition])

  const hasWrites = useMemo(
    () => (definition?.steps ?? []).some((s) => s.kind === 'action'),
    [definition],
  )

  const onRun = () => {
    if (!selectedId) return
    void run({ runbookId: selectedId, inputs: values, dryRun, autoApprove: !dryRun && autoApprove })
  }

  return (
    <div className="flex h-full min-h-0 gap-4 p-3">
      {/* List */}
      <Card className="w-64 shrink-0 overflow-auto">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-sm">Runbooks</CardTitle>
          <Button size="sm" variant="ghost" onClick={() => void refresh()}>
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="space-y-1">
          {listing && <p className="text-xs text-muted-foreground">Loading…</p>}
          {listError && <Alert variant="destructive"><AlertDescription>{listError}</AlertDescription></Alert>}
          {!listing && runbooks.length === 0 && (
            <p className="text-xs text-muted-foreground">No runbooks yet.</p>
          )}
          {runbooks.map((rb) => (
            <button
              key={rb.id}
              onClick={() => setSelectedId(rb.id)}
              className={`block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted ${
                selectedId === rb.id ? 'bg-muted font-medium' : ''
              }`}
            >
              {rb.name}
              {rb.lastRunStatus && (
                <span className="ml-1 text-xs text-muted-foreground">· {rb.lastRunStatus}</span>
              )}
            </button>
          ))}
        </CardContent>
      </Card>

      {/* Detail */}
      <Card className="flex-1 overflow-auto">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{definition?.name ?? 'Select a runbook'}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading && <p className="text-xs text-muted-foreground">Loading definition…</p>}
          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

          {definition && (
            <>
              {definition.description && (
                <p className="text-sm text-muted-foreground">{definition.description}</p>
              )}

              <ParamForm inputs={definition.inputs ?? []} values={values} onChange={setValues} idPrefix="rb" />

              <div className="space-y-2 rounded border p-3">
                <div className="flex items-center justify-between">
                  <Label htmlFor="rb-dryrun" className="flex items-center gap-1.5 text-xs">
                    <ShieldCheck className="h-3.5 w-3.5" /> Dry run (preview, no writes)
                  </Label>
                  <Switch id="rb-dryrun" checked={dryRun} onCheckedChange={setDryRun} />
                </div>
                {!dryRun && hasWrites && (
                  <div className="flex items-center justify-between">
                    <Label htmlFor="rb-approve" className="flex items-center gap-1.5 text-xs text-amber-600">
                      <AlertTriangle className="h-3.5 w-3.5" /> Approve write actions
                    </Label>
                    <Switch id="rb-approve" checked={autoApprove} onCheckedChange={setAutoApprove} />
                  </div>
                )}
              </div>

              <Button onClick={onRun} disabled={running}>
                <Play className="mr-1.5 h-4 w-4" />
                {running ? 'Running…' : dryRun ? 'Dry Run' : 'Run'}
              </Button>

              {result && (
                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-sm font-medium">
                    Result
                    <Badge variant={result.failed ? 'destructive' : 'default'}>
                      {result.failed ? 'failed' : 'ok'}
                    </Badge>
                    {result.dryRun && <Badge variant="outline">dry run</Badge>}
                  </div>
                  {result.outcomes.map((oc) => (
                    <OutcomeRow key={oc.stepId} outcome={oc} />
                  ))}
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
