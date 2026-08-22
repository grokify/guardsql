import { parse } from './parser.js'
import type { Expr, Join, JoinType, Order, Query, SelectItem, Value } from './types.js'

export type FormatStyle = 'multiline' | 'singleline'
export type HighlightStyle = 'none' | 'ansi'

export interface FormatOptions {
  style?: FormatStyle
  indent?: string
  highlight?: HighlightStyle
}

export function format(input: string, options: FormatOptions = {}): string {
  return formatQuery(parse(input), options)
}

export function formatQuery(query: Query, options: FormatOptions = {}): string {
  const opts = normalizeOptions(options)
  let out = formatQueryLevel(query, opts, 0)
  if (opts.style === 'singleline') out = out.split(/\s+/).filter(Boolean).join(' ')
  if (opts.highlight === 'ansi') out = highlightANSI(out)
  return out
}

export function highlightANSI(input: string): string {
  return input.replace(
    /\b(WITH|AS|SELECT|FROM|JOIN|INNER|LEFT|RIGHT|FULL|OUTER|CROSS|ON|WHERE|AND|OR|NOT|IN|CONTAINS|IS|NULL|GROUP|BY|HAVING|PRIORITIZE|ORDER|ASC|DESC|LIMIT|COUNT|SUM|AVG|MIN|MAX|TRUE|FALSE)\b/g,
    (word) => {
      if (word === 'TRUE' || word === 'FALSE' || word === 'NULL') return `\u001b[35m${word}\u001b[0m`
      return `\u001b[1;36m${word}\u001b[0m`
    },
  )
}

function normalizeOptions(options: FormatOptions): Required<FormatOptions> {
  return {
    style: options.style ?? 'multiline',
    indent: options.indent ?? '\t',
    highlight: options.highlight ?? 'none',
  }
}

function formatQueryLevel(query: Query, options: Required<FormatOptions>, level: number): string {
  const clauses: string[] = []
  if (query.ctes.length > 0) clauses.push(formatCTEs(query, options, level))
  if (query.select.length > 0) clauses.push(`SELECT ${formatSelectItems(query.select)}`)
  clauses.push(`FROM ${formatSource(query, options, level)}`)
  query.joins.forEach((join) => clauses.push(formatJoin(join, options, level)))
  if (query.where) clauses.push(`WHERE ${formatExpr(query.where)}`)
  if (query.groupBy.length > 0) clauses.push(`GROUP BY ${query.groupBy.join(', ')}`)
  if (query.having) clauses.push(`HAVING ${formatExpr(query.having)}`)
  if (query.prioritizeBy) clauses.push(`PRIORITIZE BY ${query.prioritizeBy}`)
  if (query.orderBy.length > 0) clauses.push(`ORDER BY ${formatOrders(query.orderBy)}`)
  if (query.limit !== undefined) clauses.push(`LIMIT ${query.limit}`)
  if (options.style === 'singleline') return clauses.join(' ')
  const prefix = options.indent.repeat(level)
  return `${prefix}${clauses.join(`\n${prefix}`)}`
}

function formatCTEs(query: Query, options: Required<FormatOptions>, level: number): string {
  const ctes = query.ctes.map((cte) => {
    const nested = formatQueryLevel(cte.query, options, level + 1)
    if (options.style === 'singleline') return `${cte.name} AS (${nested})`
    return `${cte.name} AS (\n${nested}\n${options.indent.repeat(level)})`
  })
  return `WITH ${ctes.join(', ')}`
}

function formatSource(query: Query, options: Required<FormatOptions>, level: number): string {
  let source = query.from
  if (query.fromQuery) {
    const nested = formatQueryLevel(query.fromQuery, options, level + 1)
    source =
      options.style === 'singleline'
        ? `(${nested})`
        : `(\n${nested}\n${options.indent.repeat(level)})`
  }
  if (query.fromAlias) source += ` AS ${query.fromAlias}`
  return source
}

function formatJoin(join: Join, options: Required<FormatOptions>, level: number): string {
  let source = join.entity
  if (join.query) {
    const nested = formatQueryLevel(join.query, options, level + 1)
    source =
      options.style === 'singleline'
        ? `(${nested})`
        : `(\n${nested}\n${options.indent.repeat(level)})`
  }
  if (join.alias) source += ` AS ${join.alias}`
  const on = join.type !== 'cross' && join.on ? ` ON ${formatExpr(join.on)}` : ''
  return `${joinKeyword(join.type)} ${source}${on}`
}

function joinKeyword(type: JoinType): string {
  if (type === 'left') return 'LEFT JOIN'
  if (type === 'right') return 'RIGHT JOIN'
  if (type === 'full') return 'FULL JOIN'
  if (type === 'cross') return 'CROSS JOIN'
  return 'JOIN'
}

function formatSelectItems(items: SelectItem[]): string {
  return items
    .map((item) => {
      let text = item.star ? '*' : item.field
      if (item.aggregate) {
        text = item.aggregate.star ? `${item.aggregate.func}(*)` : `${item.aggregate.func}(${item.aggregate.field})`
      }
      if (item.alias) text += ` AS ${item.alias}`
      return text
    })
    .join(', ')
}

function formatOrders(orders: Order[]): string {
  return orders.map((order) => `${order.field}${order.desc ? ' DESC' : ''}`).join(', ')
}

function formatExpr(expr: Expr): string {
  return formatExprPrecedence(expr, 0)
}

function formatExprPrecedence(expr: Expr, parentPrecedence: number): string {
  if (expr.kind === 'logical') {
    const precedence = expr.op === 'AND' ? 2 : 1
    const text = `${formatExprPrecedence(expr.left, precedence)} ${expr.op} ${formatExprPrecedence(
      expr.right,
      precedence + 1,
    )}`
    return precedence < parentPrecedence ? `(${text})` : text
  }
  if (expr.kind === 'not') {
    const text = `NOT ${formatExprPrecedence(expr.expr, 3)}`
    return 3 < parentPrecedence ? `(${text})` : text
  }
  if (expr.op === 'IS_NULL') return `${expr.field} IS NULL`
  if (expr.op === 'IS_NOT_NULL') return `${expr.field} IS NOT NULL`
  if (expr.op === 'IN') return `${expr.field} IN (${(expr.values ?? []).map(formatValue).join(', ')})`
  return `${expr.field} ${expr.op} ${formatValue(expr.value)}`
}

function formatValue(value: Value | undefined): string {
  if (value === undefined || value === null) return 'null'
  if (typeof value === 'string') return JSON.stringify(value)
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  return String(value)
}
