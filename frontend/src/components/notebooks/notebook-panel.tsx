import { Play } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'

import { ParamForm } from '@/components/shared/param-form'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useNotebookRun, useNotebooks } from '@/hooks/use-notebook'
import { type NotebookCellResult } from '@/lib/notebook-api'
import { defaultValues, type ParamValues } from '@/lib/param-types'

function CellOutput({ cell }: { cell: NotebookCellResult }) {
  if (cell.kind === 'markdown') {
    return <div className="whitespace-pre-wrap text-sm text-foreground">{cell.markdown}</div>
  }
  if (cell.status === 'error') {
    return (
      <Alert variant="destructive">
        <AlertDescription>{cell.error}</AlertDescription>
      </Alert>
    )
  }
  if (cell.status === 'skipped') {
    return <p className="text-xs text-muted-foreground">Skipped.</p>
  }
  const columns = cell.columns ?? []
  const rows = (cell.rows ?? []).slice(0, 50)
  return (
    <div className="space-y-1">
      {cell.sql && <pre className="overflow-x-auto whitespace-pre-wrap text-xs text-muted-foreground">{cell.sql}</pre>}
      {columns.length > 0 ? (
        <div className="max-h-64 overflow-auto rounded border">
          <Table>
            <TableHeader>
              <TableRow>
                {columns.map((c) => (
                  <TableHead key={c}>{c}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row, i) => (
                <TableRow key={i}>
                  {columns.map((c) => (
                    <TableCell key={c}>{formatCell(row[c])}</TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">No rows.</p>
      )}
    </div>
  )
}

function formatCell(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

/**
 * NotebookPanel lists saved notebooks and runs the selected one, rendering each
 * cell's output (markdown or a result table) in order.
 */
export function NotebookPanel() {
  const { notebooks, loading: listing, error: listError, refresh } = useNotebooks()
  const { definition, result, loading, running, error, load, run } = useNotebookRun()

  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [values, setValues] = useState<ParamValues>({})

  useEffect(() => {
    if (selectedId) void load(selectedId)
  }, [selectedId, load])

  useEffect(() => {
    if (definition?.inputs) setValues(defaultValues(definition.inputs))
  }, [definition])

  const cellTitles = useMemo(() => {
    const map: Record<string, string> = {}
    for (const c of definition?.cells ?? []) map[c.id] = c.title || c.id
    return map
  }, [definition])

  return (
    <div className="flex h-full min-h-0 gap-4 p-3">
      <Card className="w-64 shrink-0 overflow-auto">
        <CardHeader className="flex flex-row items-center justify-between pb-2">
          <CardTitle className="text-sm">Notebooks</CardTitle>
          <Button size="sm" variant="ghost" onClick={() => void refresh()}>
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="space-y-1">
          {listing && <p className="text-xs text-muted-foreground">Loading…</p>}
          {listError && <Alert variant="destructive"><AlertDescription>{listError}</AlertDescription></Alert>}
          {!listing && notebooks.length === 0 && <p className="text-xs text-muted-foreground">No notebooks yet.</p>}
          {notebooks.map((nb) => (
            <button
              key={nb.id}
              onClick={() => setSelectedId(nb.id)}
              className={`block w-full rounded px-2 py-1 text-left text-sm hover:bg-muted ${
                selectedId === nb.id ? 'bg-muted font-medium' : ''
              }`}
            >
              {nb.name}
            </button>
          ))}
        </CardContent>
      </Card>

      <Card className="flex-1 overflow-auto">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{definition?.name ?? 'Select a notebook'}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {loading && <p className="text-xs text-muted-foreground">Loading…</p>}
          {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}

          {definition && (
            <>
              <ParamForm inputs={definition.inputs ?? []} values={values} onChange={setValues} idPrefix="nb" />
              <Button
                onClick={() => selectedId && void run({ notebookId: selectedId, inputs: values })}
                disabled={running}
              >
                <Play className="mr-1.5 h-4 w-4" />
                {running ? 'Running…' : 'Run notebook'}
              </Button>

              {result && (
                <div className="space-y-3">
                  {result.failed && (
                    <Badge variant="destructive">some cells failed</Badge>
                  )}
                  {result.cells.map((cell) => (
                    <div key={cell.cellId} className="space-y-1">
                      <div className="text-xs font-medium text-muted-foreground">
                        {cellTitles[cell.cellId] || cell.cellId}
                      </div>
                      <CellOutput cell={cell} />
                    </div>
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
