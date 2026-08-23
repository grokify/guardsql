import { GuardSQLParseError } from './errors.js'
import { lex, type Token } from './lexer.js'
import type {
  AggregateFunction,
  CompareOp,
  Expr,
  Join,
  JoinType,
  Order,
  Query,
  SelectItem,
  Value,
} from './types.js'

export function parse(input: string): Query {
  return new Parser(lex(input)).parseQuery()
}

class Parser {
  private pos = 0

  constructor(private readonly tokens: Token[]) {}

  parseQuery(): Query {
    const ctes = this.matchKeyword('WITH') ? this.parseCTEs() : []
    const select = this.matchKeyword('SELECT') ? this.parseSelectList() : [{ field: '', star: true }]
    this.expectKeyword('FROM')
    const source = this.parseSource()
    const joins: Join[] = []
    while (this.nextStartsJoin()) joins.push(this.parseJoin())
    const query: Query = {
      operation: 'read',
      select,
      from: source.entity,
      fromAlias: source.alias,
      fromQuery: source.query,
      joins,
      groupBy: [],
      orderBy: [],
      ctes,
    }
    if (this.matchKeyword('WHERE')) query.where = this.parseExpr()
    if (this.matchKeyword('GROUP')) {
      this.expectKeyword('BY')
      query.groupBy = this.parseIdentifierList()
    }
    if (this.matchKeyword('HAVING')) query.having = this.parseExpr()
    if (this.matchKeyword('PRIORITIZE')) {
      this.expectKeyword('BY')
      query.prioritizeBy = this.parseIdentifier()
    }
    if (this.matchKeyword('ORDER')) {
      this.expectKeyword('BY')
      query.orderBy = this.parseOrderList()
    }
    if (this.matchKeyword('LIMIT')) {
      const tok = this.consume()
      if (tok.type !== 'number' || !/^\d+$/.test(tok.text)) {
        throw new GuardSQLParseError('LIMIT must be an integer', tok.offset)
      }
      query.limit = Number(tok.text)
    }
    this.expectEOFOrCloseParen()
    return query
  }

  private parseCTEs() {
    const ctes = []
    do {
      const name = this.parseIdentifier()
      this.expectKeyword('AS')
      this.expectPunct('(')
      const query = this.parseQuery()
      this.expectPunct(')')
      ctes.push({ name, query })
    } while (this.matchPunct(','))
    return ctes
  }

  private parseSelectList(): SelectItem[] {
    const items: SelectItem[] = []
    do {
      if (this.matchPunct('*')) {
        items.push({ field: '', star: true })
        continue
      }
      const name = this.parseIdentifier()
      const maybeAgg = aggregateFunction(name)
      let item: SelectItem
      if (maybeAgg && this.matchPunct('(')) {
        if (this.matchPunct('*')) {
          item = { field: '', aggregate: { func: maybeAgg, field: '', star: true } }
        } else {
          const field = this.parseIdentifier()
          item = { field, aggregate: { func: maybeAgg, field } }
        }
        this.expectPunct(')')
      } else {
        item = { field: name }
      }
      if (this.matchKeyword('AS')) item.alias = this.parseIdentifier()
      else if (this.peek().type === 'identifier' && !reserved(this.peek().text)) item.alias = this.parseIdentifier()
      items.push(item)
    } while (this.matchPunct(','))
    return items
  }

  private parseSource(): { entity: string; alias?: string; query?: Query } {
    let entity = ''
    let query: Query | undefined
    if (this.matchPunct('(')) {
      query = this.parseQuery()
      this.expectPunct(')')
    } else {
      entity = this.parseIdentifier()
    }
    let alias: string | undefined
    if (this.matchKeyword('AS')) alias = this.parseIdentifier()
    else if (this.peek().type === 'identifier' && !reserved(this.peek().text)) alias = this.parseIdentifier()
    return { entity, alias, query }
  }

  private parseJoin(): Join {
    let type: JoinType = 'inner'
    if (this.matchKeyword('JOIN')) {
      type = 'inner'
    } else if (this.matchKeyword('INNER')) {
      this.expectKeyword('JOIN')
      type = 'inner'
    } else if (this.matchKeyword('LEFT')) {
      this.matchKeyword('OUTER')
      this.expectKeyword('JOIN')
      type = 'left'
    } else if (this.matchKeyword('RIGHT')) {
      this.matchKeyword('OUTER')
      this.expectKeyword('JOIN')
      type = 'right'
    } else if (this.matchKeyword('FULL')) {
      this.matchKeyword('OUTER')
      this.expectKeyword('JOIN')
      type = 'full'
    } else {
      this.expectKeyword('CROSS')
      this.expectKeyword('JOIN')
      type = 'cross'
    }
    const source = this.parseSource()
    const join: Join = { type, entity: source.entity, alias: source.alias, query: source.query }
    if (type !== 'cross' || this.matchKeyword('ON')) {
      if (!join.on) join.on = this.parseExpr()
    }
    return join
  }

  private parseExpr(): Expr {
    return this.parseOr()
  }

  private parseOr(): Expr {
    let left = this.parseAnd()
    while (this.matchKeyword('OR')) left = { kind: 'logical', op: 'OR', left, right: this.parseAnd() }
    return left
  }

  private parseAnd(): Expr {
    let left = this.parseNot()
    while (this.matchKeyword('AND')) left = { kind: 'logical', op: 'AND', left, right: this.parseNot() }
    return left
  }

  private parseNot(): Expr {
    if (this.matchKeyword('NOT')) return { kind: 'not', expr: this.parsePrimaryExpr() }
    return this.parsePrimaryExpr()
  }

  private parsePrimaryExpr(): Expr {
    if (this.matchPunct('(')) {
      const expr = this.parseExpr()
      this.expectPunct(')')
      return expr
    }
    const field = this.parseIdentifier()
    if (this.matchKeyword('IS')) {
      const not = this.matchKeyword('NOT')
      this.expectKeyword('NULL')
      return { kind: 'compare', field, op: not ? 'IS_NOT_NULL' : 'IS_NULL' }
    }
    if (this.matchKeyword('IN')) {
      this.expectPunct('(')
      const values: Value[] = []
      if (!this.checkPunct(')')) {
        do values.push(this.parseValue())
        while (this.matchPunct(','))
      }
      this.expectPunct(')')
      return { kind: 'compare', field, op: 'IN', values }
    }
    let op: CompareOp
    if (this.matchKeyword('CONTAINS')) op = 'CONTAINS'
    else {
      const tok = this.consume()
      if (tok.type !== 'operator') throw new GuardSQLParseError('expected comparison operator', tok.offset)
      if (!['=', '!=', '<', '<=', '>', '>='].includes(tok.text)) {
        throw new GuardSQLParseError('invalid comparison operator', tok.offset)
      }
      op = tok.text as CompareOp
    }
    return { kind: 'compare', field, op, value: this.parseValue() }
  }

  private parseValue(): Value {
    const tok = this.consume()
    if (tok.type === 'string') return tok.text
    if (tok.type === 'number') return Number(tok.text)
    if (tok.type === 'identifier') {
      if (eq(tok.text, 'TRUE')) return true
      if (eq(tok.text, 'FALSE')) return false
      if (eq(tok.text, 'NULL')) return null
    }
    throw new GuardSQLParseError('expected literal value', tok.offset)
  }

  private parseIdentifierList(): string[] {
    const out = []
    do out.push(this.parseIdentifier())
    while (this.matchPunct(','))
    return out
  }

  private parseOrderList(): Order[] {
    const out = []
    do {
      const field = this.parseIdentifier()
      const desc = this.matchKeyword('DESC')
      if (!desc) this.matchKeyword('ASC')
      out.push({ field, desc })
    } while (this.matchPunct(','))
    return out
  }

  private parseIdentifier(): string {
    const tok = this.consume()
    if (tok.type !== 'identifier') throw new GuardSQLParseError('expected identifier', tok.offset)
    return tok.text
  }

  private nextStartsJoin() {
    return ['JOIN', 'INNER', 'LEFT', 'RIGHT', 'FULL', 'CROSS'].some((kw) => this.checkKeyword(kw))
  }

  private expectEOFOrCloseParen() {
    if (this.peek().type === 'eof' || this.checkPunct(')')) return
    throw new GuardSQLParseError('unexpected token', this.peek().offset)
  }

  private consume() {
    return this.tokens[this.pos++]
  }

  private peek() {
    return this.tokens[this.pos]
  }

  private matchKeyword(keyword: string) {
    if (!this.checkKeyword(keyword)) return false
    this.pos++
    return true
  }

  private checkKeyword(keyword: string) {
    const tok = this.peek()
    return tok.type === 'identifier' && eq(tok.text, keyword)
  }

  private expectKeyword(keyword: string) {
    if (!this.matchKeyword(keyword)) throw new GuardSQLParseError(`expected ${keyword}`, this.peek().offset)
  }

  private matchPunct(text: string) {
    if (!this.checkPunct(text)) return false
    this.pos++
    return true
  }

  private checkPunct(text: string) {
    const tok = this.peek()
    return tok.type === 'punct' && tok.text === text
  }

  private expectPunct(text: string) {
    if (!this.matchPunct(text)) throw new GuardSQLParseError(`expected ${text}`, this.peek().offset)
  }
}

function aggregateFunction(value: string): AggregateFunction | undefined {
  const upper = value.toUpperCase()
  if (upper === 'COUNT' || upper === 'SUM' || upper === 'AVG' || upper === 'MIN' || upper === 'MAX') {
    return upper
  }
  return undefined
}

function reserved(value: string) {
  return [
    'JOIN',
    'INNER',
    'LEFT',
    'RIGHT',
    'FULL',
    'OUTER',
    'CROSS',
    'FROM',
    'WHERE',
    'GROUP',
    'HAVING',
    'PRIORITIZE',
    'ORDER',
    'LIMIT',
    'ON',
  ].some((kw) => eq(value, kw))
}

function eq(a: string, b: string) {
  return a.toUpperCase() === b.toUpperCase()
}
