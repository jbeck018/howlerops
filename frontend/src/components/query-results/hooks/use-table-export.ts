import { useCallback } from 'react'

import { toast } from '../../../hooks/use-toast'
import type { QueryResultRow } from '../../../store/query-store'
import type { EditableTableContext, ExportOptions } from '../../../types/table'
import { serialiseCsvValue } from '../utils'

interface UseTableExportOptions {
  connectionId?: string
  query: string
  columnNames: string[]
  tableContextRef: React.MutableRefObject<EditableTableContext | null>
  resolveCurrentRows: () => QueryResultRow[]
}

interface UseTableExportReturn {
  handleExport: (options: ExportOptions) => Promise<void>
}

// Build CSV/JSON content from row objects keyed by column name. Shared by the
// selected-rows export and the "loaded rows" fallback so both serialise
// identically.
function buildContentFromObjects(
  rows: QueryResultRow[],
  columns: string[],
  options: ExportOptions
): { filename: string; content: string } {
  const timestamp = Date.now()
  if (options.format === 'csv') {
    const header = options.includeHeaders ? columns.join(',') : ''
    const records = rows.map((row) =>
      columns.map((column) => serialiseCsvValue(row[column])).join(',')
    )
    return {
      filename: `query-results-${timestamp}.csv`,
      content: options.includeHeaders ? [header, ...records].join('\n') : records.join('\n'),
    }
  }
  return {
    filename: `query-results-${timestamp}.json`,
    content: JSON.stringify(rows, null, 2),
  }
}

export function useTableExport({
  connectionId,
  query,
  columnNames,
  tableContextRef,
  resolveCurrentRows,
}: UseTableExportOptions): UseTableExportReturn {
  const handleExport = useCallback(async (options: ExportOptions) => {
    if (!connectionId) {
      toast({
        title: 'Export failed',
        description: 'No active connection',
        variant: 'destructive',
      })
      return
    }

    // Fallback used when the live database session is gone (e.g. after a page
    // reload, where results persist but connections do not) and we can't
    // re-establish it: export the rows already loaded in the grid instead of
    // failing outright, making clear this is only the loaded subset.
    const exportLoadedRows = async () => {
      const rows = resolveCurrentRows()
      const { filename, content } = buildContentFromObjects(rows, columnNames, options)
      const { SaveToDownloads } = await import('../../../../bindings/github.com/jbeck018/howlerops/app')
      const filePath = await SaveToDownloads(filename, content)
      toast({
        title: 'Exported loaded rows',
        description: `Couldn't reach the database, so only the ${rows.length.toLocaleString()} loaded rows were exported to: ${filePath}. Reconnect and re-run the query to export everything.`,
        variant: 'default',
      })
    }

    try {
      // For selected rows only, export the current loaded data
      if (options.selectedOnly && tableContextRef.current?.state.selectedRows.length && tableContextRef.current.state.selectedRows.length > 0) {
        const currentRows = resolveCurrentRows()
        const selectedIds = tableContextRef.current.state.selectedRows
        const dataToExport = currentRows.filter(row => selectedIds.includes(row.__rowId!))

        const { filename, content } = buildContentFromObjects(dataToExport, columnNames, options)

        const { SaveToDownloads } = await import('../../../../bindings/github.com/jbeck018/howlerops/app')
        const filePath = await SaveToDownloads(filename, content)

        toast({
          title: 'Export successful',
          description: `File saved to: ${filePath}`,
          variant: 'default',
        })
        return
      }

      // For full export, re-query with isExport=true to get ALL rows
      toast({
        title: 'Export starting',
        description: 'Fetching all results from database...',
        variant: 'default',
      })

      // The full export re-queries the database, which needs a live session.
      // Sessions are dropped on reload while results persist, so re-establish
      // one first if it's missing. If we can't (no stored credentials, backend
      // unreachable), fall back to exporting the rows already on screen.
      const { useConnectionStore } = await import('../../../store/connection-store')
      const hasSession = () =>
        Boolean(useConnectionStore.getState().connections.find((c) => c.id === connectionId)?.sessionId)

      if (!hasSession()) {
        try {
          await useConnectionStore.getState().connectToDatabase(connectionId)
        } catch {
          // Reconnect failed; handled by the hasSession check below.
        }
        if (!hasSession()) {
          await exportLoadedRows()
          return
        }
      }

      const { executeQueryByConnectionId, queryResultRowsToMatrix } = await import('../../../lib/query-engine/runtime')
      const result = await executeQueryByConnectionId(
        connectionId,
        query,
        {
          limit: 0, // limit=0 triggers unlimited export (backend handles max 1M rows)
          offset: 0,
          timeout: 300, // 5 minute timeout
          isExport: true,
        }
      )

      // Prepare export data
      const exportRows = queryResultRowsToMatrix(result)
      const exportColumns = result.columns || []

      // Show warning if hitting max export limit (1M rows)
      if (exportRows.length >= 1000000) {
        toast({
          title: 'Export limit reached',
          description: 'Export limited to 1 million rows. Consider filtering your query.',
          variant: 'default',
        })
      }

      const timestamp = Date.now()
      let filename: string
      let content: string

      if (options.format === 'csv') {
        filename = `query-results-${timestamp}.csv`
        const header = options.includeHeaders ? exportColumns.join(',') : ''
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Backend returns rows as any[] arrays from Wails
        const records = exportRows.map((row: any[]) =>
          row.map((cell) => serialiseCsvValue(cell)).join(',')
        )
        content = options.includeHeaders ? [header, ...records].join('\n') : records.join('\n')
      } else {
        filename = `query-results-${timestamp}.json`
        // Convert rows array to objects for JSON export
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Backend returns rows as any[] arrays and cells as any from Wails
        const jsonData = exportRows.map((row: any[]) => {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Cell values are unknown types from database
          const obj: Record<string, any> = {}
          exportColumns.forEach((col: string, idx: number) => {
            obj[col] = row[idx]
          })
          return obj
        })
        content = JSON.stringify(jsonData, null, 2)
      }

      const { SaveToDownloads } = await import('../../../../bindings/github.com/jbeck018/howlerops/app')
      const filePath = await SaveToDownloads(filename, content)

      toast({
        title: 'Export successful',
        description: `${exportRows.length.toLocaleString()} rows saved to: ${filePath}`,
        variant: 'default',
      })
    } catch (error) {
      console.error('Failed to export:', error)

      toast({
        title: 'Export failed',
        description: error instanceof Error ? error.message : 'Failed to export data',
        variant: 'destructive',
      })
    }
  }, [connectionId, query, columnNames, resolveCurrentRows, tableContextRef])

  return {
    handleExport,
  }
}
