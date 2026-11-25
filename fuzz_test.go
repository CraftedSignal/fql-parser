package fql

import "testing"

func FuzzFQLParser(f *testing.F) {
	for _, query := range []string{
		`FileName:'test.exe'`,
		`event_simpleName:['ProcessRollup2'] + ImageFileName:'*\powershell.exe'`,
		`(RemotePort:445, RemotePort:3389) | timerange(24h)`,
		`NOT (a:1 OR b:2)`,
		`field:[1, true, null, 'x']`,
		``,
		`field:'unterminated`,
		`field:'é' + other:😀`,
	} {
		f.Add(query)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > MaxInputSize {
			return
		}

		parsed, err := Parse(input)
		if parsed == nil {
			t.Fatal("Parse returned a nil query")
		}
		if err == nil {
			rendered := parsed.String()
			reparsed, err := Parse(rendered)
			if err != nil {
				t.Fatalf("rendered valid query %q failed to parse: %v", rendered, err)
			}
			if got := reparsed.String(); got != rendered {
				t.Fatalf("canonical rendering is not stable: first %q, then %q", rendered, got)
			}
		}

		_, _ = ParseExpression(input)
		if result := ExtractConditions(input); result == nil {
			t.Fatal("ExtractConditions returned a nil result")
		}
		if result := AnalyzeQuery(input); result == nil {
			t.Fatal("AnalyzeQuery returned a nil result")
		}
		_ = NormalizeQuery(input)
	})
}
