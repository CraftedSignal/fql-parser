package fql

import "testing"

// Only a positive equality on event_simpleName names the event type. Negative
// operators exclude it, and comparisons and text matches do not pin one type.
func TestEventTypeComesOnlyFromPositiveEquality(t *testing.T) {
	for _, tc := range []struct{ query, want string }{
		{`event_simpleName:'ProcessRollup2'`, "crowdstrike_processrollup2"},
		{`event_simpleName=ProcessRollup2`, "crowdstrike_processrollup2"},
		{`event_simpleName:*'ProcessRollup2'`, "crowdstrike_processrollup2"},
		{`event_simpleName:['ProcessRollup2','DnsRequest']`, "crowdstrike_processrollup2"},
		{`event_simpleName:!'ProcessRollup2'`, ""},
		{`event_simpleName!='ProcessRollup2'`, ""},
		{`event_simpleName!:'ProcessRollup2'`, ""},
		{`event_simpleName:!['ProcessRollup2','DnsRequest']`, ""},
		{`event_simpleName:!~'ProcessRollup2'`, ""},
		{`event_simpleName!~'ProcessRollup2'`, ""},
		{`event_simpleName:~'proc'`, ""},
		{`event_simpleName~'ProcessRollup2'`, ""},
		{`event_simpleName:>'A'`, ""},
		{`event_simpleName>=A`, ""},
		{`event_simpleName:<='Z'`, ""},
		{`event_simpleName:!'ProcessRollup2'+event_simpleName:'DnsRequest'`, "crowdstrike_dnsrequest"},
	} {
		result := ExtractConditions(tc.query)
		if len(result.Errors) != 0 {
			t.Fatalf("%s: unexpected errors %v", tc.query, result.Errors)
		}
		if got := GetEventTypeFromConditions(result); got != tc.want {
			t.Errorf("%s: event type %q, want %q", tc.query, got, tc.want)
		}
	}
}
