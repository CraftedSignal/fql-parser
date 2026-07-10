package fql

import (
	"strings"
	"testing"
)

func TestExtractSimple(t *testing.T) {
	result := ExtractConditions(`CommandLine:'*-enc*' + UserName:'admin'`)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %+v", result.Conditions)
	}
	if result.Conditions[0].Field != "CommandLine" || result.Conditions[0].Operator != ":" {
		t.Fatalf("unexpected first condition: %+v", result.Conditions[0])
	}
	if result.Conditions[1].LogicalOp != "AND" {
		t.Fatalf("expected AND connector, got %q", result.Conditions[1].LogicalOp)
	}
	if len(result.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %v", result.Fields)
	}
}

func TestExtractListAlternatives(t *testing.T) {
	result := ExtractConditions(`event_simpleName:['ProcessRollup2','SyntheticProcessRollup2']`)
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %+v", result.Conditions)
	}
	c := result.Conditions[0]
	if len(c.Alternatives) != 2 || c.Value != "ProcessRollup2" {
		t.Fatalf("unexpected alternatives: %+v", c)
	}
}

func TestExtractMergedSameFieldOr(t *testing.T) {
	result := ExtractConditions(`(CommandLine:'*-enc*', CommandLine:'*-encodedcommand*')`)
	if len(result.Conditions) != 1 {
		t.Fatalf("expected merged condition, got %+v", result.Conditions)
	}
	if len(result.Conditions[0].Alternatives) != 2 {
		t.Fatalf("expected 2 alternatives, got %+v", result.Conditions[0])
	}
}

func TestExtractNegationThroughGroups(t *testing.T) {
	result := ExtractConditions(`!(UserName:'svc-scan')`)
	if len(result.Conditions) != 1 || !result.Conditions[0].Negated {
		t.Fatalf("expected negated condition, got %+v", result.Conditions)
	}

	// Double negation cancels
	result = ExtractConditions(`!(!UserName:'svc-scan')`)
	if len(result.Conditions) != 1 || result.Conditions[0].Negated {
		t.Fatalf("expected non-negated condition, got %+v", result.Conditions)
	}
}

func TestExtractNegatedOperatorsKeptRaw(t *testing.T) {
	// Family convention: the library reports raw operator spellings and only
	// folds !-prefix/NOT into Negated; adapters do semantic normalization.
	result := ExtractConditions(`Status!:'closed' + Verdict!='FalsePositive'`)
	if len(result.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %+v", result.Conditions)
	}
	if result.Conditions[0].Operator != "!:" || result.Conditions[0].Negated {
		t.Fatalf("expected raw !: operator, got %+v", result.Conditions[0])
	}
	if result.Conditions[1].Operator != "!=" {
		t.Fatalf("expected raw != operator, got %+v", result.Conditions[1])
	}
}

func TestExtractTildeCaseInsensitive(t *testing.T) {
	result := ExtractConditions(`CommandLine~'mimikatz'`)
	if len(result.Conditions) != 1 || !result.Conditions[0].CaseInsensitive {
		t.Fatalf("expected case-insensitive ~ condition, got %+v", result.Conditions)
	}
}

func TestExtractPipes(t *testing.T) {
	result := ExtractConditions(`RemotePort:445 | timerange(24h)`)
	if len(result.Pipes) != 1 || result.Pipes[0].Name != "timerange" || result.Pipes[0].Stage != 1 {
		t.Fatalf("unexpected pipes: %+v", result.Pipes)
	}
	if len(result.Commands) != 1 || result.Commands[0] != "timerange" {
		t.Fatalf("unexpected commands: %v", result.Commands)
	}
}

func TestExtractFreeTextSearch(t *testing.T) {
	result := ExtractConditions(`mimikatz + UserName:'admin'`)
	if len(result.Searches) != 1 || result.Searches[0] != "mimikatz" {
		t.Fatalf("expected free-text search, got %+v", result.Searches)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %+v", result.Conditions)
	}
}

func TestExtractEventSearchStyle(t *testing.T) {
	result := ExtractConditions(`event_simpleName=ProcessRollup2 FileName=cmd.exe`)
	if len(result.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %+v", result.Conditions)
	}
	if result.Conditions[1].LogicalOp != "AND" {
		t.Fatalf("adjacency should read as AND, got %q", result.Conditions[1].LogicalOp)
	}
}

func TestGetEventTypeFromConditions(t *testing.T) {
	result := ExtractConditions(`event_simpleName:['ProcessRollup2'] + CommandLine:'*x*'`)
	if got := GetEventTypeFromConditions(result); got != "crowdstrike_processrollup2" {
		t.Fatalf("expected crowdstrike_processrollup2, got %q", got)
	}

	result = ExtractConditions(`#event_simpleName=DnsRequest`)
	if got := GetEventTypeFromConditions(result); got != "crowdstrike_dnsrequest" {
		t.Fatalf("expected crowdstrike_dnsrequest, got %q", got)
	}

	result = ExtractConditions(`CommandLine:'*x*'`)
	if got := GetEventTypeFromConditions(result); got != "" {
		t.Fatalf("expected empty event type, got %q", got)
	}
}

func TestDeduplicateConditions(t *testing.T) {
	result := ExtractConditions(`a:'1' + a:'1' + b:'2'`)
	deduped := DeduplicateConditions(result.Conditions)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 after dedup, got %+v", deduped)
	}
}

func TestExtractNeverPanics(t *testing.T) {
	inputs := []string{
		"", "   ", "!", ":", "'''", `"`, "((((", "]]]]", "a:", ":b", "!!!!a:b",
		"a:[", "a:[,,,]", "| | |", "a:'x' |", strings.Repeat("(a:'1',", 500),
		"\x00\x01\x02", "a:'\\'", "�", "a" + strings.Repeat("+", 1000),
	}
	for _, in := range inputs {
		result := ExtractConditions(in)
		if result == nil {
			t.Fatalf("nil result for %q", in)
		}
	}
}

func TestNormalizeQuery(t *testing.T) {
	in := "```fql\nCommandLine:‘*-enc*’\n```"
	out := NormalizeQuery(in)
	if out != "CommandLine:'*-enc*'" {
		t.Fatalf("unexpected normalization: %q", out)
	}
}

// TestPlatformCorpus covers the exact shapes the CraftedSignal translator and
// hunt dialect emit — the primary consumers of this library.
func TestPlatformCorpus(t *testing.T) {
	corpus := []struct {
		query      string
		conditions int
	}{
		{`CommandLine:'*regsvr32*/s /u /n /i:http*scrobj*'`, 1},
		{`event_platform:['Win'] + event_simpleName:['ProcessRollup2'] + ImageFileName:'*\powershell.exe'`, 3},
		{`!ParentBaseFileName:['sccm.exe','schtasks.exe'] + CommandLine~'.*mimikatz.*'`, 2},
		{`(RemotePort:445, RemotePort:3389) | timerange(24h)`, 1}, // merged same-field OR
		{`event_simpleName:['DnsRequest'] + DomainName:'*.evil.example'`, 2},
	}
	for _, tc := range corpus {
		result := ExtractConditions(tc.query)
		if len(result.Errors) > 0 {
			t.Fatalf("query %q: unexpected errors %v", tc.query, result.Errors)
		}
		if len(result.Conditions) != tc.conditions {
			t.Fatalf("query %q: expected %d conditions, got %+v", tc.query, tc.conditions, result.Conditions)
		}
	}
}
