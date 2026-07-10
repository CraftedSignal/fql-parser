package fql

import (
	"fmt"
	"strings"
)

const (
	// MaxInputSize bounds accepted query text; larger inputs are rejected
	// rather than parsed, keeping worst-case work linear and small.
	MaxInputSize = 128 * 1024
	// MaxDepth bounds expression nesting to keep recursion shallow.
	MaxDepth = 64
)

// Parse parses a Falcon query into an AST. On malformed input err is non-nil
// and the returned Query still holds the partial tree parsed so far.
func Parse(query string) (*Query, error) {
	p := newParser(query)
	q := p.parseQuery()
	if len(p.errors) > 0 {
		return q, fmt.Errorf("%s", strings.Join(p.errors, "; "))
	}
	return q, nil
}

// ParseExpression parses a standalone boolean filter expression (no pipes).
func ParseExpression(input string) (Expr, error) {
	p := newParser(input)
	expr := p.parseExpr(0)
	if p.cur().Type != TokenEOF {
		p.errorf("unexpected trailing input starting at %q", p.cur().Text)
	}
	if len(p.errors) > 0 {
		return expr, fmt.Errorf("%s", strings.Join(p.errors, "; "))
	}
	return expr, nil
}

type parser struct {
	tokens []Token
	pos    int
	errors []string
	input  string
}

func newParser(input string) *parser {
	p := &parser{input: input}
	if len(input) > MaxInputSize {
		p.errors = append(p.errors, fmt.Sprintf("input too large: %d bytes (max %d)", len(input), MaxInputSize))
		p.tokens = []Token{{Type: TokenEOF}}
		return p
	}
	p.tokens = newLexer(input).lex()
	return p
}

func (p *parser) cur() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) peekType(offset int) TokenType {
	if p.pos+offset >= len(p.tokens) {
		return TokenEOF
	}
	return p.tokens[p.pos+offset].Type
}

func (p *parser) next() Token {
	t := p.cur()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *parser) errorf(format string, args ...any) {
	t := p.cur()
	p.errors = append(p.errors, fmt.Sprintf("line %d:%d: %s", t.Line, t.Col, fmt.Sprintf(format, args...)))
}

func (p *parser) parseQuery() *Query {
	q := &Query{}
	if p.cur().Type != TokenEOF && p.cur().Type != TokenPipe {
		q.Expr = p.parseExpr(0)
	}
	for p.cur().Type == TokenPipe {
		p.next()
		q.Pipes = append(q.Pipes, p.parsePipe())
	}
	if p.cur().Type != TokenEOF {
		p.errorf("unexpected trailing input starting at %q", p.cur().Text)
	}
	return q
}

// parsePipe parses `name(args)` or a bare `name arg...` command after a pipe.
func (p *parser) parsePipe() Pipe {
	if p.cur().Type != TokenWord {
		p.errorf("expected pipe command name, got %q", p.cur().Text)
		p.recoverToPipe()
		return Pipe{Name: "?"}
	}
	name := p.next().Text
	pipe := Pipe{Name: name}
	if p.cur().Type == TokenLParen {
		p.next()
		// Capture raw argument text until the matching close paren.
		depth := 1
		var args []string
		for p.cur().Type != TokenEOF {
			t := p.cur()
			if t.Type == TokenLParen {
				depth++
			}
			if t.Type == TokenRParen {
				depth--
				if depth == 0 {
					p.next()
					break
				}
			}
			args = append(args, t.Text)
			p.next()
		}
		if depth != 0 {
			p.errorf("unterminated pipe arguments for %q", name)
		}
		pipe.Args = strings.Join(args, " ")
	} else {
		// Bare arguments until the next pipe or EOF.
		var args []string
		for p.cur().Type != TokenEOF && p.cur().Type != TokenPipe {
			args = append(args, p.next().Text)
		}
		pipe.Args = strings.Join(args, " ")
	}
	return pipe
}

func (p *parser) recoverToPipe() {
	for p.cur().Type != TokenEOF && p.cur().Type != TokenPipe {
		p.next()
	}
}

// parseExpr parses a disjunction: AndExpr ( (','|OR) AndExpr )*.
func (p *parser) parseExpr(depth int) Expr {
	if depth > MaxDepth {
		p.errorf("expression nesting too deep (max %d)", MaxDepth)
		return nil
	}
	first := p.parseAnd(depth)
	terms := []Expr{first}
	for {
		t := p.cur()
		if t.Type == TokenComma || (t.Type == TokenWord && strings.EqualFold(t.Text, "OR")) {
			p.next()
			terms = append(terms, p.parseAnd(depth))
			continue
		}
		break
	}
	if len(terms) == 1 {
		return first
	}
	return &OrExpr{Terms: terms}
}

// parseAnd parses a conjunction: Unary ( ('+'|AND|adjacency) Unary )*.
func (p *parser) parseAnd(depth int) Expr {
	first := p.parseUnary(depth)
	terms := []Expr{first}
	for {
		t := p.cur()
		switch {
		case t.Type == TokenPlus:
			p.next()
			terms = append(terms, p.parseUnary(depth))
		case t.Type == TokenWord && strings.EqualFold(t.Text, "AND"):
			p.next()
			terms = append(terms, p.parseUnary(depth))
		case p.startsUnary():
			// Space-adjacent terms are an implicit AND (Event Search style):
			// event_simpleName=ProcessRollup2 FileName=cmd.exe
			terms = append(terms, p.parseUnary(depth))
		default:
			if len(terms) == 1 {
				return first
			}
			return &AndExpr{Terms: terms}
		}
	}
}

// startsUnary reports whether the current token can begin a new term.
func (p *parser) startsUnary() bool {
	switch p.cur().Type {
	case TokenBang, TokenLParen, TokenString:
		return true
	case TokenWord:
		// OR/AND keywords are handled by the callers, not new terms.
		return !strings.EqualFold(p.cur().Text, "OR") && !strings.EqualFold(p.cur().Text, "AND")
	}
	return false
}

func (p *parser) parseUnary(depth int) Expr {
	t := p.cur()
	if t.Type == TokenBang {
		p.next()
		// `!field:value` reads as a negated condition; `!(...)` as a negated group.
		if p.cur().Type == TokenWord && isOperatorType(p.peekType(1)) {
			cond := p.parseCondition()
			if c, ok := cond.(*ConditionExpr); ok {
				c.Negated = true
			}
			return cond
		}
		return &NotExpr{X: p.parsePrimary(depth)}
	}
	if t.Type == TokenWord && strings.EqualFold(t.Text, "NOT") && p.peekType(1) != TokenEOF && !isOperatorType(p.peekType(1)) {
		p.next()
		return &NotExpr{X: p.parseUnary(depth)}
	}
	return p.parsePrimary(depth)
}

func isOperatorType(tt TokenType) bool {
	switch tt {
	case TokenColon, TokenNColon, TokenTilde, TokenNTilde, TokenEq, TokenNEq, TokenGT, TokenLT, TokenGTE, TokenLTE:
		return true
	}
	return false
}

func operatorText(tt TokenType) string {
	switch tt {
	case TokenColon:
		return ":"
	case TokenNColon:
		return "!:"
	case TokenTilde:
		return "~"
	case TokenNTilde:
		return "!~"
	case TokenEq:
		return "="
	case TokenNEq:
		return "!="
	case TokenGT:
		return ">"
	case TokenLT:
		return "<"
	case TokenGTE:
		return ">="
	case TokenLTE:
		return "<="
	}
	return "?"
}

func (p *parser) parsePrimary(depth int) Expr {
	t := p.cur()
	switch t.Type {
	case TokenLParen:
		p.next()
		inner := p.parseExpr(depth + 1)
		if p.cur().Type == TokenRParen {
			p.next()
		} else {
			p.errorf("expected ')' to close group")
		}
		return &GroupExpr{X: inner}

	case TokenWord:
		if isOperatorType(p.peekType(1)) {
			return p.parseCondition()
		}
		p.next()
		return &SearchExpr{Term: t.Text}

	case TokenString:
		p.next()
		return &SearchExpr{Term: t.Text, Quoted: true}

	case TokenEOF:
		p.errorf("unexpected end of query")
		return &SearchExpr{}

	default:
		p.errorf("unexpected token %q", t.Text)
		p.next()
		return &SearchExpr{Term: t.Text}
	}
}

func (p *parser) parseCondition() Expr {
	field := p.next().Text
	op := operatorText(p.next().Type)
	value := p.parseValue()
	return &ConditionExpr{Field: field, Operator: op, Value: value}
}

func (p *parser) parseValue() Value {
	t := p.cur()
	switch t.Type {
	case TokenLBrack:
		p.next()
		var list []Value
		for p.cur().Type != TokenRBrack && p.cur().Type != TokenEOF {
			list = append(list, p.parseScalar())
			if p.cur().Type == TokenComma {
				p.next()
			}
		}
		if p.cur().Type == TokenRBrack {
			p.next()
		} else {
			p.errorf("expected ']' to close list")
		}
		if list == nil {
			list = []Value{}
		}
		return Value{List: list}
	default:
		return p.parseScalar()
	}
}

func (p *parser) parseScalar() Value {
	t := p.cur()
	switch t.Type {
	case TokenString:
		p.next()
		return Value{Scalar: t.Text, Quoted: true}
	case TokenWord:
		p.next()
		return Value{Scalar: t.Text}
	default:
		p.errorf("expected value, got %q", t.Text)
		p.next()
		return Value{}
	}
}
