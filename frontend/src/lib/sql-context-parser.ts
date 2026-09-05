/**
 * SQL Context Parser
 *
 * Parses SQL queries to extract context for intelligent autocomplete:
 * - Tables and their aliases
 * - Current SQL clause (WHERE, SELECT, JOIN, etc.)
 * - Multi-DB references (@connection.schema.table)
 *
 * This is a pragmatic parser using regex - handles 90% of cases without full AST parsing.
 */

export type SQLClause =
  | 'SELECT'
  | 'FROM'
  | 'WHERE'
  | 'JOIN'
  | 'ON'
  | 'ORDER_BY'
  | 'GROUP_BY'
  | 'HAVING'
  | 'INSERT'
  | 'UPDATE'
  | 'UNKNOWN'

export interface TableReference {
  /** Original table identifier (could be 'table', 'schema.table', or '@conn.schema.table') */
  identifier: string

  /** Parsed table name */
  tableName: string

  /** Schema name if specified */
  schema?: string

  /** Connection ID for multi-DB mode (@connection) */
  connectionId?: string

  /** Alias if specified (FROM accounts a, FROM users AS u) */
  alias?: string

  /** Whether this is a multi-DB reference starting with @ */
  isMultiDB: boolean
}

export interface QueryContext {
  /** All table references in the query */
  tables: TableReference[]

  /** Current SQL clause where cursor is positioned */
  currentClause: SQLClause

  /** Alias to table mapping for quick lookup */
  aliasMap: Map<string, TableReference>

  /** Full query text */
  query: string

  /** Cursor position in query */
  cursorPos: number
}

/**
 * Parse a SQL query and extract context for autocomplete
 */
export function parseQueryContext(query: string, cursorPos: number): QueryContext {
  // Get the current statement (in case of multiple statements separated by ;)
  // together with the cursor offset *inside* that statement — the raw cursorPos
  // is an offset into the whole document, so using it against a sliced/trimmed
  // statement misidentifies the clause (and suppresses column suggestions).
  const { statement: currentStatement, cursorPos: statementCursorPos } =
    getCurrentStatement(query, cursorPos)

  // Parse table references from FROM and JOIN clauses
  const tables = parseTableReferences(currentStatement)

  // Build alias map
  const aliasMap = new Map<string, TableReference>()
  tables.forEach(table => {
    if (table.alias) {
      aliasMap.set(table.alias.toLowerCase(), table)
    }
    // Also map table name itself for non-aliased references
    aliasMap.set(table.tableName.toLowerCase(), table)
  })

  // Detect current clause
  const currentClause = detectCurrentClause(currentStatement, statementCursorPos)

  return {
    tables,
    currentClause,
    aliasMap,
    query: currentStatement,
    cursorPos
  }
}

/**
 * Get the current SQL statement (handles multiple statements separated by ;)
 * plus the cursor's offset within that trimmed statement.
 */
function getCurrentStatement(
  query: string,
  cursorPos: number
): { statement: string; cursorPos: number } {
  const statements = query.split(';')
  let start = 0

  for (let i = 0; i < statements.length; i++) {
    const raw = statements[i]
    const end = start + raw.length

    if (cursorPos <= end || i === statements.length - 1) {
      const leading = raw.length - raw.trimStart().length
      const statement = raw.trim()
      const relative = Math.max(0, Math.min(statement.length, cursorPos - start - leading))
      return { statement, cursorPos: relative }
    }

    start = end + 1 // skip the semicolon
  }

  const last = statements[statements.length - 1].trim()
  return { statement: last, cursorPos: last.length }
}

/**
 * Words that can directly follow a table name in FROM/JOIN but are NOT aliases.
 * Without this guard `FROM accounts WHERE ...` parses "WHERE" as the table's
 * alias, which then leaks into the alias map and into completion labels as a
 * bogus qualifier (e.g. `where.accounts` instead of `accounts`).
 */
const NON_ALIAS_KEYWORDS = new Set([
  'WHERE', 'JOIN', 'INNER', 'LEFT', 'RIGHT', 'FULL', 'CROSS', 'OUTER', 'NATURAL',
  'ON', 'USING', 'GROUP', 'ORDER', 'HAVING', 'LIMIT', 'OFFSET', 'FETCH', 'UNION',
  'INTERSECT', 'EXCEPT', 'WINDOW', 'SET', 'VALUES', 'RETURNING', 'FOR', 'LATERAL',
  'AS', 'SELECT', 'FROM', 'INTO', 'WITH', 'TABLESAMPLE', 'AND', 'OR', 'NOT',
  'QUALIFY', 'PARTITION', 'SAMPLE', 'ASOF', 'ANTI', 'SEMI', 'PIVOT', 'UNPIVOT',
])

/**
 * Regex fragment for the optional `[AS] alias` that may follow a table name.
 * The negative lookahead keeps the alias group from *consuming* a keyword:
 * without it `FROM orders JOIN accounts` swallows "JOIN" as the alias of
 * `orders`, and the joined table is never parsed at all.
 */
const ALIAS_SUFFIX = `(?:\\s+(?:AS\\s+)?(?!(?:${[...NON_ALIAS_KEYWORDS].join('|')})\\b)(\\w+))?`

/**
 * Treat a captured trailing word as an alias only when it is a real identifier
 * and not a SQL keyword that merely follows the table name. The lookahead above
 * covers the common cases; this is the belt-and-braces check on the capture.
 */
function normaliseAlias(candidate: string | undefined): string | undefined {
  if (!candidate) return undefined
  if (NON_ALIAS_KEYWORDS.has(candidate.toUpperCase())) return undefined
  return candidate
}

/**
 * Parse table references from FROM and JOIN clauses
 */
function parseTableReferences(query: string): TableReference[] {
  const tables: TableReference[] = []

  // Remove comments to avoid false matches
  const cleaned = removeComments(query)

  // Pattern 1: Multi-DB format - @connection.schema.table [AS] alias
  // Matches: @db1.public.accounts, @db1.public.accounts a, @db1.public.accounts AS acc
  const multiDBPattern = new RegExp(`@([\\w-]+)\\.([\\w-]+)\\.([\\w-]+)${ALIAS_SUFFIX}`, 'gi')
  let match: RegExpExecArray | null

  while ((match = multiDBPattern.exec(cleaned)) !== null) {
    const [, connectionId, schema, tableName, alias] = match
    tables.push({
      identifier: match[0],
      tableName,
      schema,
      connectionId,
      alias: normaliseAlias(alias),
      isMultiDB: true
    })
  }

  // Pattern 2: Schema-qualified format - schema.table [AS] alias
  // Matches: public.accounts, dbo.users AS u
  const schemaTablePattern = new RegExp(`\\b(?:FROM|JOIN)\\s+([\\w-]+)\\.([\\w-]+)${ALIAS_SUFFIX}`, 'gi')

  while ((match = schemaTablePattern.exec(cleaned)) !== null) {
    const [, schema, tableName, alias] = match

    // Skip if this looks like it was part of a multi-DB reference (already captured)
    const prevChar = match.index > 0 ? cleaned[match.index - 1] : ''
    if (prevChar === '@') continue

    tables.push({
      identifier: `${schema}.${tableName}`,
      tableName,
      schema,
      alias: normaliseAlias(alias),
      isMultiDB: false
    })
  }

  // Pattern 3: Simple table format - table [AS] alias
  // Matches: accounts, users AS u, orders AS o
  const simpleTablePattern = new RegExp(`\\b(?:FROM|JOIN)\\s+([\\w-]+)${ALIAS_SUFFIX}`, 'gi')

  while ((match = simpleTablePattern.exec(cleaned)) !== null) {
    const [, tableName, alias] = match

    // Skip if this looks like it was part of a schema.table or @conn.schema.table
    const prevChar = match.index > 0 ? cleaned[match.index - 1] : ''
    if (prevChar === '.' || prevChar === '@') continue

    // Skip when the captured name is itself a schema qualifier, i.e. it is
    // immediately followed by a dot (`FROM public.accounts` matches "public"
    // here). That `schema.table` was already captured by Pattern 2; counting
    // the schema as a separate table inflates the table count and makes
    // single-table completions wrongly table-qualify their columns.
    const nextChar = cleaned[match.index + match[0].length]
    if (nextChar === '.') continue

    tables.push({
      identifier: tableName,
      tableName,
      alias: normaliseAlias(alias),
      isMultiDB: false
    })
  }

  // Deduplicate by schema + table + alias so a table referenced by more than one
  // pattern (or repeated in the query) is only counted once. Counting it twice
  // would flip single-table completions into table-qualified ones.
  const seen = new Set<string>()
  return tables.filter((t) => {
    const key = `${t.connectionId ?? ''}|${t.schema ?? ''}|${t.tableName}|${t.alias ?? ''}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

/**
 * Detect which SQL clause the cursor is currently in
 */
function detectCurrentClause(query: string, cursorPos: number): SQLClause {
  const textBeforeCursor = query.slice(0, cursorPos).toUpperCase()

  // Check for clauses in reverse order of precedence
  // (later clauses override earlier ones)

  if (/\bINSERT\s+INTO\b/.test(textBeforeCursor)) {
    return 'INSERT'
  }

  if (/\bUPDATE\b/.test(textBeforeCursor) && /\bSET\b/.test(textBeforeCursor)) {
    return 'UPDATE'
  }

  // Check which clause keyword appears last before cursor
  const selectPos = textBeforeCursor.lastIndexOf('SELECT')
  const fromPos = textBeforeCursor.lastIndexOf('FROM')
  const wherePos = textBeforeCursor.lastIndexOf('WHERE')
  const joinPos = textBeforeCursor.lastIndexOf('JOIN')
  const onPos = textBeforeCursor.lastIndexOf(' ON ')
  const orderPos = textBeforeCursor.lastIndexOf('ORDER BY')
  const groupPos = textBeforeCursor.lastIndexOf('GROUP BY')
  const havingPos = textBeforeCursor.lastIndexOf('HAVING')

  const positions = [
    { pos: selectPos, clause: 'SELECT' as SQLClause },
    { pos: fromPos, clause: 'FROM' as SQLClause },
    { pos: wherePos, clause: 'WHERE' as SQLClause },
    { pos: joinPos, clause: 'JOIN' as SQLClause },
    { pos: onPos, clause: 'ON' as SQLClause },
    { pos: orderPos, clause: 'ORDER_BY' as SQLClause },
    { pos: groupPos, clause: 'GROUP_BY' as SQLClause },
    { pos: havingPos, clause: 'HAVING' as SQLClause },
  ]

  // Find the clause with the highest position (closest to cursor)
  const lastClause = positions
    .filter(p => p.pos >= 0)
    .sort((a, b) => b.pos - a.pos)[0]

  return lastClause?.clause || 'UNKNOWN'
}

/**
 * Remove SQL comments to avoid false positives in parsing
 */
function removeComments(query: string): string {
  // Remove single-line comments (-- comment)
  let cleaned = query.replace(/--[^\n]*/g, '')

  // Remove multi-line comments (/* comment */)
  cleaned = cleaned.replace(/\/\*[\s\S]*?\*\//g, '')

  return cleaned
}

/**
 * Check if a word is a table alias based on context
 */
export function isAlias(word: string, context: QueryContext): boolean {
  return context.aliasMap.has(word.toLowerCase())
}

/**
 * Resolve an alias to its table reference
 */
export function resolveAlias(alias: string, context: QueryContext): TableReference | undefined {
  return context.aliasMap.get(alias.toLowerCase())
}

/**
 * Get all tables that are in scope for the current position
 */
export function getTablesInScope(context: QueryContext): TableReference[] {
  // For now, return all tables in the query
  // Future: could be more sophisticated based on subquery scope
  return context.tables
}
