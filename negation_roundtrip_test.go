package fql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Nested negations must retain their parity and canonical spelling after parsing.
func TestNestedNegationCanonicalRoundTrip(t *testing.T) {
	for _, leaf := range []struct {
		name     string
		query    string
		negated  []bool
		searches []string
	}{
		{"condition", "FileName:'a'", []bool{false}, nil},
		{"field_negation", "!FileName:'a'", []bool{true}, nil},
		{"and_group", "(FileName:'a' AND !ParentFileName:'b')", []bool{false, true}, nil},
		{"or_group", "(FileName:'a' OR ParentFileName:'b')", []bool{false, false}, nil},
		{"quoted_search", "'search term'", nil, []string{"search term"}},
		{"bare_search", "needle", nil, []string{"needle"}},
	} {
		for _, count := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("%s/%d", leaf.name, count), func(t *testing.T) {
				query := strings.Repeat("NOT ", count) + leaf.query
				canonical := assertNegationRoundTrip(t, query)
				for _, input := range []string{query, canonical} {
					result := ExtractConditions(input)
					if len(result.Conditions) != len(leaf.negated) || !reflect.DeepEqual(result.Searches, leaf.searches) {
						t.Fatalf("extraction changed for %q: %+v", input, result)
					}
					for i, condition := range result.Conditions {
						want := leaf.negated[i] != (count%2 == 1)
						if condition.Negated != want {
							t.Fatalf("%q condition %d negation = %v, want %v", input, i, condition.Negated, want)
						}
					}
				}
			})
		}
	}
}

func assertNegationRoundTrip(t *testing.T, input string) string {
	t.Helper()
	parsed, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse(%q): %v", input, err)
	}
	canonical := parsed.String()
	reparsed, err := Parse(canonical)
	if err != nil {
		t.Fatalf("Parse(canonical %q): %v", canonical, err)
	}
	if got := reparsed.String(); got != canonical {
		t.Fatalf("canonical rendering changed: %q -> %q", canonical, got)
	}
	for _, query := range []string{input, canonical} {
		expression, err := ParseExpression(query)
		if err != nil || ExprString(expression) != canonical {
			t.Fatalf("ParseExpression(%q): %v, canonical %q", query, err, ExprString(expression))
		}
		if result := ExtractConditions(query); len(result.Errors) != 0 {
			t.Fatalf("ExtractConditions(%q): %v", query, result.Errors)
		}
		if result := AnalyzeQuery(query); len(result.Errors) != 0 {
			t.Fatalf("AnalyzeQuery(%q): %v", query, result.Errors)
		}
	}
	return canonical
}

func TestNestedNegationRoundTripAtDepthBoundary(t *testing.T) {
	for _, leaf := range []string{"FileName:'a'", "!FileName:'a'", "'search term'", "(FileName:'a' OR ParentFileName:'b')"} {
		for _, depth := range []int{MaxDepth - 1, MaxDepth} {
			count := depth
			if strings.HasPrefix(leaf, "(") {
				count-- // The group and unary operators share one depth budget.
			}
			t.Run(fmt.Sprintf("%s/%d", leaf, depth), func(t *testing.T) {
				query := strings.Repeat("NOT ", count) + leaf
				canonical := assertNegationRoundTrip(t, query)
				for _, input := range []string{query, canonical} {
					for _, condition := range ExtractConditions(input).Conditions {
						want := (count%2 == 1) != strings.HasPrefix(leaf, "!")
						if condition.Negated != want {
							t.Fatalf("negation at depth %d = %v, want %v", depth, condition.Negated, want)
						}
					}
				}
			})
		}
		count := MaxDepth + 1
		if strings.HasPrefix(leaf, "(") {
			count--
		}
		query := strings.Repeat("NOT ", count) + leaf
		if _, err := Parse(query); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
			t.Fatalf("Parse must reject over-depth %q: %v", query, err)
		}
		if _, err := ParseExpression(query); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
			t.Fatalf("ParseExpression must reject over-depth %q: %v", query, err)
		}
		if result := ExtractConditions(query); !strings.Contains(strings.Join(result.Errors, ";"), "nesting too deep") {
			t.Fatalf("ExtractConditions omitted depth error: %+v", result)
		}
		if result := AnalyzeQuery(query); !strings.Contains(strings.Join(result.Errors, ";"), "nesting too deep") {
			t.Fatalf("AnalyzeQuery omitted depth error: %+v", result)
		}
	}
}
