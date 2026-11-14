package fql

import (
	"reflect"
	"testing"
)

// These fixtures protect colon/operator consumption and quoted/list boundaries:
// dropping the optional operator must fail before extraction can leak searches.
func TestClassicAPISyntax(t *testing.T) {
	tests := []struct {
		input, field, op string
		value            Value
		insensitive      bool
	}{
		{`platform_name:!'Linux'`, "platform_name", ":!", Value{Scalar: "Linux", Quoted: true}, false},
		{`hostname:~'Server'`, "hostname", ":~", Value{Scalar: "Server", Quoted: true}, true},
		{`hostname:!~'Server'`, "hostname", ":!~", Value{Scalar: "Server", Quoted: true}, true},
		{`posts.count:>10`, "posts.count", ":>", Value{Scalar: "10"}, false},
		{`posts.count:>=10`, "posts.count", ":>=", Value{Scalar: "10"}, false},
		{`posts.count:<10`, "posts.count", ":<", Value{Scalar: "10"}, false},
		{`last_seen:<='2021-08-31T12:00:00Z'`, "last_seen", ":<=", Value{Scalar: "2021-08-31T12:00:00Z", Quoted: true}, false},
		{`featured:!true`, "featured", ":!", Value{Scalar: "true"}, false},
		{`name:!null`, "name", ":!", Value{Scalar: "null"}, false},
		{`name:!['a']`, "name", ":!", Value{List: []Value{{Scalar: "a", Quoted: true}}}, false},
		{`name:!['a','b']`, "name", ":!", Value{List: []Value{{Scalar: "a", Quoted: true}, {Scalar: "b", Quoted: true}}}, false},
		{`hostname:*'prod*'`, "hostname", ":*", Value{Scalar: "prod*", Quoted: true}, false},
		{`hostname:'prod*'`, "hostname", ":", Value{Scalar: "prod*", Quoted: true}, false},
		{`hostname:*`, "hostname", ":", Value{Scalar: "*"}, false},
		{`hostname:prod*`, "hostname", ":", Value{Scalar: "prod*"}, false},
		{`field!:value`, "field", "!:", Value{Scalar: "value"}, false},
		{`field~value`, "field", "~", Value{Scalar: "value"}, true},
		{`field!~value`, "field", "!~", Value{Scalar: "value"}, true},
		{`field>value`, "field", ">", Value{Scalar: "value"}, false},
		{`field=value`, "field", "=", Value{Scalar: "value"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q, err := Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			c, ok := q.Expr.(*ConditionExpr)
			if !ok {
				t.Fatalf("want condition, got %#v", q.Expr)
			}
			if c.Field != tt.field || c.Operator != tt.op || !reflect.DeepEqual(c.Value, tt.value) || c.Negated {
				t.Fatalf("unexpected AST: %#v", c)
			}
			expr, err := ParseExpression(tt.input)
			if err != nil || !reflect.DeepEqual(expr, q.Expr) {
				t.Fatalf("ParseExpression: %#v, %v", expr, err)
			}
			round, err := Parse(q.String())
			if err != nil || !reflect.DeepEqual(q, round) || round.String() != q.String() {
				t.Fatalf("unstable round trip: %s, %v", q.String(), err)
			}
			var alts []string
			value := tt.value.Scalar
			if tt.value.IsList() {
				for _, v := range tt.value.List {
					alts = append(alts, v.Scalar)
				}
				value = alts[0]
			}
			want := Condition{Field: tt.field, Operator: tt.op, Value: value, Alternatives: alts, CaseInsensitive: tt.insensitive, Bracketed: tt.value.IsList()}
			extracted := ExtractConditions(tt.input)
			if len(extracted.Errors) != 0 || len(extracted.Searches) != 0 || !reflect.DeepEqual(extracted.Conditions, []Condition{want}) {
				t.Fatalf("ExtractConditions: %#v", extracted)
			}
			analyzed := AnalyzeQuery(tt.input)
			if len(analyzed.Errors) != 0 || len(analyzed.Searches) != 0 || !reflect.DeepEqual(analyzed.Conditions, []Condition{want}) || analyzed.Expression.Operator != "condition" || !reflect.DeepEqual(analyzed.Expression.Condition, &want) {
				t.Fatalf("AnalyzeQuery: %#v / %#v", analyzed, analyzed.Expression)
			}
			if !reflect.DeepEqual(analyzed, AnalyzeQuery(q.String())) {
				t.Fatal("semantic round trip changed")
			}
		})
	}
}

func TestClassicOperatorUnaryNegation(t *testing.T) {
	for _, input := range []string{`!field:value`, `!field:!'a'`, `NOT field:!'a'`, `NOT !field:!'a'`} {
		t.Run(input, func(t *testing.T) {
			q, err := Parse(input)
			if err != nil {
				t.Fatal(err)
			}
			round, err := Parse(q.String())
			if err != nil || round.String() != q.String() {
				t.Fatalf("round trip: %v", err)
			}
			expr, err := ParseExpression(input)
			if err != nil || !reflect.DeepEqual(expr, q.Expr) {
				t.Fatalf("expression: %v", err)
			}
			op := ":!"
			if input == `!field:value` {
				op = ":"
			}
			negated := input != `NOT !field:!'a'`
			r := ExtractConditions(input)
			a := AnalyzeQuery(input)
			if len(r.Errors) != 0 || len(r.Searches) != 0 || len(r.Conditions) != 1 || r.Conditions[0].Operator != op || r.Conditions[0].Negated != negated || !reflect.DeepEqual(r.Conditions, a.Conditions) || len(a.Errors) != 0 || len(a.Searches) != 0 {
				t.Fatalf("negation lost: %#v / %#v", r, a)
			}
			if !reflect.DeepEqual(a.Conditions, AnalyzeQuery(q.String()).Conditions) {
				t.Fatal("semantic negation round trip changed")
			}
		})
	}
}

func TestClassicOperatorsMissingOperand(t *testing.T) {
	for _, input := range []string{`f:!`, `f:~`, `f:!~`, `f:>`, `f:>=`, `f:<`, `f:<=`, `f:!+g:a`, `f:!['a'`, `f:>~'a'`, `f:='a'`} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Fatal("Parse accepted missing/invalid operand")
			}
			if _, err := ParseExpression(input); err == nil {
				t.Fatal("ParseExpression accepted missing/invalid operand")
			}
			if len(ExtractConditions(input).Errors) == 0 {
				t.Fatal("ExtractConditions omitted errors")
			}
			if len(AnalyzeQuery(input).Errors) == 0 {
				t.Fatal("AnalyzeQuery omitted errors")
			}
		})
	}
}
