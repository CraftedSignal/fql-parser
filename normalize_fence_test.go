package fql

import "testing"

// A BOM or zero-width character in front of a markdown fence must not hide the
// fence, and normalising the result again must not change it.
func TestNormalizeQueryStripsFenceAfterInvisiblePrefix(t *testing.T) {
	for _, prefix := range []string{"\ufeff", "\u200b", "\u200c", "\u200d", "\ufeff\u200b "} {
		query := prefix + "```fql\nCommandLine:'x'\n```"
		once := NormalizeQuery(query)
		if once != "CommandLine:'x'" {
			t.Errorf("NormalizeQuery(%q) = %q, want the fence stripped", query, once)
		}
		if twice := NormalizeQuery(once); twice != once {
			t.Errorf("NormalizeQuery(%q) not idempotent: %q, then %q", query, once, twice)
		}
	}
}

func TestNormalizeQueryDoesNotSliceOverlappingFence(t *testing.T) {
	query := "````0"
	if got := NormalizeQuery(query); got != query {
		t.Fatalf("NormalizeQuery(%q) = %q, want unchanged text", query, got)
	}
}
