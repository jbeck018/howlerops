/**
 * Derive a concise, human-friendly query-tab title from a SQL string.
 *
 * Used to auto-name tabs that still carry the default "New Query" title when a
 * query is executed, so the Open Tabs list shows something meaningful (e.g.
 * "Select users", "Update orders") instead of a wall of "New Query" entries.
 * Users can still rename manually; inference only ever touches default titles.
 */

const DEFAULT_TAB_TITLES = new Set(["", "new query", "untitled", "query"])

/** True when a title is an auto-generated placeholder safe to overwrite. */
export function isDefaultTabTitle(title: string | undefined | null): boolean {
  if (!title) return true
  return DEFAULT_TAB_TITLES.has(title.trim().toLowerCase())
}

function stripComments(sql: string): string {
  return sql
    .replace(/\/\*[\s\S]*?\*\//g, " ") // block comments
    .replace(/--[^\n]*/g, " ") // line comments
}

/** Strip quotes/brackets/backticks and any schema prefix, keep the last segment. */
function cleanIdentifier(raw: string): string {
  const unquoted = raw.replace(/[`"[\]]/g, "")
  const segments = unquoted.split(".")
  return segments[segments.length - 1] ?? unquoted
}

function truncate(value: string, max = 40): string {
  const trimmed = value.trim()
  return trimmed.length > max ? `${trimmed.slice(0, max - 1)}…` : trimmed
}

// Order matters: more specific statements (INSERT/UPDATE/DELETE/DDL) are matched
// before the generic FROM clause so a SELECT falls through to the last rule.
const PATTERNS: Array<{ re: RegExp; verb: string }> = [
  { re: /\binsert\s+into\s+([`"[\]\w.]+)/i, verb: "Insert" },
  { re: /\bupdate\s+([`"[\]\w.]+)/i, verb: "Update" },
  { re: /\bdelete\s+from\s+([`"[\]\w.]+)/i, verb: "Delete" },
  {
    re: /\bcreate\s+(?:or\s+replace\s+)?(?:table|view|materialized\s+view)\s+(?:if\s+not\s+exists\s+)?([`"[\]\w.]+)/i,
    verb: "Create",
  },
  { re: /\balter\s+table\s+([`"[\]\w.]+)/i, verb: "Alter" },
  { re: /\bdrop\s+(?:table|view)\s+(?:if\s+exists\s+)?([`"[\]\w.]+)/i, verb: "Drop" },
  { re: /\bfrom\s+([`"[\]\w.]+)/i, verb: "Select" }, // SELECT ... FROM
]

/**
 * Infer a title from SQL. Returns null when nothing meaningful can be derived
 * (e.g. an empty query), so callers can leave the existing title untouched.
 */
export function inferTabTitle(sql: string): string | null {
  if (!sql) return null

  const cleaned = stripComments(sql).replace(/\s+/g, " ").trim()

  if (!cleaned) {
    // Only comments — use the first comment's text as the name, if any.
    const lineComment = sql.match(/--\s*(.+)/)?.[1]
    const blockComment = sql.match(/\/\*\s*([\s\S]*?)\s*\*\//)?.[1]
    const comment = (lineComment ?? blockComment)?.trim()
    return comment ? truncate(comment) : null
  }

  for (const { re, verb } of PATTERNS) {
    const match = cleaned.match(re)
    if (match?.[1]) {
      const table = cleanIdentifier(match[1])
      if (table) return truncate(`${verb} ${table}`)
    }
  }

  // Fallback: the leading chunk of the query itself.
  return truncate(cleaned)
}
