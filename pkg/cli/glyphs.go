package cli

import (
	"fmt"
	"os"
)

// Status glyphs — the single source for human-facing prefixes, so every
// command marks success/warning/failure the same way.
const (
	GlyphOK   = "✓"
	GlyphWarn = "⚠"
	GlyphFail = "✗"
	GlyphInfo = "•"
)

// Hint prints a next-step suggestion to stderr for a human at a terminal.
// Pipes and agents never see hints — their contract is the JSON on stdout.
func Hint(msg string) {
	if IsTerminal() && IsStdoutTerminal() {
		fmt.Fprintf(os.Stderr, "→ next: %s\n", msg)
	}
}
