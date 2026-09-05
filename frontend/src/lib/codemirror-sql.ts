/**
 * CodeMirror 6 SQL Extension with Multi-Database Support
 *
 * Provides syntax highlighting, autocomplete, and language support
 * for both single-database and multi-database queries with @connection.table syntax
 */

import {
  acceptCompletion,
  autocompletion,
  Completion,
  CompletionContext,
  CompletionResult,
  startCompletion} from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap, indentMore } from '@codemirror/commands'
import { sql, SQLDialect } from '@codemirror/lang-sql'
import { searchKeymap } from '@codemirror/search'
import { EditorState, Extension, RangeSetBuilder, StateEffect, StateField } from '@codemirror/state'
import { oneDark } from '@codemirror/theme-one-dark'
import { Decoration, DecorationSet, EditorView, keymap, ViewPlugin, ViewUpdate } from '@codemirror/view'

import type { QueryContext, TableReference } from './sql-context-parser'
import { getTablesInScope,isAlias, parseQueryContext, resolveAlias } from './sql-context-parser'

export interface Connection {
  id: string
  name: string
  type: string
  database: string
  sessionId?: string
  isConnected?: boolean
  alias?: string
}

export interface SchemaNode {
  name: string
  type: 'database' | 'schema' | 'table' | 'column'
  children?: SchemaNode[]
  dataType?: string
  nullable?: boolean
  primaryKey?: boolean
  sessionId?: string
}

export interface Column {
  name: string
  dataType: string
  nullable?: boolean
  primaryKey?: boolean
}

export type ColumnLoader = (sessionId: string, schema: string, table: string) => Promise<Column[]>

// State effect to update schema data
export const updateSchemaEffect = StateEffect.define<{
  connections: Connection[]
  schemas: Map<string, SchemaNode[]>
  mode: 'single' | 'multi'
  isLoading?: boolean
}>()

// State field to hold schema data
export const schemaState = StateField.define<{
  connections: Connection[]
  schemas: Map<string, SchemaNode[]>
  mode: 'single' | 'multi'
  columnCache: Map<string, Column[]>
  isLoading: boolean
}>({
  create() {
    return {
      connections: [],
      schemas: new Map(),
      mode: 'single',
      columnCache: new Map(),
      isLoading: false
    }
  },
  update(value, tr) {
    for (const effect of tr.effects) {
      if (effect.is(updateSchemaEffect)) {
        return {
          ...value,
          connections: effect.value.connections,
          schemas: effect.value.schemas,
          mode: effect.value.mode,
          isLoading: effect.value.isLoading ?? value.isLoading
        }
      }
    }
    return value
  }
})

/**
 * SQL Keywords for autocomplete
 */
const SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'OUTER',
  'ON', 'AS', 'AND', 'OR', 'NOT', 'IN', 'EXISTS', 'LIKE', 'BETWEEN',
  'ORDER', 'BY', 'GROUP', 'HAVING', 'LIMIT', 'OFFSET', 'UNION', 'ALL',
  'DISTINCT', 'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE',
  'CREATE', 'TABLE', 'DROP', 'ALTER', 'INDEX', 'VIEW', 'WITH',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'NULL', 'IS', 'CAST',
  'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'COALESCE', 'NULLIF'
]

/**
 * Find connection by name or ID (case-insensitive on name/alias, so @prod
 * resolves a connection named "Prod").
 */
function findConnection(connections: Connection[], identifier: string): Connection | undefined {
  const exact = connections.find(c =>
    c.name === identifier ||
    c.id === identifier ||
    c.alias === identifier
  )
  if (exact) return exact

  const lower = identifier.toLowerCase()
  return connections.find(c =>
    c.name?.toLowerCase() === lower ||
    c.alias?.toLowerCase() === lower
  )
}

/**
 * Look up a connection's schema list by id/name, case-insensitively.
 */
function getConnectionSchemas(
  schemas: Map<string, SchemaNode[]>,
  identifier: string
): SchemaNode[] | undefined {
  const exact = schemas.get(identifier)
  if (exact) return exact

  const lower = identifier.toLowerCase()
  for (const [key, value] of schemas) {
    if (key.toLowerCase() === lower) return value
  }
  return undefined
}

/**
 * The identifier a column should be qualified with when it is ambiguous:
 * the explicit alias when the query declares one, otherwise the table name.
 */
function tableQualifier(tableRef: TableReference): string {
  return tableRef.alias || tableRef.tableName
}

/**
 * Check if user is typing a SQL keyword
 */
function isTypingSQLKeyword(word: string): boolean {
  if (!word || word.length < 2) return false
  const upperWord = word.toUpperCase()
  return SQL_KEYWORDS.some(kw => kw.startsWith(upperWord))
}

// ================================================================
// Smart Autocomplete Helpers - JOIN ON and Enhanced Ranking
// ================================================================

/**
 * Context for JOIN ON clause to enable smart FK suggestions
 */
interface JoinOnContext {
  leftAlias: string
  leftColumn: string
  rightAlias: string
}

/**
 * Detect if cursor is in a JOIN ON clause and extract context
 * Example: "ON a.user_id = u." → { leftAlias: 'a', leftColumn: 'user_id', rightAlias: 'u' }
 */
function detectJoinOnContext(textBeforeCursor: string): JoinOnContext | null {
  // Pattern: ON alias.column = alias.
  const joinOnPattern = /ON\s+(\w+)\.(\w+)\s*=\s*(\w+)\.(\w*)$/i
  const match = textBeforeCursor.match(joinOnPattern)

  if (match) {
    const [, leftAlias, leftColumn, rightAlias] = match
    return { leftAlias, leftColumn, rightAlias }
  }

  return null
}

/**
 * Check if a column matches foreign key pattern and return boost value
 * Higher values = better match
 */
function matchesForeignKeyPattern(
  leftColumn: string,
  rightColumn: string,
  isRightPrimaryKey: boolean
): number {
  const leftLower = leftColumn.toLowerCase()
  const rightLower = rightColumn.toLowerCase()

  // Exact match (e.g., user_id = user_id)
  if (leftLower === rightLower) {
    return 10
  }

  // Left ends with _id, right is 'id' and is PK (most common FK pattern)
  // e.g., user_id = id (where id is PK)
  if (leftLower.endsWith('_id') && rightLower === 'id' && isRightPrimaryKey) {
    return 15
  }

  // Extract base name from left column (user_id → user)
  const leftBase = leftLower.replace(/_id$/, '').replace(/_uuid$/, '').replace(/_guid$/, '')

  // Right column contains the base name
  // e.g., user_id → user_id, user_pk, user_uuid
  if (leftBase && rightLower.includes(leftBase)) {
    return 12
  }

  // Left contains base of right
  // e.g., account_user_id → user_id
  const rightBase = rightLower.replace(/_id$/, '').replace(/_uuid$/, '').replace(/_guid$/, '')
  if (rightBase && leftLower.includes(rightBase)) {
    return 10
  }

  // Right is PK and left contains 'id' (weak correlation)
  if (isRightPrimaryKey && leftLower.includes('id')) {
    return 5
  }

  return 0
}

/**
 * Get boost for common/utility columns
 */
function getCommonColumnBoost(columnName: string): number {
  const lower = columnName.toLowerCase()

  // Primary identifiers
  if (['id', 'uuid', 'guid'].includes(lower)) {
    return 10
  }

  // Audit trail columns
  if (['created_at', 'created_date', 'created_time', 'updated_at', 'updated_date', 'updated_time', 'modified_at'].includes(lower)) {
    return 5
  }

  // Soft delete
  if (lower === 'deleted_at') {
    return 5
  }

  // Audit user tracking
  if (['created_by', 'updated_by', 'modified_by'].includes(lower)) {
    return 3
  }

  // Foreign key patterns
  if (lower.endsWith('_id') || lower.endsWith('_uuid') || lower.endsWith('_guid')) {
    return 3
  }

  // Status columns
  if (['status', 'state', 'is_active', 'enabled', 'disabled', 'active'].includes(lower)) {
    return 3
  }

  // Name/description columns
  if (['name', 'title', 'description', 'label', 'email'].includes(lower)) {
    return 3
  }

  // Timestamp columns
  if (lower.includes('_at') || lower.includes('_date') || lower.includes('_time')) {
    return 2
  }

  return 0
}

/**
 * Calculate smart boost value for a column based on context
 */
function calculateColumnBoost(
  column: Column,
  context: QueryContext,
  joinOnContext?: JoinOnContext | null,
  baseBoost: number = 80
): number {
  let boost = baseBoost

  // 1. Primary key boost
  if (column.primaryKey) {
    boost += 10
  }

  // 2. Common column patterns
  boost += getCommonColumnBoost(column.name)

  // 3. JOIN ON context - boost FK matches
  if (joinOnContext && context.currentClause === 'ON') {
    const fkMatchBoost = matchesForeignKeyPattern(
      joinOnContext.leftColumn,
      column.name,
      column.primaryKey || false
    )
    boost += fkMatchBoost
  }

  // 4. Clause-specific boosts
  if (context.currentClause === 'SELECT') {
    // Slightly boost commonly selected columns
    const commonSelects = ['id', 'name', 'title', 'email', 'username']
    if (commonSelects.includes(column.name.toLowerCase())) {
      boost += 2
    }
  }

  return boost
}

/**
 * Create SQL autocomplete extension
 */
export function sqlAutocompletion(columnLoader?: ColumnLoader): Extension {
  return autocompletion({
    override: [
      async (context: CompletionContext): Promise<CompletionResult | null> => {
        const state = context.state.field(schemaState, false)
        if (!state) return null

        const { connections, schemas, mode, columnCache } = state
        const word = context.matchBefore(/[@\w.-]*/)
        if (!word) return null

        const textBeforeCursor = context.state.doc.sliceString(
          Math.max(0, context.pos - 200),
          context.pos
        )

        const options: Completion[] = []

        // ===================================================================
        // INTELLIGENT CONTEXT-AWARE AUTOCOMPLETE
        // ===================================================================

        // Parse query context for intelligent suggestions
        const fullQuery = context.state.doc.toString()
        const queryContext = parseQueryContext(fullQuery, context.pos)

        // Detect JOIN ON context for smart FK suggestions
        const joinOnContext = detectJoinOnContext(textBeforeCursor)

        // Check for alias prefix (e.g., "a." or "u.")
        const aliasPattern = /(\w+)\.(\w*)$/
        const aliasMatch = textBeforeCursor.match(aliasPattern)

        if (aliasMatch && columnLoader) {
          const [, prefix, partialColumn] = aliasMatch

          // Check if prefix is an alias
          if (isAlias(prefix, queryContext)) {
            const tableRef = resolveAlias(prefix, queryContext)

            if (tableRef) {
              // For multi-DB, find the connection
              let connection: Connection | undefined
              let sessionId: string | undefined

              if (tableRef.isMultiDB && tableRef.connectionId) {
                connection = findConnection(connections, tableRef.connectionId)
                sessionId = connection?.sessionId
              } else {
                // Single-DB mode or non-multi-DB table
                connection = connections[0]
                sessionId = connection?.sessionId
              }

              if (sessionId) {
                // Determine schema name
                const schemaName = tableRef.schema || 'public'

                const cacheKey = `${sessionId}-${schemaName}-${tableRef.tableName}`
                let columns: Column[]

                if (columnCache.has(cacheKey)) {
                  columns = columnCache.get(cacheKey)!
                } else {
                  try {
                    columns = await columnLoader(sessionId, schemaName, tableRef.tableName)
                    columnCache.set(cacheKey, columns)
                  } catch (error) {
                    console.error('[CodeMirror SQL] Failed to load columns:', error)
                    columns = []
                  }
                }

                // Add column suggestions with smart boosting
                columns
                  .filter(col => !partialColumn || col.name.toLowerCase().startsWith(partialColumn.toLowerCase()))
                  .forEach(col => {
                    const boost = calculateColumnBoost(col, queryContext, joinOnContext, 90)
                    options.push({
                      label: col.name,
                      type: 'property',
                      detail: col.dataType,
                      info: col.nullable ? 'Nullable' : 'Not null',
                      boost
                    })
                  })

                if (options.length > 0) {
                  return {
                    from: context.pos - partialColumn.length,
                    options
                  }
                }
              }
            }
          }
        }

        // Context-aware suggestions (no prefix) - suggest columns from tables in scope
        // Only in WHERE, SELECT, HAVING clauses
        if (['WHERE', 'SELECT', 'HAVING', 'ON', 'ORDER_BY', 'GROUP_BY'].includes(queryContext.currentClause)) {
          const tablesInScope = getTablesInScope(queryContext)

          // Only show context-aware if we're not already in an alias. pattern
          if (tablesInScope.length > 0 && !aliasMatch && columnLoader) {
            // Check if we're typing a column name (no table prefix)
            const plainWordPattern = /\b(\w+)$/
            const plainWordMatch = textBeforeCursor.match(plainWordPattern)

            if (plainWordMatch) {
              const partialWord = plainWordMatch[1]

              // Suggest columns as soon as one character is typed (in any of the
              // supported clauses).
              if (partialWord.length >= 1) {

                // Load columns from every table in scope first, so we know which
                // names are genuinely ambiguous before deciding how to label them.
                const scopedColumns: { tableRef: TableReference; column: Column }[] = []

                for (const tableRef of tablesInScope) {
                  let connection: Connection | undefined
                  let sessionId: string | undefined

                  if (tableRef.isMultiDB && tableRef.connectionId) {
                    connection = findConnection(connections, tableRef.connectionId)
                    sessionId = connection?.sessionId
                  } else {
                    connection = connections[0]
                    sessionId = connection?.sessionId
                  }

                  if (!sessionId) continue

                  const schemaName = tableRef.schema || 'public'
                  const cacheKey = `${sessionId}-${schemaName}-${tableRef.tableName}`
                  let columns: Column[]

                  if (columnCache.has(cacheKey)) {
                    columns = columnCache.get(cacheKey)!
                  } else {
                    try {
                      columns = await columnLoader(sessionId, schemaName, tableRef.tableName)
                      columnCache.set(cacheKey, columns)
                    } catch (error) {
                      console.error('[CodeMirror SQL] Failed to load columns for context:', error)
                      columns = []
                    }
                  }

                  columns
                    .filter(col => col.name.toLowerCase().startsWith(partialWord.toLowerCase()))
                    .forEach(column => scopedColumns.push({ tableRef, column }))
                }

                // A column name is ambiguous only when more than one in-scope
                // table exposes it. Everything else is inserted bare — the user
                // typed no qualifier, so we must not invent one.
                const owners = new Map<string, Set<string>>()
                scopedColumns.forEach(({ tableRef, column }) => {
                  const key = column.name.toLowerCase()
                  const owner = tableQualifier(tableRef)
                  const existing = owners.get(key)
                  if (existing) {
                    existing.add(owner)
                  } else {
                    owners.set(key, new Set([owner]))
                  }
                })

                const seenLabels = new Set<string>()

                scopedColumns.forEach(({ tableRef, column }) => {
                  const qualifier = tableQualifier(tableRef)
                  const ambiguous = (owners.get(column.name.toLowerCase())?.size ?? 0) > 1

                  // Same column from the same table can surface twice when a
                  // query mentions a table more than once — keep one entry.
                  const dedupeKey = ambiguous
                    ? `${qualifier}.${column.name}`.toLowerCase()
                    : column.name.toLowerCase()
                  if (seenLabels.has(dedupeKey)) return
                  seenLabels.add(dedupeKey)

                  const boost = calculateColumnBoost(column, queryContext, joinOnContext, 80)

                  options.push({
                    // The label is always the bare column name so it still
                    // matches what the user is typing; only an ambiguous column
                    // gets table-qualified, and only on insert via `apply`.
                    label: column.name,
                    apply: ambiguous ? `${qualifier}.${column.name}` : undefined,
                    type: 'property',
                    detail: ambiguous ? `${qualifier} · ${column.dataType}` : column.dataType,
                    info: tablesInScope.length > 1
                      ? `From ${qualifier}${column.nullable ? ' (Nullable)' : ''}`
                      : (column.nullable ? 'Nullable' : 'Not null'),
                    boost
                  })
                })

                if (options.length > 0) {
                  return {
                    from: context.pos - partialWord.length,
                    options
                  }
                }
              }
            }
          }
        }

        // ===================================================================
        // ORIGINAL MULTI-DB MODE PATTERNS (fallback)
        // ===================================================================

        // Multi-DB Mode Patterns
        if (mode === 'multi') {
          // Pattern 1: User typed '@' - show connections. Disconnected ones are
          // still offered (so you can write the query) but ranked below the
          // connected ones and flagged as disconnected.
          if (textBeforeCursor.endsWith('@')) {
            connections.forEach((conn, idx) => {
              const connected = conn.isConnected
              options.push({
                label: `@${conn.name}`,
                type: 'namespace',
                detail: connected ? `${conn.type} · ${conn.database}` : `${conn.type} · disconnected`,
                info: connected
                  ? `Connection: ${conn.name}`
                  : `Connection: ${conn.name} — not connected (will connect when used)`,
                // Connected connections sort first; disconnected ones below.
                boost: (connected ? 100 : 30) - idx,
              })
            })

            return {
              from: context.pos - 1,
              options,
              validFor: /@\w*/
            }
          }

          // Pattern 2: @connection.schema.table. - show columns
          const columnPattern = /@([\w-]+)\.([\w-]+)\.([\w-]+)\.(\w*)$/
          const columnMatch = textBeforeCursor.match(columnPattern)

          if (columnMatch && columnLoader) {
            const [, connId, schemaName, tableName, partialColumn] = columnMatch
            const connection = findConnection(connections, connId)

            if (connection?.sessionId) {
              const cacheKey = `${connection.sessionId}-${schemaName}-${tableName}`
              let columns: Column[]

              if (columnCache.has(cacheKey)) {
                columns = columnCache.get(cacheKey)!
              } else {
                try {
                  columns = await columnLoader(connection.sessionId, schemaName, tableName)
                  columnCache.set(cacheKey, columns)
                } catch {
                  columns = []
                }
              }

              columns
                .filter(col => !partialColumn || col.name.toLowerCase().startsWith(partialColumn.toLowerCase()))
                .forEach(col => {
                  options.push({
                    label: col.name,
                    type: 'property',
                    detail: col.dataType,
                    info: col.nullable ? 'Nullable' : 'Not null',
                    boost: 90
                  })
                })

              return {
                from: context.pos - partialColumn.length,
                options
              }
            }
          }

          // Pattern 2b: @connection.X. - if X is a table, show its columns
          // (single-schema shorthand for @conn.table.col); if X is a schema,
          // show that schema's tables. Pattern 2 above handles the fully
          // qualified @conn.schema.table. form.
          const twoPartPattern = /@([\w-]+)\.([\w-]+)\.(\w*)$/
          const twoPartMatch = textBeforeCursor.match(twoPartPattern)

          if (twoPartMatch) {
            const [, connId, middle, partial] = twoPartMatch
            const connection = findConnection(connections, connId)
            const connSchemas = getConnectionSchemas(schemas, connId)

            if (connection && connSchemas) {
              // (a) middle is a table -> offer its columns (prefer public schema)
              let tableSchema: string | undefined
              for (const schema of connSchemas) {
                if (
                  schema.type === 'schema' &&
                  schema.children?.some(
                    t => t.type === 'table' && t.name.toLowerCase() === middle.toLowerCase()
                  )
                ) {
                  tableSchema = schema.name
                  if (schema.name === 'public') break
                }
              }

              if (tableSchema && connection.sessionId && columnLoader) {
                const cacheKey = `${connection.sessionId}-${tableSchema}-${middle}`
                let columns: Column[]
                if (columnCache.has(cacheKey)) {
                  columns = columnCache.get(cacheKey)!
                } else {
                  try {
                    columns = await columnLoader(connection.sessionId, tableSchema, middle)
                    columnCache.set(cacheKey, columns)
                  } catch {
                    columns = []
                  }
                }
                columns
                  .filter(col => !partial || col.name.toLowerCase().startsWith(partial.toLowerCase()))
                  .forEach(col => {
                    options.push({
                      label: col.name,
                      type: 'property',
                      detail: col.dataType,
                      info: col.nullable ? 'Nullable' : 'Not null',
                      boost: 90,
                    })
                  })
                return { from: context.pos - partial.length, options }
              }

              // (b) middle is a schema -> offer the schema's tables
              const schemaNode = connSchemas.find(
                s => s.type === 'schema' && s.name.toLowerCase() === middle.toLowerCase()
              )
              if (schemaNode?.children) {
                schemaNode.children
                  .filter(t => t.type === 'table')
                  .filter(t => !partial || t.name.toLowerCase().startsWith(partial.toLowerCase()))
                  .forEach(table => {
                    options.push({
                      label: table.name,
                      type: 'class',
                      detail: `@${connection.name}.${schemaNode.name}.${table.name}`,
                      info: `Table from ${connection.name}`,
                      boost: 80,
                    })
                  })
                return { from: context.pos - partial.length, options }
              }
            }
          }

          // Pattern 3: @connection. - show schemas/tables
          const tablePattern = /@([\w-]+)\.(\w*)$/
          const tableMatch = textBeforeCursor.match(tablePattern)

          if (tableMatch) {
            const [, connId, partialTable] = tableMatch
            const connection = findConnection(connections, connId)

            // Case-insensitive schema lookup by connection id/name.
            const connSchemas = getConnectionSchemas(schemas, connId)
            if (connection && connSchemas) {

              connSchemas.forEach(schema => {
                if (schema.type === 'schema' && schema.children) {
                  schema.children
                    .filter(t => t.type === 'table')
                    .filter(t => !partialTable || t.name.toLowerCase().startsWith(partialTable.toLowerCase()))
                    .forEach(table => {
                      const fullName = schema.name === 'public' ? table.name : `${schema.name}.${table.name}`
                      options.push({
                        label: table.name,
                        type: 'class',
                        detail: `@${connection.name}.${schema.name}.${table.name}`,
                        apply: fullName,
                        info: `Table from ${connection.name}`,
                        boost: 80
                      })
                    })
                }
              })

              return {
                from: context.pos - partialTable.length,
                options
              }
            }
            
            // If pattern matched but no schemas found, return empty to prevent fallthrough
            return {
              from: context.pos - partialTable.length,
              options: []
            }
          }
        }

        // Single-DB Mode or fallback: table.column pattern
        if (mode === 'single' || !textBeforeCursor.includes('@')) {
          const tableColPattern = /(\w+)\.(\w*)$/
          const match = textBeforeCursor.match(tableColPattern)

          if (match && columnLoader && schemas.size > 0) {
            const [, tableName, partialColumn] = match
            const firstSchemaSet = Array.from(schemas.values())[0]

            if (firstSchemaSet && connections.length > 0 && connections[0].sessionId) {
              for (const schemaNode of firstSchemaSet) {
                if (schemaNode.type === 'schema' && schemaNode.children) {
                  const table = schemaNode.children.find(
                    t => t.type === 'table' && t.name.toLowerCase() === tableName.toLowerCase()
                  )

                  if (table) {
                    const cacheKey = `${connections[0].sessionId}-${schemaNode.name}-${tableName}`
                    let columns: Column[]

                    if (columnCache.has(cacheKey)) {
                      columns = columnCache.get(cacheKey)!
                    } else {
                      try {
                        columns = await columnLoader(connections[0].sessionId, schemaNode.name, tableName)
                        columnCache.set(cacheKey, columns)
                      } catch {
                        columns = []
                      }
                    }

                    columns
                      .filter(col => !partialColumn || col.name.toLowerCase().startsWith(partialColumn.toLowerCase()))
                      .forEach(col => {
                        options.push({
                          label: col.name,
                          type: 'property',
                          detail: col.dataType,
                          info: col.nullable ? 'Nullable' : 'Not null',
                          boost: 90
                        })
                      })

                    if (options.length > 0) {
                      return {
                        from: context.pos - partialColumn.length,
                        options
                      }
                    }
                  }
                }
              }
            }
          }
        }

        // Default: SQL keywords and tables
        const currentWord = word.text.toUpperCase()
        const isKeyword = isTypingSQLKeyword(currentWord)

        // Add SQL keywords
        if (!textBeforeCursor.includes('@') || isKeyword) {
          SQL_KEYWORDS.forEach((keyword, idx) => {
            if (keyword.startsWith(currentWord) || currentWord.length < 2) {
              options.push({
                label: keyword,
                type: 'keyword',
                detail: 'SQL keyword',
                boost: 50 - idx
              })
            }
          })
        }

        // Add tables from all schemas
        const schemasToProcess = mode === 'single'
          ? [Array.from(schemas.values())[0]]
          : Array.from(schemas.values())

        schemasToProcess.forEach(schemaSet => {
          if (schemaSet) {
            schemaSet.forEach(schemaNode => {
              if (schemaNode.type === 'schema' && schemaNode.children) {
                schemaNode.children.forEach(table => {
                  if (table.type === 'table') {
                    options.push({
                      label: table.name,
                      type: 'class',
                      detail: `Table in ${schemaNode.name}`,
                      info: mode === 'multi' && connections.length > 1
                        ? `Use @connection.${table.name} for multi-DB queries`
                        : undefined,
                      boost: 70
                    })
                  }
                })
              }
            })
          }
        })

        if (options.length === 0) return null

        return {
          from: word.from,
          options
        }
      }
    ]
  })
}

// ================================================================
// Multi-Database Connection Syntax Highlighting
// ================================================================

/**
 * Decoration styles for multi-database connection references
 * Highlights @connection.table patterns in the SQL editor
 */
const connectionRefMark = Decoration.mark({
  class: 'cm-connection-ref',
  attributes: {
    'data-tooltip': 'Multi-database reference'
  }
})

/**
 * Build decorations for @connection references in the document
 */
function buildConnectionDecorations(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()

  // Pattern: @connection.table or @connection.schema.table
  // Also matches @alias.table when used after aliasing
  const connectionPattern = /@[\w-]+(?:\.[\w-]+)+/g

  for (const { from, to } of view.visibleRanges) {
    const text = view.state.doc.sliceString(from, to)
    let match: RegExpExecArray | null

    connectionPattern.lastIndex = 0
    while ((match = connectionPattern.exec(text))) {
      builder.add(from + match.index, from + match.index + match[0].length, connectionRefMark)
    }
  }

  return builder.finish()
}

/**
 * ViewPlugin for highlighting @connection references
 */
const connectionHighlighter = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet

    constructor(view: EditorView) {
      this.decorations = buildConnectionDecorations(view)
    }

    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) {
        this.decorations = buildConnectionDecorations(update.view)
      }
    }
  },
  {
    decorations: v => v.decorations
  }
)

/**
 * Theme extension for connection reference highlighting
 * Adds visual styles for the @connection.table syntax
 */
const connectionHighlightTheme = EditorView.baseTheme({
  '.cm-connection-ref': {
    color: 'var(--cm-connection-color, #7c3aed)', // Purple/violet for connection references
    fontWeight: '500',
    backgroundColor: 'var(--cm-connection-bg, rgba(124, 58, 237, 0.1))',
    borderRadius: '2px',
    padding: '0 2px'
  }
})

/**
 * Dark theme overrides for connection highlighting
 */
const connectionHighlightDarkTheme = EditorView.theme({
  '.cm-connection-ref': {
    color: '#a78bfa', // Lighter purple for dark mode
    backgroundColor: 'rgba(167, 139, 250, 0.15)'
  }
})

/**
 * Create the multi-database syntax highlighting extension
 */
export function multiDatabaseHighlighting(theme: 'light' | 'dark'): Extension[] {
  const extensions: Extension[] = [
    connectionHighlighter,
    connectionHighlightTheme
  ]

  if (theme === 'dark') {
    extensions.push(connectionHighlightDarkTheme)
  }

  return extensions
}

/**
 * Create base SQL extensions
 */
export function createSQLExtensions(
  theme: 'light' | 'dark',
  columnLoader?: ColumnLoader,
  onChange?: (value: string) => void
): Extension[] {
  const extensions: Extension[] = [
    history(),
    EditorView.lineWrapping,
    EditorState.tabSize.of(2),
    sql({
      dialect: SQLDialect.define({
        // Support @ in identifiers for multi-DB syntax
        charSetCasts: true,
        // Add support for @connection syntax
      })
    }),
    schemaState,
    sqlAutocompletion(columnLoader),
    // Multi-database @connection.table syntax highlighting
    ...multiDatabaseHighlighting(theme),
    keymap.of([
      // Tab key accepts completion if available, otherwise indents
      {
        key: 'Tab',
        run: (view) => {
          // Try to accept completion first
          if (acceptCompletion(view)) {
            return true
          }
          // Otherwise, indent
          return indentMore(view)
        }
      },
      ...defaultKeymap,
      ...historyKeymap,
      ...searchKeymap,
      {
        key: 'Ctrl-Space',
        mac: 'Cmd-Space',
        run: startCompletion
      }
    ]),
    EditorView.updateListener.of((update: ViewUpdate) => {
      if (update.docChanged && onChange) {
        onChange(update.state.doc.toString())
      }
    })
  ]

  // Add theme
  if (theme === 'dark') {
    extensions.push(oneDark)
  }

  return extensions
}

/**
 * Update editor schema state
 */
export function updateEditorSchema(
  view: EditorView,
  connections: Connection[],
  schemas: Map<string, SchemaNode[]>,
  mode: 'single' | 'multi',
  isLoading?: boolean
) {
  view.dispatch({
    effects: updateSchemaEffect.of({ connections, schemas, mode, isLoading })
  })
}

