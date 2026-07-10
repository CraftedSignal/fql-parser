package fql

import "strings"

// NormalizeQuery cleans pasted or machine-extracted query text so it lexes
// the way the author intended: markdown code fences are stripped, smart
// quotes become ASCII quotes, and zero-width/non-breaking spaces are removed.
func NormalizeQuery(query string) string {
	q := strings.TrimSpace(query)

	// Strip markdown code fences: ```fql\n...\n``` or ```\n...\n```
	if strings.HasPrefix(q, "```") {
		if end := strings.LastIndex(q, "```"); end > 0 {
			body := q[3:end]
			if nl := strings.IndexByte(body, '\n'); nl >= 0 {
				// Drop the info string on the opening fence line.
				firstLine := strings.TrimSpace(body[:nl])
				if firstLine == "" || isFenceInfoString(firstLine) {
					body = body[nl+1:]
				}
			}
			q = strings.TrimSpace(body)
		}
	}

	replacer := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", // smart single quotes
		"\u201c", `"`, "\u201d", `"`, // smart double quotes
		"\u00a0", " ", // non-breaking space
		"\u200b", "", "\u200c", "", "\u200d", "", "\ufeff", "", // zero-width chars and BOM
	)
	return replacer.Replace(q)
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
