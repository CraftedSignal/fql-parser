package fql

import (
	"reflect"
	"testing"
)

// Pipe arguments are the raw text inside the parentheses, or after a bare
// command name, so quotes and escapes survive and String() round-trips.
func TestPipeArgsKeepRawText(t *testing.T) {
	for _, tc := range []struct {
		query string
		pipes []Pipe
	}{
		{`a:1 | f('x)y')`, []Pipe{{Name: "f", Args: `'x)y'`}}},
		{`a:1 | f('p+q')`, []Pipe{{Name: "f", Args: `'p+q'`}}},
		{`a:1 | f('a b', "c")`, []Pipe{{Name: "f", Args: `'a b', "c"`}}},
		{`a:1 | cmd(foo(bar), 'it\'s')`, []Pipe{{Name: "cmd", Args: `foo(bar), 'it\'s'`}}},
		{`a:1 | timerange( 24h )`, []Pipe{{Name: "timerange", Args: "24h"}}},
		{`a:1 | table 'a b'  c | head`, []Pipe{{Name: "table", Args: `'a b'  c`}, {Name: "head"}}},
	} {
		q, err := Parse(tc.query)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.query, err)
		}
		if !reflect.DeepEqual(q.Pipes, tc.pipes) {
			t.Errorf("Parse(%q) pipes %+v, want %+v", tc.query, q.Pipes, tc.pipes)
		}
		rendered := q.String()
		round, err := Parse(rendered)
		if err != nil {
			t.Errorf("String() of %q is %q, which does not parse: %v", tc.query, rendered, err)
			continue
		}
		if !reflect.DeepEqual(round.Pipes, q.Pipes) || round.String() != rendered {
			t.Errorf("round trip of %q through %q changed the pipes to %+v, rendered %q", tc.query, rendered, round.Pipes, round.String())
		}
		var want []PipeInfo
		for i, p := range tc.pipes {
			want = append(want, PipeInfo{Name: p.Name, Args: p.Args, Stage: i + 1})
		}
		if got := ExtractConditions(tc.query).Pipes; !reflect.DeepEqual(got, want) {
			t.Errorf("ExtractConditions(%q) pipes %+v, want %+v", tc.query, got, want)
		}
	}
}

// An unterminated string in bare pipe arguments is reported, as it is inside
// parentheses: kept raw, String() could not write it back.
func TestBarePipeArgsUnterminatedStringIsReported(t *testing.T) {
	for _, query := range []string{`a:1 | f 'abc`, `a:1 | f x "abc`} {
		if _, err := Parse(query); err == nil {
			t.Errorf("Parse(%q) accepted an unterminated string in pipe arguments", query)
		}
		if result := ExtractConditions(query); len(result.Errors) == 0 {
			t.Errorf("ExtractConditions(%q) reported no error", query)
		}
	}
}

func TestBarePipeArgsWithUnbalancedParenthesisRoundTrip(t *testing.T) {
	query := `|0!(`
	parsed, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	rendered := parsed.String()
	if rendered != ` | 0 !(` {
		t.Fatalf("String() = %q, want bare pipe args preserved", rendered)
	}
	roundTrip, err := Parse(rendered)
	if err != nil {
		t.Fatalf("String() rendered %q, which does not parse: %v", rendered, err)
	}
	if !reflect.DeepEqual(roundTrip.Pipes, parsed.Pipes) || roundTrip.String() != rendered {
		t.Fatalf("round trip of %q changed pipes from %+v to %+v", rendered, parsed.Pipes, roundTrip.Pipes)
	}
}
