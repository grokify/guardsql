package guardsql

import (
	"strconv"
	"strings"
	"unicode"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokNumber
	tokBool
	tokNull
	tokComma
	tokLParen
	tokRParen
	tokStar
	tokOp
)

type token struct {
	kind   tokenKind
	text   string
	offset int
}

type lexer struct {
	input string
	pos   int
}

func lexAll(input string) ([]token, error) {
	l := &lexer{input: input}
	var toks []token
	for {
		tok, err := l.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		if tok.kind == tokEOF {
			return toks, nil
		}
	}
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.input) {
		r := rune(l.input[l.pos])
		if !unicode.IsSpace(r) {
			break
		}
		l.pos++
	}
	start := l.pos
	if start >= len(l.input) {
		return token{kind: tokEOF, offset: start}, nil
	}
	ch := l.input[l.pos]
	switch ch {
	case ',':
		l.pos++
		return token{kind: tokComma, text: ",", offset: start}, nil
	case '(':
		l.pos++
		return token{kind: tokLParen, text: "(", offset: start}, nil
	case ')':
		l.pos++
		return token{kind: tokRParen, text: ")", offset: start}, nil
	case '*':
		l.pos++
		return token{kind: tokStar, text: "*", offset: start}, nil
	case '=', '<', '>', '!':
		return l.operator()
	case '\'', '"':
		return l.quoted()
	}
	if isIdentStart(rune(ch)) {
		return l.ident()
	}
	if ch == '-' || ch == '+' || (ch >= '0' && ch <= '9') {
		return l.number()
	}
	return token{}, &ParseError{Message: "unexpected character " + strconv.QuoteRune(rune(ch)), Offset: start}
}

func (l *lexer) operator() (token, error) {
	start := l.pos
	ch := l.input[l.pos]
	l.pos++
	if l.pos < len(l.input) && l.input[l.pos] == '=' {
		if ch == '<' || ch == '>' || ch == '!' {
			l.pos++
			return token{kind: tokOp, text: l.input[start:l.pos], offset: start}, nil
		}
	}
	if ch == '=' || ch == '<' || ch == '>' {
		return token{kind: tokOp, text: l.input[start:l.pos], offset: start}, nil
	}
	return token{}, &ParseError{Message: "invalid operator", Offset: start}
}

func (l *lexer) quoted() (token, error) {
	quote := l.input[l.pos]
	start := l.pos
	l.pos++
	var b strings.Builder
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		l.pos++
		if ch == quote {
			return token{kind: tokString, text: b.String(), offset: start}, nil
		}
		if ch == '\\' && l.pos < len(l.input) {
			next := l.input[l.pos]
			l.pos++
			switch next {
			case '\\', '\'', '"':
				b.WriteByte(next)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				return token{}, &ParseError{Message: "unsupported escape sequence", Offset: l.pos - 2}
			}
			continue
		}
		b.WriteByte(ch)
	}
	return token{}, &ParseError{Message: "unterminated string", Offset: start}
}

func (l *lexer) ident() (token, error) {
	start := l.pos
	l.pos++
	for l.pos < len(l.input) && isIdentPart(rune(l.input[l.pos])) {
		l.pos++
	}
	text := l.input[start:l.pos]
	switch strings.ToUpper(text) {
	case "TRUE", "FALSE":
		return token{kind: tokBool, text: strings.ToLower(text), offset: start}, nil
	case "NULL":
		return token{kind: tokNull, text: "null", offset: start}, nil
	default:
		return token{kind: tokIdent, text: text, offset: start}, nil
	}
}

func (l *lexer) number() (token, error) {
	start := l.pos
	if l.input[l.pos] == '-' || l.input[l.pos] == '+' {
		l.pos++
	}
	digits := 0
	for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
		l.pos++
		digits++
	}
	if l.pos < len(l.input) && l.input[l.pos] == '.' {
		l.pos++
		for l.pos < len(l.input) && l.input[l.pos] >= '0' && l.input[l.pos] <= '9' {
			l.pos++
			digits++
		}
	}
	if digits == 0 {
		return token{}, &ParseError{Message: "invalid number", Offset: start}
	}
	return token{kind: tokNumber, text: l.input[start:l.pos], offset: start}, nil
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}
