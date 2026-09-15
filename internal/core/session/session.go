// Package session centralises tmux session naming for monkeypuzzle.
//
// Sessions are namespaced by project so that worktrees from different
// repositories never collide:
//
//	mp/{project}            -> a project's main worktree
//	mp/{project}/{piece}    -> a piece worktree within that project
package session

import "strings"

// Prefix is the leading component shared by all monkeypuzzle tmux sessions.
const Prefix = "mp"

// Sanitize makes s safe to embed in a tmux session name. tmux disallows "."
// and ":" in session names; we also collapse whitespace to "-". The absence
// of ":" is also load-bearing for the herdr adapter: herdr pane ids are
// "wN:pN", so a colon in a PaneOps target unambiguously means pane, not
// session.
func Sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '.', ':':
			b.WriteByte('-')
		default:
			if r == ' ' || r == '\t' || r == '\n' {
				b.WriteByte('-')
			} else {
				b.WriteRune(r)
			}
		}
	}
	out := b.String()
	out = strings.Trim(out, "-")
	if out == "" {
		return "unnamed"
	}
	return out
}

// MainName returns the session name for a project's main worktree.
func MainName(project string) string {
	return Prefix + "/" + Sanitize(project)
}

// Name returns the session name for a piece worktree within a project.
func Name(project, piece string) string {
	return Prefix + "/" + Sanitize(project) + "/" + Sanitize(piece)
}
