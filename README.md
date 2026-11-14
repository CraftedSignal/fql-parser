# fql-parser

A dependency-free Go lexer/parser for CrowdStrike Falcon Query Language (FQL) —
the `field:[operator]value` filter syntax used by Falcon Event Search and the
Falcon platform APIs.

Part of the CraftedSignal query-parser family (`kql-parser`, `leql-parser`,
`spl-parser`, `sigma-parser`, `eql-parser`) and shares its API shape:
`ExtractConditions` for structured condition extraction, `Parse` for a
round-tripping AST, `NormalizeQuery` for cleaning pasted text.

## Dialect scope

This library targets **classic FQL**: `field:'value'` conditions, `+` (AND) and
`,` (OR) connectives, `!` negation, `[...]` value lists, `~` text matches,
comparison operators, parenthesized groups, `| command(args)` pipes, plus
Event Search conveniences (space-adjacent implicit AND, `=` as an equality
spelling, `AND`/`OR`/`NOT` keywords).

LogScale / CrowdStrike Query Language (CQL) — the pipe-based Humio language —
is a different, much larger language and is **out of scope**.

Classic property/operator syntax follows the [official Falcon FQL guide](https://developer.crowdstrike.com/api-reference/falcon-query-language/):
`platform_name:!'Linux'`, `hostname:~'Server'`, `posts.count:>=10`, and
`hostname:*'prod*'`. The last form keeps the wildcard hint in Operator (`:*`)
and the quoted pattern in Value. Bare `hostname:*` remains a value comparison.
API endpoint restrictions can differ, especially for list combinations.

## Usage

```go
package main

import (
	"fmt"

	fql "github.com/craftedsignal/fql-parser"
)

func main() {
	result := fql.ExtractConditions(
		`event_simpleName:['ProcessRollup2'] + ImageFileName:'*\powershell.exe' + (CommandLine:'*-enc*', CommandLine:'*-encodedcommand*')`,
	)

	for _, c := range result.Conditions {
		fmt.Printf("%s %s %q negated=%v alternatives=%v\n",
			c.Field, c.Operator, c.Value, c.Negated, c.Alternatives)
	}
	// event_simpleName : "ProcessRollup2" negated=false alternatives=[ProcessRollup2]
	// ImageFileName : "*\\powershell.exe" negated=false alternatives=[]
	// CommandLine : "*-enc*" negated=false alternatives=[*-enc* *-encodedcommand*]

	fmt.Println(fql.GetEventTypeFromConditions(result)) // crowdstrike_processrollup2
}
```

AST access:

```go
q, err := fql.Parse(`(RemotePort:445, RemotePort:3389) | timerange(24h)`)
if err != nil {
	// err is non-nil for malformed input, but q still holds the partial AST.
}
fmt.Println(q.String()) // canonical re-rendering (round-trips)
```

## Semantics

- Operators are reported **raw** (`:`, `=`, `~`, `!:`, `!~`, `!=`, `>`, `>=`,
  `<`, `<=`, classic `:!`, `:~`, `:!~`, `:>`, `:>=`, `:<`, `:<=`, and `:*`); only `!field` prefixes and `NOT` fold into `Condition.Negated`.
  Consumers decide semantic normalization.
- Flat conditions are a best-effort view of grouped Boolean expressions. Use
  `AnalyzeQuery.Expression` when exact grouping and negation matter; for example,
  `!(a:1+b:2)` cannot be reconstructed from the flat condition connectors.
  Bracketed exact matches retain their AST list shape, but extracted conditions
  and semantic leaves currently share the `Alternatives` representation with
  merged ORs; this does not preserve their distinct matching semantics.
- Same-field OR groups (`(f:'a', f:'b')`) and `[...]` lists surface as one
  condition with `Alternatives`.
- `ExtractConditions` never panics and never hangs: input size and nesting
  depth are bounded, extraction is wrapped in a recover guard, and parse
  problems are reported in `ParseResult.Errors` while extraction continues
  best-effort.

## License

AGPL-3.0 — see [LICENSE](LICENSE).
