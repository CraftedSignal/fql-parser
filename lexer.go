package fql

import "strings"

// TokenType identifies the lexical class of a token.
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIllegal
	TokenWord   // bare word: field names and unquoted values (incl. wildcards)
	TokenString // quoted string literal, single or double quotes
	TokenBang   // !   (negation prefix)
	TokenColon  // :
	TokenNColon // !:
	TokenTilde  // ~
	TokenNTilde // !~
	TokenEq     // =
	TokenNEq    // !=
	TokenGT     // >
	TokenLT     // <
	TokenGTE    // >=
	TokenLTE    // <=
	TokenPlus   // +   (AND)
	TokenComma  // ,   (OR / list separator)
	TokenLParen // (
	TokenRParen // )
	TokenLBrack // [
	TokenRBrack // ]
	TokenPipe   // |
)

// Token is a single lexed unit with its source position.
type Token struct {
	Type TokenType
	Text string // raw text; for TokenString this is the DECODED value
	Pos  int    // byte offset in the input
	Line int    // 1-based line
	Col  int    // 1-based column
}

// wordBoundary reports whether ch terminates a bare word.
func wordBoundary(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n', '!', ':', '~', '=', '<', '>', '+', ',', '(', ')', '[', ']', '|', '\'', '"':
		return true
	}
	return false
}

type lexer struct {
	input string
	pos   int
	line  int
	col   int
}

func newLexer(input string) *lexer {
	return &lexer{input: input, line: 1, col: 1}
}

func (l *lexer) advance() byte {
	ch := l.input[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

func (l *lexer) peek() byte {
	if l.pos >= len(l.input) {
		return 0
	}
	return l.input[l.pos]
}

func (l *lexer) peekAt(offset int) byte {
	if l.pos+offset >= len(l.input) {
		return 0
	}
	return l.input[l.pos+offset]
}

// lex tokenizes the whole input. It never fails: unknown bytes become
// TokenIllegal tokens and lexing continues.
func (l *lexer) lex() []Token {
	var tokens []Token
	for l.pos < len(l.input) {
		startPos, startLine, startCol := l.pos, l.line, l.col
		ch := l.peek()

		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			l.advance()
			continue

		case ch == '\'' || ch == '"':
			value, ok := l.lexString(ch)
			tt := TokenString
			if !ok {
				tt = TokenIllegal
			}
			tokens = append(tokens, Token{Type: tt, Text: value, Pos: startPos, Line: startLine, Col: startCol})

		case ch == '!':
			l.advance()
			switch l.peek() {
			case ':':
				l.advance()
				tokens = append(tokens, Token{Type: TokenNColon, Text: "!:", Pos: startPos, Line: startLine, Col: startCol})
			case '~':
				l.advance()
				tokens = append(tokens, Token{Type: TokenNTilde, Text: "!~", Pos: startPos, Line: startLine, Col: startCol})
			case '=':
				l.advance()
				tokens = append(tokens, Token{Type: TokenNEq, Text: "!=", Pos: startPos, Line: startLine, Col: startCol})
			default:
				tokens = append(tokens, Token{Type: TokenBang, Text: "!", Pos: startPos, Line: startLine, Col: startCol})
			}

		case ch == '>':
			l.advance()
			if l.peek() == '=' {
				l.advance()
				tokens = append(tokens, Token{Type: TokenGTE, Text: ">=", Pos: startPos, Line: startLine, Col: startCol})
			} else {
				tokens = append(tokens, Token{Type: TokenGT, Text: ">", Pos: startPos, Line: startLine, Col: startCol})
			}

		case ch == '<':
			l.advance()
			if l.peek() == '=' {
				l.advance()
				tokens = append(tokens, Token{Type: TokenLTE, Text: "<=", Pos: startPos, Line: startLine, Col: startCol})
			} else {
				tokens = append(tokens, Token{Type: TokenLT, Text: "<", Pos: startPos, Line: startLine, Col: startCol})
			}

		case ch == ':':
			l.advance()
			tokens = append(tokens, Token{Type: TokenColon, Text: ":", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '~':
			l.advance()
			tokens = append(tokens, Token{Type: TokenTilde, Text: "~", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '=':
			l.advance()
			tokens = append(tokens, Token{Type: TokenEq, Text: "=", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '+':
			l.advance()
			tokens = append(tokens, Token{Type: TokenPlus, Text: "+", Pos: startPos, Line: startLine, Col: startCol})
		case ch == ',':
			l.advance()
			tokens = append(tokens, Token{Type: TokenComma, Text: ",", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '(':
			l.advance()
			tokens = append(tokens, Token{Type: TokenLParen, Text: "(", Pos: startPos, Line: startLine, Col: startCol})
		case ch == ')':
			l.advance()
			tokens = append(tokens, Token{Type: TokenRParen, Text: ")", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '[':
			l.advance()
			tokens = append(tokens, Token{Type: TokenLBrack, Text: "[", Pos: startPos, Line: startLine, Col: startCol})
		case ch == ']':
			l.advance()
			tokens = append(tokens, Token{Type: TokenRBrack, Text: "]", Pos: startPos, Line: startLine, Col: startCol})
		case ch == '|':
			l.advance()
			tokens = append(tokens, Token{Type: TokenPipe, Text: "|", Pos: startPos, Line: startLine, Col: startCol})

		default:
			word := l.lexWord()
			if word == "" {
				// Unknown byte (e.g. control char) — consume so lexing progresses.
				l.advance()
				tokens = append(tokens, Token{Type: TokenIllegal, Text: string(ch), Pos: startPos, Line: startLine, Col: startCol})
			} else {
				tokens = append(tokens, Token{Type: TokenWord, Text: word, Pos: startPos, Line: startLine, Col: startCol})
			}
		}
	}
	tokens = append(tokens, Token{Type: TokenEOF, Pos: l.pos, Line: l.line, Col: l.col})
	return tokens
}

// lexString consumes a quoted string with backslash escapes and returns the
// decoded value. ok is false when the closing quote is missing.
func (l *lexer) lexString(quote byte) (string, bool) {
	l.advance() // opening quote
	var sb strings.Builder
	for l.pos < len(l.input) {
		ch := l.advance()
		if ch == '\\' && l.pos < len(l.input) {
			next := l.advance()
			switch next {
			case quote, '\\':
				// Only the quote char and backslash itself are escapes.
				sb.WriteByte(next)
			default:
				// Everything else stays literal: FQL values are full of
				// Windows paths ('*\regsvr32.exe') and regex escapes ('\d+')
				// where \r \n \t \d must remain backslash + char, not a
				// control character.
				sb.WriteByte('\\')
				sb.WriteByte(next)
			}
			continue
		}
		if ch == quote {
			return sb.String(), true
		}
		sb.WriteByte(ch)
	}
	return sb.String(), false // unterminated
}

// lexWord consumes a run of non-boundary bytes.
func (l *lexer) lexWord() string {
	start := l.pos
	for l.pos < len(l.input) && !wordBoundary(l.input[l.pos]) {
		l.advance()
	}
	return l.input[start:l.pos]
}
