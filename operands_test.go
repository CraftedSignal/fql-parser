package fql

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Legacy fixtures assert the existing flat fields independently of the new RHS.
func legacyCondition(c Condition) Condition { c.RHS = nil; return c }
func legacyConditions(in []Condition) []Condition {
	out := make([]Condition, len(in))
	for i, c := range in {
		out[i] = legacyCondition(c)
	}
	return out
}

// Canonical rendering intentionally changes quote style and list whitespace.
// Compare every other AST and semantic field during round-trip tests.
func withoutValueSource(v *Value) {
	v.Source = ""
	for i := range v.List {
		withoutValueSource(&v.List[i])
	}
}
func withoutASTSource(q *Query) *Query {
	var visit func(Expr)
	visit = func(e Expr) {
		switch n := e.(type) {
		case *ConditionExpr:
			withoutValueSource(&n.Value)
		case *AndExpr:
			for _, x := range n.Terms {
				visit(x)
			}
		case *OrExpr:
			for _, x := range n.Terms {
				visit(x)
			}
		case *GroupExpr:
			visit(n.X)
		case *NotExpr:
			visit(n.X)
		}
	}
	visit(q.Expr)
	return q
}
func withoutOperandSource(o *Operand) {
	if o == nil {
		return
	}
	o.Source = ""
	for i := range o.Items {
		withoutOperandSource(&o.Items[i])
	}
}
func withoutSemanticSource(q *SemanticQuery) *SemanticQuery {
	for i := range q.Conditions {
		withoutOperandSource(q.Conditions[i].RHS)
	}
	var visit func(*SemanticExpression)
	visit = func(e *SemanticExpression) {
		if e == nil {
			return
		}
		if e.Condition != nil {
			withoutOperandSource(e.Condition.RHS)
		}
		for _, child := range e.Children {
			visit(child)
		}
	}
	visit(q.Expression)
	return q
}
func withoutExtractionSource(r *ParseResult) *ParseResult {
	for i := range r.Conditions {
		withoutOperandSource(r.Conditions[i].RHS)
	}
	return r
}

func TestTypedScalarOperandsPreserveAuthoredSyntax(t *testing.T) {
	cases := []struct {
		input, source, value string
		kind                 OperandKind
		quoted               bool
	}{
		{`42`, `42`, `42`, OperandNumber, false},
		{`9.0`, `9.0`, `9.0`, OperandNumber, false},
		{`-0.125`, `-0.125`, `-0.125`, OperandNumber, false},
		{`1e3`, `1e3`, `1e3`, OperandNumber, false},
		{`1e+3`, `1e+3`, `1e+3`, OperandNumber, false},
		{`+0.125`, `+0.125`, `+0.125`, OperandNumber, false},
		{`+1e3`, `+1e3`, `+1e3`, OperandNumber, false},
		{`'42'`, `'42'`, `42`, OperandString, true},
		{`true`, `true`, `true`, OperandBool, false},
		{`'true'`, `'true'`, `true`, OperandString, true},
		{`false`, `false`, `false`, OperandBool, false},
		{`null`, `null`, ``, OperandNull, false},
		{`'null'`, `'null'`, `null`, OperandString, true},
		{`''`, `''`, ``, OperandString, true},
		{`+900719925474099312345`, `+900719925474099312345`, `+900719925474099312345`, OperandNumber, false},
		{`-0009`, `-0009`, `-0009`, OperandNumber, false},
		{`cmd.exe`, `cmd.exe`, `cmd.exe`, OperandBare, false},
		{`TRUE`, `TRUE`, `TRUE`, OperandBare, false},
		{`'2026-11-15'`, `'2026-11-15'`, `2026-11-15`, OperandString, true},
		{`"café"`, `"café"`, `café`, OperandString, true},
		{`'a\'b'`, `'a\'b'`, `a'b`, OperandString, true},
		{`"a\"b"`, `"a\"b"`, `a"b`, OperandString, true},
		{`'C:\\Windows\d\n'`, `'C:\\Windows\d\n'`, `C:\Windows\d\n`, OperandString, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			query := `x:` + tc.input + `   `
			parsed, err := Parse(query)
			if err != nil {
				t.Fatal(err)
			}
			ast := parsed.Expr.(*ConditionExpr).Value
			if ast.Source != tc.source || ast.Quoted != tc.quoted || ast.Scalar != tc.value && tc.kind != OperandNull {
				t.Fatalf("AST lost source/quoted/value: %+v", ast)
			}
			want := &Operand{Kind: tc.kind, Source: tc.source, Value: tc.value}
			got := ExtractConditions(query)
			if len(got.Errors) != 0 || len(got.Conditions) != 1 || !reflect.DeepEqual(got.Conditions[0].RHS, want) {
				t.Fatalf("extraction RHS: %+v", got)
			}
			if got.Conditions[0].Value != ast.Scalar {
				t.Fatalf("legacy value changed: %+v", got.Conditions[0])
			}
			encoded, err := json.Marshal(got.Conditions[0])
			if err != nil {
				t.Fatal(err)
			}
			var decoded Condition
			if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(decoded.RHS, want) {
				t.Fatalf("scalar JSON lost type/source: %s: %+v: %v", encoded, decoded, err)
			}
			semantic := AnalyzeQuery(query)
			if len(semantic.Errors) != 0 || len(semantic.Conditions) != 1 || !reflect.DeepEqual(semantic.Conditions[0].RHS, want) || !reflect.DeepEqual(semantic.Expression.Condition.RHS, want) {
				t.Fatalf("semantic RHS: %+v", semantic)
			}
		})
	}
}

func TestTypedListAndUnionProvenance(t *testing.T) {
	for _, tc := range []struct {
		query     string
		items     []Operand
		bracketed bool
	}{
		{`x:[]`, []Operand{}, true},
		{`x:['']`, []Operand{{Kind: OperandString, Source: `''`}}, true},
		{`x:[42, '42',true,'true',null,'null']`, []Operand{
			{Kind: OperandNumber, Source: `42`, Value: `42`}, {Kind: OperandString, Source: `'42'`, Value: `42`},
			{Kind: OperandBool, Source: `true`, Value: `true`}, {Kind: OperandString, Source: `'true'`, Value: `true`},
			{Kind: OperandNull, Source: `null`}, {Kind: OperandString, Source: `'null'`, Value: `null`},
		}, true},
		{`x:42,x:'42'`, []Operand{{Kind: OperandNumber, Source: `42`, Value: `42`}, {Kind: OperandString, Source: `'42'`, Value: `42`}}, false},
	} {
		t.Run(tc.query, func(t *testing.T) {
			a := AnalyzeQuery(tc.query)
			if len(a.Errors) != 0 || len(a.Conditions) != 1 {
				t.Fatalf("analysis: %+v", a)
			}
			c := a.Conditions[0]
			if c.RHS == nil || c.RHS.Kind != OperandList || c.Bracketed != tc.bracketed || !reflect.DeepEqual(c.RHS.Items, tc.items) {
				t.Fatalf("list/union provenance: %+v", c)
			}
			if tc.bracketed && c.RHS.Source != strings.TrimPrefix(tc.query, `x:`) || !tc.bracketed && c.RHS.Source != "" {
				t.Fatalf("wrong root source: %+v", c.RHS)
			}
			if len(tc.items) > 0 && c.Value != tc.items[0].Value {
				t.Fatalf("legacy first value changed: %+v", c)
			}
			encoded, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			var decoded Condition
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.RHS == nil || decoded.RHS.Kind != c.RHS.Kind || decoded.RHS.Source != c.RHS.Source || !reflect.DeepEqual(decoded.RHS.Items, c.RHS.Items) && len(c.RHS.Items) != 0 || decoded.Bracketed != c.Bracketed {
				t.Fatalf("JSON lost RHS: %s: %+v: %v", encoded, decoded, err)
			}
			if !tc.bracketed {
				if a.Expression.Operator != "or" || len(a.Expression.Children) != 2 || a.Expression.Children[0].Condition.RHS.Kind != OperandNumber || a.Expression.Children[1].Condition.RHS.Kind != OperandString {
					t.Fatalf("OR leaves collapsed: %+v", a.Expression)
				}
			}
		})
	}
}

func TestMixedBracketItemsDoNotCollapseIntoQuotedStrings(t *testing.T) {
	mixed := AnalyzeQuery(`x:[42,'42']`)
	quoted := AnalyzeQuery(`x:['42','42']`)
	if len(mixed.Errors) != 0 || len(quoted.Errors) != 0 || reflect.DeepEqual(mixed, quoted) || mixed.Conditions[0].RHS.Items[0].Kind != OperandNumber || quoted.Conditions[0].RHS.Items[0].Kind != OperandString {
		t.Fatalf("mixed and quoted brackets collided: %+v / %+v", mixed, quoted)
	}
	if !reflect.DeepEqual(legacyConditions(mixed.Conditions), legacyConditions(quoted.Conditions)) {
		t.Fatal("legacy bracket projection changed")
	}
}

func TestMalformedListHasNoAuthoritativeRHS(t *testing.T) {
	result := ExtractConditions(`x:['ok'`)
	if len(result.Errors) == 0 || len(result.Conditions) != 1 || result.Conditions[0].RHS != nil {
		t.Fatalf("incomplete list fabricated RHS: %+v", result)
	}
}

func TestTypedOperandDedupAndOwnership(t *testing.T) {
	num := Condition{Field: "x", Operator: ":", Value: "42", RHS: &Operand{Kind: OperandNumber, Source: "42", Value: "42"}}
	str := Condition{Field: "x", Operator: ":", Value: "42", RHS: &Operand{Kind: OperandString, Source: "'42'", Value: "42"}}
	legacy := Condition{Field: "x", Operator: ":", Value: "42"}
	if got := DeduplicateConditions([]Condition{num, str, legacy, num}); len(got) != 3 {
		t.Fatalf("typed identity collapsed: %+v", got)
	}
	list := Condition{Field: "x", Operator: ":", RHS: &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandBare, Source: "a", Value: "a"}}}}
	adversary := list
	adversary.RHS = &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandBare, Source: "a", Value: "a"}, {Kind: OperandBare, Source: "", Value: ""}}}
	if got := DeduplicateConditions([]Condition{list, adversary}); len(got) != 2 {
		t.Fatalf("list identity collided: %+v", got)
	}
	a := AnalyzeQuery(`x:[42,'42']+x:[42,'42']`)
	if len(a.Errors) != 0 || len(a.Conditions) != 2 {
		t.Fatalf("analysis: %+v", a)
	}
	a.Conditions[0].RHS.Items[0].Value = "mutated"
	if a.Conditions[1].RHS.Items[0].Value != "42" || a.Expression.Children[0].Condition.RHS.Items[0].Value != "42" || AnalyzeQuery(`x:[42,'42']`).Conditions[0].RHS.Items[0].Value != "42" {
		t.Fatal("semantic RHS aliased across independent views")
	}
}

func TestOperandSourceStopsAtAuthoredBoundary(t *testing.T) {
	for _, tc := range []struct{ query, source string }{
		{`x:'a'`, `'a'`},
		{`x:"a"+gate:'yes'`, `"a"`},
		{`x:'a',gate:'yes'`, `'a'`},
		{`x:[42, '42'  ]+gate:'yes'`, `[42, '42'  ]`},
		{`x:42   `, `42`},
		{`x:你好`, `你好`},
	} {
		t.Run(tc.query, func(t *testing.T) {
			a := AnalyzeQuery(tc.query)
			if len(a.Errors) != 0 || len(a.Conditions) == 0 || a.Conditions[0].RHS == nil || a.Conditions[0].RHS.Source != tc.source {
				t.Fatalf("source boundary: %+v", a)
			}
		})
	}
}

func TestOperandTopologyAndUnionBarriers(t *testing.T) {
	for _, tc := range []struct {
		query     string
		flat      int
		firstKind OperandKind
	}{
		{`x:42,x:'42'`, 1, OperandList},
		{`x:42,x:42`, 1, OperandList},
		{`x:!42,x:!'42'`, 2, OperandNumber},
		{`!x:42,!x:'42'`, 2, OperandNumber},
		{`(x:42),(x:'42'+gate:'yes')`, 3, OperandNumber},
	} {
		t.Run(tc.query, func(t *testing.T) {
			a := AnalyzeQuery(tc.query)
			if len(a.Errors) != 0 || len(a.Conditions) != tc.flat || a.Conditions[0].RHS == nil || a.Conditions[0].RHS.Kind != tc.firstKind {
				t.Fatalf("flat topology changed: %+v", a)
			}
			if a.Expression == nil || a.Expression.Operator != "or" {
				t.Fatalf("complete OR topology lost: %+v", a.Expression)
			}
			if tc.firstKind == OperandList {
				if len(a.Expression.Children) != 2 || a.Expression.Children[0].Condition.RHS.Kind != OperandNumber || a.Expression.Children[1].Condition.RHS.Kind != map[bool]OperandKind{true: OperandString, false: OperandNumber}[strings.Contains(tc.query, "'42'")] {
					t.Fatalf("typed leaves lost: %+v", a.Expression)
				}
			}
		})
	}
	a := AnalyzeQuery(`!(x:42,x:'42')`)
	if len(a.Errors) != 0 || a.Expression.Operator != "not" || a.Expression.Children[0].Operator != "group" || a.Expression.Children[0].Children[0].Operator != "or" || len(a.Conditions) != 1 || !a.Conditions[0].Negated || a.Conditions[0].RHS.Kind != OperandList {
		t.Fatalf("NOT/group topology lost: %+v", a)
	}
}

func TestOperandIdentitySeparatesBoundariesAndOwnershipContract(t *testing.T) {
	first := Condition{Field: "x", Operator: ":", RHS: &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandBare, Source: "a:b", Value: "c"}}}}
	second := Condition{Field: "x", Operator: ":", RHS: &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandBare, Source: "a", Value: "b:c"}}}}
	third := Condition{Field: "x", Operator: ":", RHS: &Operand{Kind: OperandList, Items: []Operand{{Kind: OperandBare, Source: "a:b", Value: "c"}, {Kind: OperandBare}}}}
	if got := DeduplicateConditions([]Condition{first, second, third, first}); len(got) != 3 {
		t.Fatalf("operand identity collided: %+v", got)
	}
	got := DeduplicateConditions([]Condition{first})
	got[0].RHS.Items[0].Value = "shared"
	if first.RHS.Items[0].Value != "shared" {
		t.Fatal("documented first-occurrence shallow ownership changed")
	}
}

func TestManyListOperandsRemainWithinInputBound(t *testing.T) {
	const count = 10000
	query := `x:[` + strings.Repeat(`1234567890, `, count-1) + `1234567890]`
	if len(query) >= MaxInputSize || len(query) < MaxInputSize*9/10 {
		t.Fatalf("fixture not near bound: %d", len(query))
	}
	a := AnalyzeQuery(query)
	if len(a.Errors) != 0 || len(a.Conditions) != 1 || a.Conditions[0].RHS == nil || len(a.Conditions[0].RHS.Items) != count || len(a.Conditions[0].RHS.Source) != len(query)-2 {
		t.Fatalf("near-limit list lost metadata: errors=%v conditions=%d", a.Errors, len(a.Conditions))
	}
}
