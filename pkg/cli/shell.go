package cli

import (
	"fmt"
	"strings"
)

// ShQuote quotes s for a POSIX shell: wrap in single quotes, with embedded
// single quotes spliced out as quoted-backslash-quote.
func ShQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ValidSSHDest rejects ssh destinations that could be parsed as ssh options
// or smuggle shell syntax — the registry file is user-writable, so a poisoned
// host must fail closed rather than reach ssh's argv.
func ValidSSHDest(host string) error {
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\n'\"`$;|&\\") {
		return fmt.Errorf("invalid ssh host %q", host)
	}
	return nil
}
