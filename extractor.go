package fql

import "strings"

// Condition is a single extracted field comparison.
type Condition struct {
	Field           string   `json:"field"`
	Operator        string   `json:"operator"` // ":", "=", "~", "!:", "!~", "!=", ">", ">=", "<", "<="
	Value           string   `json:"value"`
	Negated         bool     `json:"negated"`
	CaseInsensitive bool     `json:"case_insensitive,omitempty"` // ~ text-match operators
	LogicalOp       string   `json:"logical_op"`                 // "AND" or "OR" connecting to the previous condition
	Alternatives    []string `json:"alternatives,omitempty"`     // list values and merged same-field ORs
	PipeStage       int      `json:"pipe_stage"`                 // 0 = main filter expression
}

// PipeInfo describes one piped command.
type PipeInfo struct {
	Name  string `json:"name"`
	Args  string `json:"args,omitempty"`
	Stage int    `json:"stage"` // 1-based pipe position
}

// ParseResult is the full extraction output for a Falcon query.
type ParseResult struct {
	Conditions []Condition `json:"conditions"`
	Searches   []string    `json:"searches,omitempty"` // bare free-text search terms
	Pipes      []PipeInfo  `json:"pipes,omitempty"`
	Commands   []string    `json:"commands,omitempty"` // pipe names in order
	Fields     []string    `json:"fields,omitempty"`   // referenced fields, source order, deduped
	Errors     []string    `json:"errors,omitempty"`
}

// ExtractConditions parses a Falcon FQL query and extracts its conditions,
// free-text searches, and pipe commands. It never panics: malformed input
// yields a best-effort result with Errors populated.
func ExtractConditions(query string) (result *ParseResult) {
	result = &ParseResult{Conditions: []Condition{}}
	defer func() {
		if r := recover(); r != nil {
			result.Errors = append(result.Errors, "internal extraction failure")
		}
	}()

	p := newParser(query)
	q := p.parseQuery()
	result.Errors = append(result.Errors, p.errors...)

	if q.Expr != nil {
		ex := &extractor{result: result}
		ex.walk(q.Expr, false, "")
	}
	for i, pipe := range q.Pipes {
		result.Pipes = append(result.Pipes, PipeInfo{Name: pipe.Name, Args: pipe.Args, Stage: i + 1})
		result.Commands = append(result.Commands, pipe.Name)
	}

	seen := make(map[string]bool)
	for _, c := range result.Conditions {
		key := strings.ToLower(c.Field)
		if c.Field != "" && !seen[key] {
			seen[key] = true
			result.Fields = append(result.Fields, c.Field)
		}
	}
	return result
}

type extractor struct {
	result *ParseResult
}

// walk emits conditions from the expression tree. negated tracks enclosing
// NOT context; connector is the logical operator joining this subtree to the
// previous sibling ("", "AND", or "OR").
func (ex *extractor) walk(e Expr, negated bool, connector string) {
	switch n := e.(type) {
	case *OrExpr:
		if merged, ok := ex.mergeSameFieldOr(n, negated, connector); ok {
			ex.result.Conditions = append(ex.result.Conditions, merged)
			return
		}
		for i, t := range n.Terms {
			conn := connector
			if i > 0 {
				conn = "OR"
			}
			ex.walk(t, negated, conn)
		}
	case *AndExpr:
		for i, t := range n.Terms {
			conn := connector
			if i > 0 {
				conn = "AND"
			}
			ex.walk(t, negated, conn)
		}
	case *NotExpr:
		ex.walk(n.X, !negated, connector)
	case *GroupExpr:
		ex.walk(n.X, negated, connector)
	case *ConditionExpr:
		ex.result.Conditions = append(ex.result.Conditions, ex.condition(n, negated, connector))
	case *SearchExpr:
		if strings.TrimSpace(n.Term) != "" {
			ex.result.Searches = append(ex.result.Searches, n.Term)
		}
	}
}

func (ex *extractor) condition(c *ConditionExpr, negated bool, connector string) Condition {
	cond := Condition{
		Field:           c.Field,
		Operator:        c.Operator,
		Negated:         negated != c.Negated,
		CaseInsensitive: c.Operator == "~" || c.Operator == "!~",
		LogicalOp:       connector,
	}
	if c.Value.IsList() {
		alts := make([]string, 0, len(c.Value.List))
		for _, v := range c.Value.List {
			alts = append(alts, v.Scalar)
		}
		cond.Alternatives = alts
		if len(alts) > 0 {
			cond.Value = alts[0]
		}
	} else {
		cond.Value = c.Value.Scalar
	}
	return cond
}

// mergeSameFieldOr collapses an OR group whose terms are all conditions on
// the same field with the same operator and polarity into one condition with
// Alternatives — e.g. (CommandLine:'*a*', CommandLine:'*b*').
func (ex *extractor) mergeSameFieldOr(or *OrExpr, negated bool, connector string) (Condition, bool) {
	if len(or.Terms) < 2 {
		return Condition{}, false
	}
	var conds []*ConditionExpr
	for _, t := range or.Terms {
		c, ok := t.(*ConditionExpr)
		if !ok || c.Value.IsList() {
			return Condition{}, false
		}
		conds = append(conds, c)
	}
	first := conds[0]
	for _, c := range conds[1:] {
		if !strings.EqualFold(c.Field, first.Field) || c.Operator != first.Operator || c.Negated != first.Negated {
			return Condition{}, false
		}
	}
	merged := ex.condition(first, negated, connector)
	alts := make([]string, 0, len(conds))
	for _, c := range conds {
		alts = append(alts, c.Value.Scalar)
	}
	merged.Alternatives = alts
	return merged, true
}

// DeduplicateConditions removes exact duplicates while preserving order.
func DeduplicateConditions(conditions []Condition) []Condition {
	seen := make(map[string]bool, len(conditions))
	out := make([]Condition, 0, len(conditions))
	for _, c := range conditions {
		key := strings.ToLower(c.Field) + "\x00" + c.Operator + "\x00" + c.Value + "\x00" +
			strings.ToLower(strings.Join(c.Alternatives, "\x01"))
		if c.Negated {
			key += "\x00!"
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, c)
		}
	}
	return out
}

// GetEventTypeFromConditions returns the canonical event type for a query
// that filters on event_simpleName (e.g. "crowdstrike_processrollup2"), or ""
// when no event type filter is present.
func GetEventTypeFromConditions(result *ParseResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Conditions {
		field := strings.ToLower(strings.TrimPrefix(c.Field, "#"))
		if field != "event_simplename" || c.Negated {
			continue
		}
		value := c.Value
		if len(c.Alternatives) > 0 {
			value = c.Alternatives[0]
		}
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" || strings.ContainsAny(value, "*?") {
			continue
		}
		return "crowdstrike_" + value
	}
	return ""
}
