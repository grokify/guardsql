import { GuardSQLParseError } from './errors.js'

export type TokenType = 'identifier' | 'string' | 'number' | 'operator' | 'punct' | 'eof'

export interface Token {
  type: TokenType
  text: string
  offset: number
}

export function lex(input: string): Token[] {
  const tokens: Token[] = []
  let pos = 0
  while (pos < input.length) {
    const ch = input[pos]
    if (/\s/.test(ch)) {
      pos++
      continue
    }
    if (isIdentifierStart(ch)) {
      const start = pos++
      while (pos < input.length && isIdentifierPart(input[pos])) pos++
      tokens.push({ type: 'identifier', text: input.slice(start, pos), offset: start })
      continue
    }
    if (ch === '"' || ch === "'") {
      const quote = ch
      const start = pos++
      let out = ''
      let closed = false
      while (pos < input.length) {
        const current = input[pos++]
        if (current === quote) {
          tokens.push({ type: 'string', text: out, offset: start })
          closed = true
          break
        }
        if (current === '\\') {
          if (pos >= input.length) throw new GuardSQLParseError('unterminated string', start)
          const escaped = input[pos++]
          if (escaped === 'n') out += '\n'
          else if (escaped === 't') out += '\t'
          else if (escaped === quote || escaped === '\\') out += escaped
          else throw new GuardSQLParseError('unsupported escape sequence', pos - 2)
          continue
        }
        out += current
      }
      if (!closed) throw new GuardSQLParseError('unterminated string', start)
      continue
    }
    if (/[0-9]/.test(ch)) {
      const start = pos++
      while (pos < input.length && /[0-9.]/.test(input[pos])) pos++
      tokens.push({ type: 'number', text: input.slice(start, pos), offset: start })
      continue
    }
    if (ch === '!' || ch === '<' || ch === '>' || ch === '=') {
      const start = pos++
      if (input[pos] === '=') pos++
      tokens.push({ type: 'operator', text: input.slice(start, pos), offset: start })
      continue
    }
    if ('(),*'.includes(ch)) {
      tokens.push({ type: 'punct', text: ch, offset: pos })
      pos++
      continue
    }
    throw new GuardSQLParseError(`unexpected character ${JSON.stringify(ch)}`, pos)
  }
  tokens.push({ type: 'eof', text: '', offset: input.length })
  return tokens
}

function isIdentifierStart(ch: string) {
  return /[A-Za-z_]/.test(ch)
}

function isIdentifierPart(ch: string) {
  return /[A-Za-z0-9_.]/.test(ch)
}
