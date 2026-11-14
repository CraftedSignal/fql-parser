package fql

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestBracketProvenanceJSON(t *testing.T) {
	for _, query := range []string{`name:['Alpha']`, `name:['Alpha','Beta']`, `name:[]`} {
		t.Run(query, func(t *testing.T) {
			result := ExtractConditions(query)
			if len(result.Errors) != 0 || len(result.Conditions) != 1 {
				t.Fatalf("unexpected extraction: %+v", result)
			}
			encoded, err := json.Marshal(result.Conditions[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), `"bracketed":true`) {
				t.Fatalf("bracket origin missing from JSON: %s", encoded)
			}
		})
	}
}

func TestDeduplicatePreservesLiteralIdentity(t *testing.T) {
	for _, operator := range []string{":", ":!", ":~", ":!~"} {
		query := "name" + operator + "['same','A']+name" + operator + "['same','a']"
		result := ExtractConditions(query)
		if len(result.Errors) != 0 || len(result.Conditions) != 2 {
			t.Fatalf("unexpected extraction for %q: %+v", query, result)
		}
		if got := DeduplicateConditions(result.Conditions); len(got) != 2 {
			t.Errorf("literal case erased for %q: %+v", query, got)
		}
	}
	cases := []struct {
		name string
		a, b []string
	}{
		{"delimiter", []string{"same", "a\x01b"}, []string{"same", "a", "b"}},
		{"empty element", nil, []string{""}},
		{"arbitrary bytes", []string{"same", "\xff"}, []string{"same", "\xfe"}},
		{"invalid UTF8 versus replacement", []string{"\xff"}, []string{"\ufffd"}},
		{"order", []string{"a", "b"}, []string{"b", "a"}},
		{"repetitions", []string{"a"}, []string{"a", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := []Condition{{Field: "name", Operator: ":", Value: "same", Alternatives: tc.a}, {Field: "name", Operator: ":", Value: "same", Alternatives: tc.b}}
			if got := DeduplicateConditions(input); !reflect.DeepEqual(got, input) {
				t.Fatalf("distinct alternative bytes or boundaries erased: %#v", got)
			}
		})
	}
}

func TestBracketProvenanceRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		query     string
		bracketed bool
		value     string
		alts      []string
	}{
		{`name:['Alpha']`, true, "Alpha", []string{"Alpha"}},
		{`name:['Alpha','Beta']`, true, "Alpha", []string{"Alpha", "Beta"}},
		{`name:[]`, true, "", []string{}},
		{`name:'Alpha'`, false, "Alpha", nil},
		{`name:''`, false, "", nil},
		{`name:'[Alpha]'`, false, "[Alpha]", nil},
		{`name:'prod*'`, false, "prod*", nil},
		{`name:*'prod*'`, false, "prod*", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			ast, ok := q.Expr.(*ConditionExpr)
			if !ok || ast.Value.IsList() != tc.bracketed {
				t.Fatalf("wrong AST value shape: %#v", q.Expr)
			}
			expr, err := ParseExpression(tc.query)
			if err != nil || !reflect.DeepEqual(expr, q.Expr) {
				t.Fatalf("ParseExpression: %#v, %v", expr, err)
			}
			round, err := Parse(q.String())
			if err != nil || !reflect.DeepEqual(round, q) {
				t.Fatalf("AST round trip: %#v, %v", round, err)
			}
			extracted := ExtractConditions(tc.query)
			semantic := AnalyzeQuery(tc.query)
			if len(extracted.Errors) != 0 || len(semantic.Errors) != 0 || len(extracted.Conditions) != 1 || len(semantic.Conditions) != 1 || semantic.Expression == nil || semantic.Expression.Condition == nil {
				t.Fatalf("unexpected extraction/analysis: %+v / %+v", extracted, semantic)
			}
			for _, c := range []Condition{extracted.Conditions[0], semantic.Conditions[0], *semantic.Expression.Condition} {
				if c.Bracketed != tc.bracketed || c.Value != tc.value || !slices.Equal(c.Alternatives, tc.alts) {
					t.Fatalf("wrong condition: %#v", c)
				}
				encoded, err := json.Marshal(c)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), `"bracketed"`) != tc.bracketed {
					t.Fatalf("wrong JSON flag presence: %s", encoded)
				}
				var decoded Condition
				if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Bracketed != tc.bracketed || decoded.Value != tc.value || !slices.Equal(decoded.Alternatives, c.Alternatives) {
					t.Fatalf("JSON lost metadata: %#v, %v", decoded, err)
				}
			}
			if !reflect.DeepEqual(ExtractConditions(round.String()), extracted) || !reflect.DeepEqual(AnalyzeQuery(round.String()), semantic) {
				t.Fatal("render/reparse changed extracted or semantic metadata")
			}
			encoded, err := json.Marshal(semantic)
			if err != nil {
				t.Fatal(err)
			}
			var decoded SemanticQuery
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Conditions[0].Bracketed != tc.bracketed || decoded.Expression.Condition.Bracketed != tc.bracketed {
				t.Fatalf("semantic JSON lost provenance: %+v, %v", decoded, err)
			}
		})
	}
}

func TestBracketProvenanceOperatorsAndNegation(t *testing.T) {
	for _, operator := range []string{":", ":!", ":~", ":!~", "!:", "!=", "!~"} {
		for _, rhs := range []string{`['Alpha']`, `['Alpha','Beta']`} {
			for _, prefix := range []string{"", "!", "NOT ", "!("} {
				query := prefix + "name" + operator + rhs
				if prefix == "!(" {
					query += ")"
				}
				t.Run(query, func(t *testing.T) {
					r := ExtractConditions(query)
					a := AnalyzeQuery(query)
					if len(r.Errors) != 0 || len(a.Errors) != 0 || len(r.Searches) != 0 || len(a.Searches) != 0 || len(r.Conditions) != 1 || len(a.Conditions) != 1 {
						t.Fatalf("unexpected results: %+v / %+v", r, a)
					}
					want := Condition{Field: "name", Operator: operator, Value: "Alpha", Negated: prefix != "", CaseInsensitive: strings.Contains(operator, "~"), Bracketed: true, Alternatives: []string{"Alpha"}}
					if strings.Contains(rhs, "Beta") {
						want.Alternatives = append(want.Alternatives, "Beta")
					}
					if !reflect.DeepEqual(r.Conditions[0], want) || !reflect.DeepEqual(a.Conditions[0], want) {
						t.Fatalf("flat metadata changed: %+v / %+v; want %+v", r.Conditions, a.Conditions, want)
					}
					leaf := a.Expression
					if prefix == "NOT " || prefix == "!(" {
						if leaf.Operator != "not" || len(leaf.Children) != 1 {
							t.Fatalf("lost NOT topology: %#v", leaf)
						}
						leaf = leaf.Children[0]
						if prefix == "!(" {
							if leaf.Operator != "group" || len(leaf.Children) != 1 {
								t.Fatalf("lost group topology: %#v", leaf)
							}
							leaf = leaf.Children[0]
						}
					}
					want.Negated = prefix == "!"
					if leaf.Operator != "condition" || leaf.Condition == nil || !reflect.DeepEqual(*leaf.Condition, want) {
						t.Fatalf("semantic leaf metadata changed: %#v; want %+v", leaf, want)
					}
					canonical := assertNegationRoundTrip(t, query)
					round := AnalyzeQuery(canonical)
					if !reflect.DeepEqual(round.Conditions, a.Conditions) {
						t.Fatalf("round trip changed flat metadata: %+v", round.Conditions)
					}
					// Rendering can canonicalize NOT field to !field. Compare the
					// leaf's source facts independently of that existing polarity move.
					roundLeaf := round.Expression
					for roundLeaf.Condition == nil {
						roundLeaf = roundLeaf.Children[0]
					}
					if roundLeaf.Condition.Bracketed != leaf.Condition.Bracketed || roundLeaf.Condition.Operator != operator || !reflect.DeepEqual(roundLeaf.Condition.Alternatives, want.Alternatives) {
						t.Fatalf("round trip changed leaf provenance: %+v", roundLeaf.Condition)
					}
				})
			}
		}
	}
}

func TestBracketProvenanceMergeBoundaries(t *testing.T) {
	brackets := ExtractConditions(`name:['Alpha','Beta']`).Conditions
	merged := ExtractConditions(`(name:'Alpha',name:'Beta')`).Conditions
	if len(brackets) != 1 || len(merged) != 1 || !brackets[0].Bracketed || merged[0].Bracketed {
		t.Fatalf("lost source distinction: %+v / %+v", brackets, merged)
	}
	legacyBracket := brackets[0]
	legacyBracket.Bracketed = false
	if !reflect.DeepEqual(legacyBracket, merged[0]) || len(DeduplicateConditions(append(brackets, merged...))) != 2 {
		t.Fatal("legacy projections differ or bracket provenance was deduplicated")
	}
	empty := append(ExtractConditions(`name:[]`).Conditions, ExtractConditions(`name:''`).Conditions...)
	if got := DeduplicateConditions(empty); len(got) != 2 || !got[0].Bracketed || got[1].Bracketed {
		t.Fatalf("empty brackets lost their identity: %+v", got)
	}
	for _, tc := range []struct {
		query        string
		flat, leaves []bool
	}{
		{`(name:'Alpha',name:'Beta')`, []bool{false}, []bool{false, false}},
		{`!(name:'Alpha',name:'Beta')`, []bool{false}, []bool{false, false}},
		{`(!name:'Alpha',!name:'Beta')`, []bool{false, false}, []bool{false, false}},
		{`(name:!'Alpha',name:!'Beta')`, []bool{false, false}, []bool{false, false}},
		{`(name:['Alpha'],name:['Beta'])`, []bool{true, true}, []bool{true, true}},
		{`(name:['Alpha'],name:'Beta')`, []bool{true, false}, []bool{true, false}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			a := AnalyzeQuery(tc.query)
			if len(a.Errors) != 0 || len(a.Conditions) != len(tc.flat) {
				t.Fatalf("unexpected merge: %+v", a)
			}
			for i, c := range a.Conditions {
				if c.Bracketed != tc.flat[i] || i > 0 && c.LogicalOp != "OR" {
					t.Fatalf("wrong flat origin or connector: %+v", a.Conditions)
				}
			}
			expr := a.Expression
			if strings.HasPrefix(tc.query, "!(") {
				if expr.Operator != "not" || len(expr.Children) != 1 {
					t.Fatalf("missing NOT: %#v", expr)
				}
				expr = expr.Children[0]
			}
			if expr.Operator != "group" || len(expr.Children) != 1 || expr.Children[0].Operator != "or" || len(expr.Children[0].Children) != len(tc.leaves) {
				t.Fatalf("lost group/OR topology: %#v", expr)
			}
			for i, leaf := range expr.Children[0].Children {
				if leaf.Condition == nil || leaf.Condition.Bracketed != tc.leaves[i] {
					t.Fatalf("wrong semantic leaf: %#v", leaf)
				}
			}
		})
	}
}

func TestDeduplicatePredicateMetadataAndOwnership(t *testing.T) {
	base := Condition{Field: "Name", Operator: ":", Value: "same", Alternatives: []string{"same", "A"}, Bracketed: true, LogicalOp: "AND"}
	for _, tc := range []struct {
		name   string
		change func(*Condition)
	}{
		{"brackets", func(c *Condition) { c.Bracketed = false }},
		{"polarity", func(c *Condition) { c.Negated = true }},
		{"matching", func(c *Condition) { c.CaseInsensitive = true }},
		{"stage", func(c *Condition) { c.PipeStage = 1 }},
		{"value case", func(c *Condition) { c.Value = "SAME" }},
		{"raw operator", func(c *Condition) { c.Operator = "=" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.change(&changed)
			input := []Condition{base, changed, base}
			if got := DeduplicateConditions(input); !reflect.DeepEqual(got, input[:2]) {
				t.Fatalf("metadata lost or order changed: %+v", got)
			}
		})
	}
	first := base
	first.Field = "NAME"
	duplicate := base
	duplicate.LogicalOp = "OR"
	input := []Condition{first, duplicate}
	before := cloneSemanticConditions(input)
	got := DeduplicateConditions(input)
	if len(got) != 1 || !reflect.DeepEqual(got[0], first) || !reflect.DeepEqual(input, before) {
		t.Fatalf("first occurrence or input changed: %+v / %+v", got, input)
	}
	got[0].Alternatives[0] = "shared"
	if input[0].Alternatives[0] != "shared" {
		t.Fatal("deduplication unexpectedly changed shallow-copy ownership")
	}
	if got := DeduplicateConditions([]Condition{{}, {Alternatives: []string{}}}); len(got) != 1 {
		t.Fatalf("nil and zero-length alternatives should share identity: %+v", got)
	}
	if got := DeduplicateConditions([]Condition{{Operator: "RAW"}, {Operator: "raw"}}); len(got) != 2 {
		t.Fatalf("raw operator case was normalized: %+v", got)
	}
	// A struct key must also keep boundaries between non-list fields separate.
	if got := DeduplicateConditions([]Condition{{Field: "a\x00b", Operator: "c"}, {Field: "a", Operator: "b\x00c"}}); len(got) != 2 {
		t.Fatalf("field/operator boundary collision: %+v", got)
	}
}

func TestBracketProvenanceCloneIsolation(t *testing.T) {
	original := []Condition{{Bracketed: true, Alternatives: []string{"Alpha", "Beta"}}}
	cloned := cloneSemanticConditions(original)
	original[0].Bracketed = false
	original[0].Alternatives[0] = "changed"
	if !cloned[0].Bracketed || cloned[0].Alternatives[0] != "Alpha" {
		t.Fatalf("clone lost origin or shares alternatives: %+v", cloned)
	}
	a := AnalyzeQuery(`name:['Alpha','Beta']`)
	leaf := a.Expression.Condition
	a.Conditions[0].Bracketed = false
	a.Conditions[0].Alternatives[0] = "flat"
	if !leaf.Bracketed || leaf.Alternatives[0] != "Alpha" {
		t.Fatalf("flat metadata aliases semantic leaf: %+v", leaf)
	}
	leaf.Bracketed = false
	leaf.Alternatives[1] = "tree"
	if a.Conditions[0].Alternatives[1] != "Beta" {
		t.Fatal("semantic leaf aliases flat alternatives")
	}
}

func TestBracketProvenancePreservesQueryMetadata(t *testing.T) {
	query := `needle + name:['Alpha'] | timerange(24h)`
	r := ExtractConditions(query)
	a := AnalyzeQuery(query)
	wantPipes := []PipeInfo{{Name: "timerange", Args: "24h", Stage: 1}}
	if len(r.Errors) != 0 || len(a.Errors) != 0 || !reflect.DeepEqual(r.Pipes, wantPipes) || !reflect.DeepEqual(a.Pipes, wantPipes) {
		t.Fatalf("pipe metadata changed: %+v / %+v", r, a)
	}
	if !slices.Equal(r.Searches, []string{"needle"}) || !slices.Equal(a.Searches, r.Searches) || !slices.Equal(r.Fields, []string{"name"}) || !slices.Equal(a.Fields, r.Fields) || !slices.Equal(r.Commands, []string{"timerange"}) || !slices.Equal(a.Commands, r.Commands) {
		t.Fatalf("query metadata changed: %+v / %+v", r, a)
	}
	if len(r.Conditions) != 1 || !r.Conditions[0].Bracketed || r.Conditions[0].PipeStage != 0 || r.Conditions[0].LogicalOp != "AND" || !reflect.DeepEqual(a.Conditions, r.Conditions) {
		t.Fatalf("filter stage or connector changed: %+v / %+v", r.Conditions, a.Conditions)
	}
}
