package fql

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, query string) *Query {
	t.Helper()
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", query, err)
	}
	return q
}

func TestParseSimpleCondition(t *testing.T) {
	q := mustParse(t, `CommandLine:'*-enc*'`)
	cond, ok := q.Expr.(*ConditionExpr)
	if !ok {
		t.Fatalf("expected ConditionExpr, got %T", q.Expr)
	}
	if cond.Field != "CommandLine" || cond.Operator != ":" || cond.Value.Scalar != "*-enc*" || !cond.Value.Quoted {
		t.Fatalf("unexpected condition: %+v", cond)
	}
}

func TestParseNegatedCondition(t *testing.T) {
	q := mustParse(t, `!ParentBaseFileName:'sccm.exe'`)
	cond, ok := q.Expr.(*ConditionExpr)
	if !ok {
		t.Fatalf("expected ConditionExpr, got %T", q.Expr)
	}
	if !cond.Negated {
		t.Fatal("expected negated condition")
	}
}

func TestParseListValue(t *testing.T) {
	q := mustParse(t, `event_simpleName:['ProcessRollup2','SyntheticProcessRollup2']`)
	cond := q.Expr.(*ConditionExpr)
	if !cond.Value.IsList() || len(cond.Value.List) != 2 {
		t.Fatalf("expected 2-item list, got %+v", cond.Value)
	}
	if cond.Value.List[1].Scalar != "SyntheticProcessRollup2" {
		t.Fatalf("unexpected list value: %+v", cond.Value.List[1])
	}
}

func TestParseAndOrPrecedence(t *testing.T) {
	// + binds tighter than , — OR of two ANDs
	q := mustParse(t, `a:'1' + b:'2', c:'3'`)
	or, ok := q.Expr.(*OrExpr)
	if !ok {
		t.Fatalf("expected OrExpr at top, got %T", q.Expr)
	}
	if len(or.Terms) != 2 {
		t.Fatalf("expected 2 OR terms, got %d", len(or.Terms))
	}
	if _, ok := or.Terms[0].(*AndExpr); !ok {
		t.Fatalf("expected AndExpr as first OR term, got %T", or.Terms[0])
	}
}

func TestParseImplicitAndAdjacency(t *testing.T) {
	// Event Search style: space-separated conditions are AND
	q := mustParse(t, `event_simpleName=ProcessRollup2 FileName=cmd.exe`)
	and, ok := q.Expr.(*AndExpr)
	if !ok {
		t.Fatalf("expected AndExpr, got %T", q.Expr)
	}
	if len(and.Terms) != 2 {
		t.Fatalf("expected 2 AND terms, got %d", len(and.Terms))
	}
}

func TestParseKeywordOperators(t *testing.T) {
	q := mustParse(t, `a:'1' AND b:'2' OR NOT c:'3'`)
	or, ok := q.Expr.(*OrExpr)
	if !ok {
		t.Fatalf("expected OrExpr, got %T", q.Expr)
	}
	if _, ok := or.Terms[1].(*NotExpr); !ok {
		t.Fatalf("expected NotExpr as second OR term, got %T", or.Terms[1])
	}
}

func TestParseGroupsAndPipes(t *testing.T) {
	q := mustParse(t, `(RemotePort:445, RemotePort:3389) | timerange(24h)`)
	if _, ok := q.Expr.(*GroupExpr); !ok {
		t.Fatalf("expected GroupExpr, got %T", q.Expr)
	}
	if len(q.Pipes) != 1 || q.Pipes[0].Name != "timerange" || q.Pipes[0].Args != "24h" {
		t.Fatalf("unexpected pipes: %+v", q.Pipes)
	}
}

func TestParseComparisons(t *testing.T) {
	q := mustParse(t, `RemotePort>1024 + Duration<=300`)
	and := q.Expr.(*AndExpr)
	first := and.Terms[0].(*ConditionExpr)
	if first.Operator != ">" || first.Value.Scalar != "1024" {
		t.Fatalf("unexpected comparison: %+v", first)
	}
}

func TestParseWindowsPathValue(t *testing.T) {
	q := mustParse(t, `ImageFileName:'C:\Windows\System32\cmd.exe'`)
	cond := q.Expr.(*ConditionExpr)
	if cond.Value.Scalar != `C:\Windows\System32\cmd.exe` {
		t.Fatalf("path mangled: %q", cond.Value.Scalar)
	}
}

func TestParseEscapedQuote(t *testing.T) {
	q := mustParse(t, `CommandLine:'it\'s fine'`)
	cond := q.Expr.(*ConditionExpr)
	if cond.Value.Scalar != "it's fine" {
		t.Fatalf("escape mishandled: %q", cond.Value.Scalar)
	}
}

func TestParseUnterminatedString(t *testing.T) {
	_, err := Parse(`CommandLine:'*enc`)
	if err == nil {
		t.Fatal("expected error for unterminated string")
	}
}

func TestParseUnbalancedParens(t *testing.T) {
	q, err := Parse(`(a:'1' + b:'2'`)
	if err == nil {
		t.Fatal("expected error for unbalanced parens")
	}
	if q == nil || q.Expr == nil {
		t.Fatal("expected partial AST despite error")
	}
}

func TestParseInputTooLarge(t *testing.T) {
	_, err := Parse(strings.Repeat("a", MaxInputSize+1))
	if err == nil {
		t.Fatal("expected error for oversized input")
	}
}

func TestParseDeepNesting(t *testing.T) {
	query := strings.Repeat("(", MaxDepth+10) + "a:'1'" + strings.Repeat(")", MaxDepth+10)
	_, err := Parse(query)
	if err == nil {
		t.Fatal("expected depth error")
	}
}

func TestRoundTrip(t *testing.T) {
	queries := []string{
		`CommandLine:'*regsvr32*/s /u /n /i:http*scrobj*'`,
		`event_simpleName:['ProcessRollup2'] + ImageFileName:'*\powershell.exe' + (CommandLine:'*-enc*', CommandLine:'*-encodedcommand*')`,
		`!ParentBaseFileName:['sccm.exe','schtasks.exe'] + CommandLine~'.*mimikatz.*'`,
		`(RemotePort:445, RemotePort:3389) | timerange(24h)`,
		`event_platform:['Win'] + event_simpleName:['DnsRequest'] + DomainName:'*.evil.example'`,
		`RemotePort>1024 + Duration<=300`,
	}
	for _, original := range queries {
		q1 := mustParse(t, original)
		rendered := q1.String()
		q2, err := Parse(rendered)
		if err != nil {
			t.Fatalf("re-parse of rendered %q failed: %v", rendered, err)
		}
		if q1.String() != q2.String() {
			t.Fatalf("round-trip not stable:\n first: %s\nsecond: %s", q1.String(), q2.String())
		}
	}
}

func TestParseExpressionTrailing(t *testing.T) {
	_, err := ParseExpression(`a:'1' | timerange(24h)`)
	if err == nil {
		t.Fatal("expected trailing-input error from ParseExpression")
	}
}
