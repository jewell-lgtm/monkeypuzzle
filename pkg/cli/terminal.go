package cli

import "os"

// IsTerminal returns true if stdin is connected to a terminal.
func IsTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// IsStdoutTerminal returns true if stdout is connected to a terminal.
func IsStdoutTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
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
