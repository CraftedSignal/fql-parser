package fql

import "testing"

// Zero-width and no-break characters inside an ASCII-quoted value are part of
// the value, like smart quotes there; outside quotes they are still cleaned.
func TestNormalizeQueryKeepsInvisibleCharactersInQuotedValues(t *testing.T) {
	for _, query := range []string{
		"FileName:'*\u200b*'",
		"FileName:'a\u00a0b'",
		"FileName:'a\ufeffb'",
		"FileName:'a\u200db'",
		"FileName:\"a\u200cb\"",
	} {
		if got := NormalizeQuery(query); got != query {
			t.Errorf("NormalizeQuery(%q) = %q, want it unchanged", query, got)
		}
		want := ExtractConditions(query).Conditions[0].Value
		if got := ExtractConditions(NormalizeQuery(query)).Conditions[0].Value; got != want {
			t.Errorf("NormalizeQuery(%q) changed the value from %q to %q", query, want, got)
		}
	}
	if a, b := NormalizeQuery("FileName:'*\u200b*'"), NormalizeQuery("FileName:'**'"); a == b {
		t.Errorf("two different values normalise to the same query %q", a)
	}
	if got := NormalizeQuery("\ufeffFileName:'a'\u00a0+\u200bb:'c'"); got != "FileName:'a' +b:'c'" {
		t.Errorf("characters outside quoted values not cleaned: %q", got)
	}
	if got := NormalizeQuery("FileName:\u2018a\u00a0b\u2019"); got != "FileName:'a b'" {
		t.Errorf("smart-quoted value not cleaned: %q", got)
	}
}
