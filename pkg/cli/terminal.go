package cli

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal returns true if stdin is connected to an interactive terminal.
//
// This is a real isatty(3) check, not merely "stdin is a character device".
// The distinction matters: /dev/null (and /dev/zero, etc.) are character
// devices but not terminals, and agents/scripts driving mp through its
// stateless API frequently have stdin wired to /dev/null. A ModeCharDevice
// test would misreport those as interactive — launching a TUI that hangs, or
// (for session management) creating/hijacking a tmux session for a caller with
// no terminal to attach.
func IsTerminal() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// IsStdoutTerminal returns true if stdout is connected to an interactive terminal.
func IsStdoutTerminal() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// HasStdinData returns true if there is data available on stdin.
func HasStdinData() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode()&os.ModeCharDevice) == 0 && fi.Size() > 0 ||
		(fi.Mode()&os.ModeNamedPipe) != 0
}
