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
  Bracketed RHS values retain their AST list shape. Extracted conditions and
  semantic leaves set `Condition.Bracketed` directly from source brackets,
  including singleton and empty lists. Scalars and merged scalar ORs leave it
  false; quoted text containing brackets does not set it.
- Positive same-field OR groups (`(f:'a', f:'b')`) and `[...]` lists surface as
  one condition with `Alternatives`. Outer NOT may negate a positive OR union;
  individually negated leaves and negative operators keep separate OR conditions.
- `Bracketed` records syntax independently of the raw operator, `Negated`, and
  `CaseInsensitive`; it does not define universal list evaluation. The
  [Falcon FQL guide](https://developer.crowdstrike.com/api-reference/falcon-query-language/)
  documents singleton string brackets as case-sensitive exact matching, while
  supported filters and syntax vary by API collection. Other accepted bracket
  combinations, including empty lists, are preserved without assigning new
  endpoint semantics.
- `DeduplicateConditions` is an opt-in predicate identity helper. It preserves
  literal bytes and ordered alternatives, matching flags, bracket provenance,
  and pipe stage; field names retain case-insensitive identity. It ignores
  `LogicalOp` and keeps the first occurrence unchanged, so it is not a general
  Boolean expression simplifier. Returned conditions are shallow copies and
  share `Alternatives` slices with their first input occurrences.
- `ExtractConditions` never panics and never hangs: input size and nesting
  depth are bounded, extraction is wrapped in a recover guard, and parse
  problems are reported in `ParseResult.Errors` while extraction continues
  best-effort.

## Adopting bracket provenance

Both bracketed RHS values and merged scalar ORs still populate `Value` and
`Alternatives` as before. Consumers must explicitly read `Bracketed` to preserve
that distinction and the documented singleton string exact-match intent. Targets
that cannot preserve bracket intent should report the limitation; endpoint
restrictions and unsupported combinations need explicit handling. Consumers
reading only `Value` and `Alternatives` do not gain these matching semantics
automatically.

Bracketed condition JSON gains `"bracketed": true`, even when empty alternatives
are omitted. Non-bracketed condition JSON is unchanged because false is omitted.
For legacy or manually built conditions, false only means no provenance was
supplied; do not infer brackets from `Alternatives`. External unkeyed Go
`Condition` struct literals may need updating; prefer keyed literals.

This parser metadata change requires separate consumer adoption. It does not
change endpoint evaluation or update downstream dependencies.

## License

AGPL-3.0 — see [LICENSE](LICENSE).
