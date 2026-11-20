package fql

import (
	"reflect"
	"testing"
)

// The wildcard hint belongs right after the property colon (property:*'v*').
// Directly before a quote anywhere else it is reported, and the pattern stays
// the condition's value instead of becoming a match-anything star plus a
// free-text search. A star separated from the next term is left alone.
func TestMisplacedWildcardHintIsReported(t *testing.T) {
	for _, tc := range []struct {
		query, op string
		values    []string
		searches  []string
		wantError bool
	}{
		{`a:!*'x*'`, ":!", []string{"x*"}, nil, true},
		{`a:~*'x*'`, ":~", []string{"x*"}, nil, true},
		{`a:!~*'x*'`, ":!~", []string{"x*"}, nil, true},
		{`a!=*'x*'`, "!=", []string{"x*"}, nil, true},
		{`a=*'x*'`, "=", []string{"x*"}, nil, true},
		{`a:[*'x*','y']`, ":", []string{"x*", "y"}, nil, true},
		{`a:['y',*'x*']`, ":", []string{"y", "x*"}, nil, true},
		{`a:*'x*'`, ":*", []string{"x*"}, nil, false},
		{`a:!*`, ":!", []string{"*"}, nil, false},
		{`a:[*,'x']`, ":", []string{"*", "x"}, nil, false},
		{`a!=* 'x'`, "!=", []string{"*"}, []string{"x"}, false},
	} {
		r := ExtractConditions(tc.query)
		_, err := Parse(tc.query)
		if (len(r.Errors) != 0) != tc.wantError || (err != nil) != tc.wantError {
			t.Errorf("%s: errors %q / %v, want an error: %v", tc.query, r.Errors, err, tc.wantError)
		}
		if len(r.Conditions) != 1 || !reflect.DeepEqual(r.Searches, tc.searches) {
			t.Errorf("%s: conditions %+v, searches %q, want searches %q", tc.query, legacyConditions(r.Conditions), r.Searches, tc.searches)
			continue
		}
		c := r.Conditions[0]
		values := []string{c.Value}
		if c.Bracketed {
			values = c.Alternatives
		}
		if c.Operator != tc.op || !reflect.DeepEqual(values, tc.values) {
			t.Errorf("%s: condition %s %q, want %s %q", tc.query, c.Operator, values, tc.op, tc.values)
		}
	}
}
