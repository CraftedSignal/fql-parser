package fql

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NormalizeQuery cleans pasted or machine-extracted query text so it lexes
// the way the author intended: markdown code fences are stripped, smart
// quotes become ASCII quotes, and zero-width/non-breaking spaces are removed.
// Text inside an ASCII-quoted value is kept as written.
func NormalizeQuery(query string) string {
	// Typography first: a leading BOM or zero-width space would hide the fence.
	q := strings.TrimSpace(normalizeTypography(query))

	// Strip markdown code fences: ```fql\n...\n``` or ```\n...\n```
	if strings.HasPrefix(q, "```") {
		if end := strings.LastIndex(q, "```"); end >= 3 {
			body := q[3:end]
			if nl := strings.IndexAny(body, "\r\n"); nl >= 0 {
				// Drop the info string on the opening fence line.
				firstLine := strings.TrimSpace(body[:nl])
				if firstLine == "" || isFenceInfoString(firstLine) {
					lineEnd := nl + 1
					if body[nl] == '\r' && lineEnd < len(body) && body[lineEnd] == '\n' {
						lineEnd++
					}
					body = body[lineEnd:]
				}
			}
			q = strings.TrimSpace(body)
		}
	}

	return q
}

func normalizeTypography(query string) string {
	var normalized strings.Builder
	normalized.Grow(len(query))

	var quote rune
	escaped := false
	for index, current := range query {
		if escaped {
			normalized.WriteRune(current)
			escaped = false
			continue
		}
		if current == '\\' && quote != 0 {
			normalized.WriteRune(current)
			escaped = true
			continue
		}

		switch current {
		case '\u00a0':
			if quote == '\'' || quote == '"' {
				normalized.WriteRune(current)
				continue
			}
			normalized.WriteByte(' ')
		case '\u200b', '\u200c', '\u200d', '\ufeff':
			if quote == '\'' || quote == '"' {
				normalized.WriteRune(current)
			}
		case '\'', '"':
			if quote == '\u2019' && current == '\'' || quote == '\u201d' && current == '"' {
				normalized.WriteByte('\\')
			}
			normalized.WriteRune(current)
			if quote == 0 {
				quote = current
			} else if quote == current {
				quote = 0
			}
		case '\u2018', '\u2019':
			if quote == '\'' || quote == '"' {
				normalized.WriteRune(current)
				continue
			}
			if quote == '\u2019' && current == '\u2019' && isWordInternalCurlyApostrophe(query, index) {
				normalized.WriteRune(current)
				continue
			}
			normalized.WriteByte('\'')
			if quote == 0 {
				quote = '\u2019'
			} else if quote == '\u2019' {
				quote = 0
			}
		case '\u201c', '\u201d':
			if quote == '\'' || quote == '"' {
				normalized.WriteRune(current)
				continue
			}
			normalized.WriteByte('"')
			if quote == 0 {
				quote = '\u201d'
			} else if quote == '\u201d' {
				quote = 0
			}
		default:
			normalized.WriteRune(current)
		}
	}

	return normalized.String()
}

func isWordInternalCurlyApostrophe(query string, index int) bool {
	previous, _ := utf8.DecodeLastRuneInString(query[:index])
	_, size := utf8.DecodeRuneInString(query[index:])
	next, _ := utf8.DecodeRuneInString(query[index+size:])
	isWordRune := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
	}
	return isWordRune(previous) && isWordRune(next)
}

// isFenceInfoString reports whether s looks like a code-fence language tag
// rather than query content.
func isFenceInfoString(s string) bool {
	if len(s) > 20 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
