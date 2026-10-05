package fql

// SemanticQuery is the parser-owned semantic view of a Falcon query.
type SemanticQuery struct {
	Conditions []Condition         `json:"conditions,omitempty"`
	Expression *SemanticExpression `json:"expression,omitempty"`
	Searches   []string            `json:"searches,omitempty"`
	Pipes      []PipeInfo          `json:"pipes,omitempty"`
	Commands   []string            `json:"commands,omitempty"`
	Fields     []string            `json:"fields,omitempty"`
	Errors     []string            `json:"errors,omitempty"`
}

// SemanticExpression preserves FQL boolean grouping and typed leaf nodes.
type SemanticExpression struct {
	Operator  string                `json:"operator"`
	Condition *Condition            `json:"condition,omitempty"`
	Search    *SemanticSearch       `json:"search,omitempty"`
	Children  []*SemanticExpression `json:"children,omitempty"`
}

// SemanticSearch is a bare FQL free-text search term.
type SemanticSearch struct {
	Term   string `json:"term"`
	Quoted bool   `json:"quoted,omitempty"`
}

// AnalyzeQuery parses an FQL query and returns the parser-owned semantic model.
func AnalyzeQuery(query string) (result *SemanticQuery) {
	result = &SemanticQuery{}
	p := newParser(query)
	parsed := p.parseQuery()
	extracted := ExtractConditions(query)

	result.Conditions = cloneSemanticConditions(extracted.Conditions)
	result.Searches = cloneSemanticStrings(extracted.Searches)
	result.Pipes = cloneSemanticPipes(extracted.Pipes)
	result.Commands = cloneSemanticStrings(extracted.Commands)
	result.Fields = cloneSemanticStrings(extracted.Fields)
	result.Errors = append(result.Errors, p.errors...)
	result.Errors = append(result.Errors, extracted.Errors...)
	result.Errors = deduplicateSemanticStrings(result.Errors)
	result.Expression = semanticExpressionFromExpr(parsed.Expr)
	return result
}

func semanticExpressionFromExpr(expression Expr) *SemanticExpression {
	switch e := expression.(type) {
	case nil:
		return nil
	case *OrExpr:
		return semanticExpressionGroup("or", e.Terms)
	case *AndExpr:
		return semanticExpressionGroup("and", e.Terms)
	case *NotExpr:
		child := semanticExpressionFromExpr(e.X)
		if child == nil {
			return nil
		}
		return &SemanticExpression{Operator: "not", Children: []*SemanticExpression{child}}
	case *GroupExpr:
		child := semanticExpressionFromExpr(e.X)
		if child == nil {
			return nil
		}
		return &SemanticExpression{Operator: "group", Children: []*SemanticExpression{child}}
	case *ConditionExpr:
		condition := semanticConditionFromConditionExpr(e)
		return &SemanticExpression{Operator: "condition", Condition: &condition}
	case *SearchExpr:
		search := &SemanticSearch{Term: e.Term, Quoted: e.Quoted}
		return &SemanticExpression{Operator: "search", Search: search}
	default:
		return nil
	}
}

func semanticExpressionGroup(operator string, expressions []Expr) *SemanticExpression {
	semantic := &SemanticExpression{Operator: operator}
	for _, expression := range expressions {
		if converted := semanticExpressionFromExpr(expression); converted != nil {
			semantic.Children = append(semantic.Children, converted)
		}
	}
	if len(semantic.Children) == 0 {
		return nil
	}
	if len(semantic.Children) == 1 {
		return semantic.Children[0]
	}
	return semantic
}

func semanticConditionFromConditionExpr(expression *ConditionExpr) Condition {
	if expression == nil {
		return Condition{}
	}
	condition := Condition{
		Field:           expression.Field,
		Operator:        expression.Operator,
		Negated:         expression.Negated,
		CaseInsensitive: expression.Operator == "~" || expression.Operator == "!~",
	}
	if expression.Value.IsList() {
		condition.Alternatives = make([]string, 0, len(expression.Value.List))
		for _, value := range expression.Value.List {
			condition.Alternatives = append(condition.Alternatives, value.Scalar)
		}
		if len(condition.Alternatives) > 0 {
			condition.Value = condition.Alternatives[0]
		}
		return condition
	}
	condition.Value = expression.Value.Scalar
	return condition
}

func cloneSemanticConditions(conditions []Condition) []Condition {
	if len(conditions) == 0 {
		return nil
	}
	out := make([]Condition, len(conditions))
	for i, condition := range conditions {
		out[i] = condition
		out[i].Alternatives = cloneSemanticStrings(condition.Alternatives)
	}
	return out
}

func cloneSemanticPipes(pipes []PipeInfo) []PipeInfo {
	if len(pipes) == 0 {
		return nil
	}
	return append([]PipeInfo(nil), pipes...)
}

func cloneSemanticStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func deduplicateSemanticStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
