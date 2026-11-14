package fql

import (
	"reflect"
	"testing"
)

func TestNegativeSameFieldORPreservesConditions(t *testing.T) {
	for _, tc := range []struct {
		input, op string
		negated   bool
	}{
		{`(!f:'a',!f:'b')`, ":", true},
		{`(f!:'a',f!:'b')`, "!:", false},
		{`(f!='a',f!='b')`, "!=", false},
		{`(f!~'a',f!~'b')`, "!~", false},
		{`(f:!'a',f:!'b')`, ":!", false},
		{`(f:!~'a',f:!~'b')`, ":!~", false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			want := []Condition{
				{Field: "f", Operator: tc.op, Value: "a", Negated: tc.negated, CaseInsensitive: tc.op == "!~" || tc.op == ":!~"},
				{Field: "f", Operator: tc.op, Value: "b", Negated: tc.negated, CaseInsensitive: tc.op == "!~" || tc.op == ":!~", LogicalOp: "OR"},
			}
			r := ExtractConditions(tc.input)
			a := AnalyzeQuery(tc.input)
			if len(r.Errors) != 0 || len(a.Errors) != 0 || !reflect.DeepEqual(r.Conditions, want) || !reflect.DeepEqual(a.Conditions, want) {
				t.Fatalf("negative OR collapsed: %#v / %#v", r, a)
			}
		})
	}
}

func TestPositiveSameFieldORStillMergesUnderOuterNOT(t *testing.T) {
	for _, tc := range []struct {
		input   string
		negated bool
	}{
		{`(f:'a',f:'b')`, false}, {`!(f:'a',f:'b')`, true},
	} {
		want := []Condition{{Field: "f", Operator: ":", Value: "a", Negated: tc.negated, Alternatives: []string{"a", "b"}}}
		if got := ExtractConditions(tc.input); len(got.Errors) != 0 || !reflect.DeepEqual(got.Conditions, want) {
			t.Fatalf("%s: %#v", tc.input, got)
		}
	}
}

// This intentionally covers only equality, negative equality and OR unions,
// not arbitrary reconstruction of grouped Boolean expressions from flat data.
func TestNegativeORTruthTableDiffersFromNegatedPositiveUnion(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []bool
	}{
		{`(!f:'a',!f:'b')`, []bool{true, true, true}},
		{`(f:!'a',f:!'b')`, []bool{true, true, true}},
		{`!(f:'a',f:'b')`, []bool{false, false, true}},
	} {
		r := ExtractConditions(tc.input)
		if len(r.Errors) != 0 {
			t.Fatal(r.Errors)
		}
		for i, value := range []string{"a", "b", "c"} {
			got := false
			for _, c := range r.Conditions {
				match := value == c.Value
				if c.Alternatives != nil {
					match = false
					for _, alt := range c.Alternatives {
						match = match || value == alt
					}
				}
				if c.Operator == ":!" {
					match = !match
				}
				if c.Negated {
					match = !match
				}
				got = got || match
			}
			if got != tc.want[i] {
				t.Errorf("%s at f=%s: %v, want %v", tc.input, value, got, tc.want[i])
			}
		}
	}
}
