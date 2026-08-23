package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// SGR codes for mp's small terminal palette. Presentation only — every colored
// string flows through a Painter, which leaves text untouched off-terminal.
const (
	SGRGreen  = "32"
	SGRYellow = "33"
	SGRRed    = "31"
	SGRCyan   = "36"
	SGRDim    = "2"
)

// Painter colors strings for one output stream. The zero value never paints,
// so pipes, tests, and JSON mode need no special-casing.
type Painter struct{ enabled bool }

// NewPainter returns a Painter that emits ANSI colors only when f is an
// interactive terminal and NO_COLOR is unset.
func NewPainter(f *os.File) Painter {
	if f == nil || os.Getenv("NO_COLOR") != "" {
		return Painter{}
	}
	fd := f.Fd()
	return Painter{enabled: isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)}
}

// Paint wraps s in the given SGR code when coloring is enabled.
func (p Painter) Paint(code, s string) string {
	if !p.enabled || s == "" {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}
