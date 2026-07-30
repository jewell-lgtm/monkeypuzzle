package agent

import (
	"fmt"
	"strings"
)

// LaunchSpec is how to start an agent of a given kind in a piece.
type LaunchSpec struct {
	Kind string
	// Line is typed into a multiplexer pane's shell (interactive TUI mode).
	Line string
	// Argv runs headlessly (detached, output to a log) when there is no
	// session to type into. Empty when the kind has no headless mode for the
	// given input (headless requires a prompt).
	Argv []string
}

// launchKinds maps agent kind to its interactive shell line and headless argv.
var launchKinds = map[string]struct {
	interactive func(prompt string) string
	headless    func(prompt string) []string
}{
	"claude": {
		interactive: func(prompt string) string {
			if prompt == "" {
				return "claude"
			}
			return "claude " + shellQuote(prompt)
		},
		headless: func(prompt string) []string { return []string{"claude", "-p", prompt} },
	},
	"codex": {
		interactive: func(prompt string) string {
			if prompt == "" {
				return "codex"
			}
			return "codex " + shellQuote(prompt)
		},
		headless: func(prompt string) []string { return []string{"codex", "exec", prompt} },
	},
}

// ValidKinds returns the supported agent kinds.
func ValidKinds() []string {
	return []string{"claude", "codex"}
}

// BuildLaunch returns the launch spec for an agent kind and optional prompt.
func BuildLaunch(kind, prompt string) (LaunchSpec, error) {
	k, ok := launchKinds[kind]
	if !ok {
		return LaunchSpec{}, fmt.Errorf("unknown agent kind %q (valid: %s)", kind, strings.Join(ValidKinds(), ", "))
	}
	spec := LaunchSpec{Kind: kind, Line: k.interactive(prompt)}
	if prompt != "" {
		spec.Argv = k.headless(prompt)
	}
	return spec, nil
}

// shellQuote single-quotes s for a POSIX shell line.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
