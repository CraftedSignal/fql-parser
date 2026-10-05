package fql

import (
	"strings"
	"testing"
)

func TestASTEdgeRendering(t *testing.T) {
	if got := ExprString(nil); got != "" {
		t.Fatalf("nil expression rendered %q", got)
	}
	if got := ExprString(&NotExpr{X: &SearchExpr{Term: "mimikatz"}}); got != "!mimikatz" {
		t.Fatalf("unexpected not rendering: %q", got)
	}
	if got := ExprString(&SearchExpr{Term: "two words", Quoted: true}); got != "'two words'" {
		t.Fatalf("unexpected quoted search rendering: %q", got)
	}
	q := &Query{
		Expr: &ConditionExpr{
			Field:    "event_simpleName",
			Operator: ":",
			Value: Value{List: []Value{
				{Scalar: "ProcessRollup2"},
				{Scalar: "SyntheticProcessRollup2", Quoted: true},
			}},
		},
		Pipes: []Pipe{{Name: "timerange", Args: "24h"}},
	}
	if got := q.String(); got != "event_simpleName:[ProcessRollup2, 'SyntheticProcessRollup2'] | timerange(24h)" {
		t.Fatalf("unexpected query rendering: %q", got)
	}
}

func TestExtractorRecoveryAndHelperEdges(t *testing.T) {
	oldHook := extractHook
	defer func() { extractHook = oldHook }()
	extractHook = func(string) { panic("forced panic") }
	result := ExtractConditions("anything")
	if result == nil || len(result.Errors) == 0 || !strings.Contains(result.Errors[0], "internal extraction failure") {
		t.Fatalf("expected panic recovery error, got %#v", result)
	}

	extractHook = nil
	ex := &extractor{result: &ParseResult{}}
	if _, ok := ex.mergeSameFieldOr(&OrExpr{Terms: []Expr{&SearchExpr{Term: "free"}}}, false, ""); ok {
		t.Fatal("free-text OR term must not merge into field alternatives")
	}
	if _, ok := ex.mergeSameFieldOr(&OrExpr{Terms: []Expr{
		&ConditionExpr{Field: "A", Operator: ":", Value: Value{List: []Value{{Scalar: "1"}}}},
		&ConditionExpr{Field: "A", Operator: ":", Value: Value{Scalar: "2"}},
	}}, false, ""); ok {
		t.Fatal("list-valued OR term must not merge into scalar alternatives")
	}
	if _, ok := ex.mergeSameFieldOr(&OrExpr{Terms: []Expr{
		&ConditionExpr{Field: "A", Operator: ":", Value: Value{Scalar: "1"}},
		&ConditionExpr{Field: "B", Operator: ":", Value: Value{Scalar: "2"}},
	}}, false, ""); ok {
		t.Fatal("different-field OR terms must not merge into alternatives")
	}

	deduped := DeduplicateConditions([]Condition{
		{Field: "A", Operator: ":", Value: "1", Negated: true},
		{Field: "A", Operator: ":", Value: "1", Negated: true},
		{Field: "A", Operator: ":", Value: "1"},
	})
	if len(deduped) != 2 {
		t.Fatalf("negated and positive conditions should dedupe separately, got %+v", deduped)
	}

	if got := GetEventTypeFromConditions(nil); got != "" {
		t.Fatalf("nil result event type = %q", got)
	}
	for _, query := range []string{
		`!event_simpleName=ProcessRollup2`,
		`event_simpleName='*Rollup*'`,
		`event_simpleName=''`,
	} {
		if got := GetEventTypeFromConditions(ExtractConditions(query)); got != "" {
			t.Fatalf("%s should not produce event type, got %q", query, got)
		}
	}
}

func TestLexerEdgeBranches(t *testing.T) {
	l := newLexer("a\nb")
	if l.advance() != 'a' || l.advance() != '\n' || l.line != 2 || l.col != 1 {
		t.Fatalf("newline advance did not update position: line=%d col=%d", l.line, l.col)
	}
	if got := l.peekAt(0); got != 'b' {
		t.Fatalf("peekAt current got %q", got)
	}
	if got := l.peekAt(100); got != 0 {
		t.Fatalf("peekAt beyond end got %q", got)
	}

	tokens := newLexer(`a<5 b>=4 c!~'x'`).lex()
	var foundLT, foundGTE, foundNTilde bool
	for _, tok := range tokens {
		switch tok.Type {
		case TokenLT:
			foundLT = true
		case TokenGTE:
			foundGTE = true
		case TokenNTilde:
			foundNTilde = true
		}
	}
	if !foundLT || !foundGTE || !foundNTilde {
		t.Fatalf("missing expected tokens in %#v", tokens)
	}
	illegal := newLexer(`'unterminated`).lex()
	if len(illegal) == 0 || illegal[0].Type != TokenIllegal {
		t.Fatalf("unterminated string should lex as illegal, got %#v", illegal)
	}
}

func TestNormalizeTypographyEdges(t *testing.T) {
	input := "\ufeffCommandLine:\u201cwhoami\u201d\u00a0\u200b"
	if got := NormalizeQuery(input); got != `CommandLine:"whoami"` {
		t.Fatalf("unexpected normalized query: %q", got)
	}
	insideASCIIQuote := NormalizeQuery(`FileDescription:'GnuPG’s OpenPGP'`)
	if insideASCIIQuote != `FileDescription:'GnuPG’s OpenPGP'` {
		t.Fatalf("smart apostrophe inside quoted value should be preserved, got %q", insideASCIIQuote)
	}
	doubleInsideSingleQuote := NormalizeQuery(`Field:'“quoted”'`)
	if doubleInsideSingleQuote != `Field:'“quoted”'` {
		t.Fatalf("smart double quotes inside single-quoted value should be preserved, got %q", doubleInsideSingleQuote)
	}
	escaped := NormalizeQuery(`Field:'a\'b’`)
	if escaped != `Field:'a\'b’` {
		t.Fatalf("escaped quoted content should preserve internal smart quote, got %q", escaped)
	}
	fenced := NormalizeQuery("```this-info-string-is-too-long\nCommandLine:'x'\n```")
	if fenced != "this-info-string-is-too-long\nCommandLine:'x'" {
		t.Fatalf("unknown code fence info string should be preserved, got %q", fenced)
	}
	invalidInfo := NormalizeQuery("```bad!info\nCommandLine:'x'\n```")
	if invalidInfo != "bad!info\nCommandLine:'x'" {
		t.Fatalf("invalid code fence info string should be preserved, got %q", invalidInfo)
	}
}

func TestParserEdgeBranches(t *testing.T) {
	p := newParser("field")
	p.pos = len(p.tokens)
	if p.cur().Type != TokenEOF || p.peekType(100) != TokenEOF {
		t.Fatalf("parser bounds should yield EOF")
	}

	for _, query := range []string{
		`a:'1' | | next`,
		`a:'1' | ( bad | next`,
		`a:'1' | cmd(foo(bar))`,
		`a:'1' | cmd(foo`,
		`a:'1' | timerange 24h now`,
		`a<5`,
		`a>=5`,
		`a!~'x'`,
		`@`,
		`(`,
	} {
		_, _ = Parse(query)
	}
	if expr, err := ParseExpression(""); err == nil || expr == nil {
		t.Fatalf("empty expression should return partial expr and error, expr=%#v err=%v", expr, err)
	}
	if expr, err := ParseExpression(`a:'1'`); err != nil || expr == nil {
		t.Fatalf("valid expression should parse cleanly, expr=%#v err=%v", expr, err)
	}
	if got := operatorText(TokenIllegal); got != "?" {
		t.Fatalf("unexpected unknown operator text %q", got)
	}

	noField := newParser(`(a:'1')`)
	if index := noField.conditionOperatorIndex(); index != -1 {
		t.Fatalf("non-word current token should have no condition operator, got %d", index)
	}
	noOperator := newParser(`field`)
	if index := noOperator.conditionOperatorIndex(); index != -1 {
		t.Fatalf("field without operator should have no condition operator, got %d", index)
	}
	noEOF := &parser{tokens: []Token{{Type: TokenWord, Text: "field", Pos: 0}}}
	if index := noEOF.conditionOperatorIndex(); index != -1 {
		t.Fatalf("parser without trailing EOF should still have no condition operator, got %d", index)
	}
	noOperator.parseCondition()
	if len(noOperator.errors) == 0 {
		t.Fatal("direct parseCondition without operator should report an error")
	}
}
