import { describe, expect, it } from 'vitest'

import { getTablesInScope, isAlias, parseQueryContext, resolveAlias } from '../sql-context-parser'

describe('parseQueryContext table references', () => {
  it('does not treat a trailing SQL keyword as a table alias', () => {
    const query = 'select * from accounts where '
    const ctx = parseQueryContext(query, query.length)

    expect(ctx.tables).toHaveLength(1)
    expect(ctx.tables[0].tableName).toBe('accounts')
    expect(ctx.tables[0].alias).toBeUndefined()
    // The bug this guards: "where" landed in the alias map and then leaked into
    // completion labels as `where.<name>`.
    expect(isAlias('where', ctx)).toBe(false)
  })

  it('does not treat JOIN / ON as aliases', () => {
    const query = 'select * from orders join accounts on orders.id = accounts.order_id where '
    const ctx = parseQueryContext(query, query.length)

    const byName = Object.fromEntries(ctx.tables.map(t => [t.tableName, t]))
    expect(byName.orders.alias).toBeUndefined()
    expect(byName.accounts.alias).toBeUndefined()
    expect(isAlias('join', ctx)).toBe(false)
    expect(isAlias('on', ctx)).toBe(false)
  })

  it('still captures real aliases, with and without AS', () => {
    const query = 'select * from accounts a join users AS u on a.id = u.account_id where '
    const ctx = parseQueryContext(query, query.length)

    expect(resolveAlias('a', ctx)?.tableName).toBe('accounts')
    expect(resolveAlias('u', ctx)?.tableName).toBe('users')
  })

  it('captures schema-qualified tables once, without a keyword alias', () => {
    const query = 'select * from public.accounts where '
    const ctx = parseQueryContext(query, query.length)

    expect(getTablesInScope(ctx)).toHaveLength(1)
    expect(ctx.tables[0]).toMatchObject({ schema: 'public', tableName: 'accounts' })
    expect(ctx.tables[0].alias).toBeUndefined()
  })

  it('keeps aliases on multi-db references but rejects keywords', () => {
    const query = 'select * from @prod.public.accounts acct join @prod.public.users where '
    const ctx = parseQueryContext(query, query.length)

    expect(resolveAlias('acct', ctx)?.tableName).toBe('accounts')
    expect(isAlias('where', ctx)).toBe(false)
  })
})

describe('parseQueryContext clause detection', () => {
  it('detects the clause using an offset relative to the current statement', () => {
    const first = 'select * from users;\n'
    const second = 'select * from accounts where '
    const query = first + second

    const ctx = parseQueryContext(query, query.length)

    expect(ctx.currentClause).toBe('WHERE')
    expect(ctx.tables.map(t => t.tableName)).toEqual(['accounts'])
  })

  it('handles leading whitespace without shifting the cursor', () => {
    const query = '\n\n   select * from accounts where '
    const ctx = parseQueryContext(query, query.length)

    expect(ctx.currentClause).toBe('WHERE')
  })

  it('reports FROM while the cursor is still in the FROM clause', () => {
    const query = 'select id from acc'
    const ctx = parseQueryContext(query, query.length)

    expect(ctx.currentClause).toBe('FROM')
  })
})
