package fql

import "strings"

// Expr is a node in the boolean expression tree of a Falcon query.
type Expr interface {
	render(sb *strings.Builder)
}

// Query is a parsed Falcon query: a boolean filter expression followed by
// zero or more pipe commands.
type Query struct {
	Expr  Expr
	Pipes []Pipe
}

// Pipe is a piped command such as `| timerange(24h)`.
type Pipe struct {
	Name string
	Args string // raw argument text inside the parentheses, "" when absent
}

// OrExpr is a comma-joined disjunction.
type OrExpr struct {
	Terms []Expr
}

// AndExpr is a plus-joined (or space-adjacent) conjunction.
type AndExpr struct {
	Terms []Expr
}

// NotExpr negates its child expression (`!(...)` or the NOT keyword).
type NotExpr struct {
	X Expr
}

// GroupExpr is a parenthesized sub-expression.
type GroupExpr struct {
	X Expr
}

// ConditionExpr is a single field comparison, e.g. CommandLine:'*-enc*'.
type ConditionExpr struct {
	Field    string
	Operator string // ":", "=", "~", "!:", "!~", "!=", ">", "<", ">=", "<="
	Value    Value
	Negated  bool // leading ! on the field
}

// SearchExpr is a bare search term with no field (free-text search).
type SearchExpr struct {
	Term   string
	Quoted bool
}

// Value is a condition's right-hand side: a scalar or a bracketed list.
type Value struct {
	Scalar string
	Quoted bool    // scalar came from a quoted string literal
	List   []Value // non-nil for [a, b] lists
}

// IsList reports whether the value is a bracketed list.
func (v Value) IsList() bool { return v.List != nil }

func quoteScalar(s string, quoted bool) string {
	if !quoted {
		return s
	}
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	return "'" + escaped + "'"
}

func (v Value) render(sb *strings.Builder) {
	if v.IsList() {
		sb.WriteByte('[')
		for i, item := range v.List {
			if i > 0 {
				sb.WriteString(", ")
			}
			item.render(sb)
		}
		sb.WriteByte(']')
		return
	}
	sb.WriteString(quoteScalar(v.Scalar, v.Quoted))
}

func (e *OrExpr) render(sb *strings.Builder) {
	for i, t := range e.Terms {
		if i > 0 {
			sb.WriteString(", ")
		}
		t.render(sb)
	}
}

func (e *AndExpr) render(sb *strings.Builder) {
	for i, t := range e.Terms {
		if i > 0 {
			sb.WriteString(" + ")
		}
		t.render(sb)
	}
}

func (e *NotExpr) render(sb *strings.Builder) {
	sb.WriteByte('!')
	e.X.render(sb)
}

func (e *GroupExpr) render(sb *strings.Builder) {
	sb.WriteByte('(')
	e.X.render(sb)
	sb.WriteByte(')')
}

func (e *ConditionExpr) render(sb *strings.Builder) {
	if e.Negated {
		sb.WriteByte('!')
	}
	sb.WriteString(e.Field)
	sb.WriteString(e.Operator)
	e.Value.render(sb)
}

func (e *SearchExpr) render(sb *strings.Builder) {
	sb.WriteString(quoteScalar(e.Term, e.Quoted))
}

// String renders the query back to canonical FQL text. The rendering
// round-trips: parsing the output yields an equivalent tree.
func (q *Query) String() string {
	var sb strings.Builder
	if q.Expr != nil {
		q.Expr.render(&sb)
	}
	for _, p := range q.Pipes {
		sb.WriteString(" | ")
		sb.WriteString(p.Name)
		if p.Args != "" {
			sb.WriteByte('(')
			sb.WriteString(p.Args)
			sb.WriteByte(')')
		}
	}
	return sb.String()
}

// ExprString renders a single expression node to text.
func ExprString(e Expr) string {
	if e == nil {
		return ""
	}
	var sb strings.Builder
	e.render(&sb)
	return sb.String()
}
