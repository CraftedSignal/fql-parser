package fql

import (
	"strings"
	"testing"
)

func TestUnaryNotNestingIsBounded(t *testing.T) {
	query := strings.Repeat("NOT ", MaxDepth+1) + "FileName:'editor.exe'"
	if _, err := Parse(query); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
		t.Fatalf("expected unary depth error, got %v", err)
	}
	if _, err := ParseExpression(query); err == nil || !strings.Contains(err.Error(), "nesting too deep") {
		t.Fatalf("expected standalone unary depth error, got %v", err)
	}
	if result := ExtractConditions(query); len(result.Errors) == 0 {
		t.Fatal("extraction must report the unary depth guard")
	}
}

func TestUnaryNotAtDepthBoundaryPreservesNegation(t *testing.T) {
	query := strings.Repeat("NOT ", MaxDepth-1) + "FileName:'editor.exe'"
	result := ExtractConditions(query)
	if len(result.Errors) != 0 || len(result.Conditions) != 1 || !result.Conditions[0].Negated {
		t.Fatalf("valid unary chain changed: %#v", result)
	}
}

func TestUnaryAndGroupNestingShareDepthBudget(t *testing.T) {
	query := "NOT " + strings.Repeat("(", MaxDepth) + "FileName:'editor.exe'" + strings.Repeat(")", MaxDepth)
	parsed, err := Parse(query)
	if err == nil || !strings.Contains(err.Error(), "nesting too deep") {
		t.Fatalf("expected combined nesting error, got %v", err)
	}
	// The bounded partial AST remains safe to inspect and render.
	_ = parsed.String()
	if result := AnalyzeQuery(query); len(result.Errors) == 0 {
		t.Fatal("semantic analysis omitted the depth error")
	}
}
