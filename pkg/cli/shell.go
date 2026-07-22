package cli

import "strings"

// ShQuote quotes s for a POSIX shell: wrap in single quotes, with embedded
// single quotes spliced out as quoted-backslash-quote.
func ShQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
