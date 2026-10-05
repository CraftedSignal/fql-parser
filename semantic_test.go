package fql

import (
	"strings"
	"testing"
)

func TestAnalyzeQueryPreservesFQLBooleanTreeAndPipes(t *testing.T) {
	semantic := AnalyzeQuery(`!(CommandLine:'*-enc*' + (event_simpleName=ProcessRollup2, event_simpleName=SyntheticProcessRollup2)) | timerange(24h)`)
	if semantic == nil {
		t.Fatal("expected semantic query")
	}
	if len(semantic.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", semantic.Errors)
	}
	if len(semantic.Pipes) != 1 || semantic.Pipes[0].Name != "timerange" || semantic.Pipes[0].Args != "24h" {
		t.Fatalf("pipe not preserved: %+v", semantic.Pipes)
	}
	if len(semantic.Commands) != 1 || semantic.Commands[0] != "timerange" {
		t.Fatalf("commands not preserved: %+v", semantic.Commands)
	}
	if len(semantic.Fields) != 2 {
		t.Fatalf("fields not deduped/preserved: %+v", semantic.Fields)
	}

	expression := semantic.Expression
	if expression == nil || expression.Operator != "not" || len(expression.Children) != 1 {
		t.Fatalf("expected top-level NOT expression, got %#v", expression)
	}
	group := expression.Children[0]
	if group.Operator != "group" || len(group.Children) != 1 {
		t.Fatalf("expected NOT child to preserve group, got %#v", group)
	}
	and := group.Children[0]
	if and.Operator != "and" || len(and.Children) != 2 {
		t.Fatalf("expected grouped AND expression, got %#v", and)
	}
	if and.Children[0].Condition == nil || and.Children[0].Condition.Field != "CommandLine" {
		t.Fatalf("expected CommandLine condition, got %#v", and.Children[0])
	}
	orGroup := and.Children[1]
	if orGroup.Operator != "group" || len(orGroup.Children) != 1 || orGroup.Children[0].Operator != "or" {
		t.Fatalf("expected grouped OR expression, got %#v", orGroup)
	}

	var eventCondition *Condition
	for i := range semantic.Conditions {
		if strings.EqualFold(semantic.Conditions[i].Field, "event_simpleName") {
			eventCondition = &semantic.Conditions[i]
			break
		}
	}
	if eventCondition == nil || len(eventCondition.Alternatives) != 2 {
		t.Fatalf("same-field OR alternatives not preserved: %+v", semantic.Conditions)
	}
}

func TestAnalyzeQueryPreservesSearchAndListValues(t *testing.T) {
	semantic := AnalyzeQuery(`'mimikatz' + event_simpleName:[ProcessRollup2, SyntheticProcessRollup2]`)
	if len(semantic.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", semantic.Errors)
	}
	if len(semantic.Searches) != 1 || semantic.Searches[0] != "mimikatz" {
		t.Fatalf("searches not preserved: %+v", semantic.Searches)
	}
	expression := semantic.Expression
	if expression == nil || expression.Operator != "and" || len(expression.Children) != 2 {
		t.Fatalf("expected search + condition AND expression, got %#v", expression)
	}
	if expression.Children[0].Search == nil || expression.Children[0].Search.Term != "mimikatz" || !expression.Children[0].Search.Quoted {
		t.Fatalf("quoted search not preserved: %#v", expression.Children[0])
	}
	condition := expression.Children[1].Condition
	if condition == nil || condition.Value != "ProcessRollup2" || len(condition.Alternatives) != 2 {
		t.Fatalf("list-valued condition not preserved: %#v", condition)
	}
}

func TestAnalyzeQuerySearchOnlyHasNoConditions(t *testing.T) {
	semantic := AnalyzeQuery(`mimikatz`)
	if len(semantic.Errors) != 0 {
		t.Fatalf("unexpected errors: %v", semantic.Errors)
	}
	if semantic.Conditions != nil || semantic.Pipes != nil || semantic.Commands != nil || semantic.Fields != nil {
		t.Fatalf("search-only query should not synthesize structured metadata: %+v", semantic)
	}
	if semantic.Expression == nil || semantic.Expression.Search == nil || semantic.Expression.Search.Term != "mimikatz" {
		t.Fatalf("search-only expression not preserved: %#v", semantic.Expression)
	}
}

func TestAnalyzeQueryReportsErrorsWithoutFallbackAPI(t *testing.T) {
	semantic := AnalyzeQuery(`a:'1' | ( bad`)
	if semantic == nil {
		t.Fatal("expected semantic query with errors")
	}
	if len(semantic.Errors) == 0 {
		t.Fatalf("expected parse errors, got %+v", semantic)
	}
}

func TestSemanticExpressionBuilderEdges(t *testing.T) {
	if got := semanticExpressionFromExpr(nil); got != nil {
		t.Fatalf("nil expression converted to %#v", got)
	}
	if got := semanticExpressionFromExpr(unknownExpr{}); got != nil {
		t.Fatalf("unknown expression converted to %#v", got)
	}
	if got := semanticExpressionGroup("and", nil); got != nil {
		t.Fatalf("empty group converted to %#v", got)
	}
	single := semanticExpressionGroup("and", []Expr{&SearchExpr{Term: "one"}})
	if single == nil || single.Operator != "search" || single.Search.Term != "one" {
		t.Fatalf("single-child group should collapse, got %#v", single)
	}
	if got := semanticExpressionFromExpr(&NotExpr{X: unknownExpr{}}); got != nil {
		t.Fatalf("not unknown expression converted to %#v", got)
	}
	if got := semanticExpressionFromExpr(&GroupExpr{X: unknownExpr{}}); got != nil {
		t.Fatalf("group unknown expression converted to %#v", got)
	}
	if got := semanticConditionFromConditionExpr(nil); got.Field != "" || got.Operator != "" || got.Value != "" || got.Alternatives != nil {
		t.Fatalf("nil condition converted to %+v", got)
	}
}

type unknownExpr struct{}

func (unknownExpr) render(*strings.Builder) {}
