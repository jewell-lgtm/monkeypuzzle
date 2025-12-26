package adapters

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Tmux provides tmux operations using an Exec interface
type Tmux struct {
	exec core.Exec
}

// NewTmux creates a Tmux adapter with the provided Exec interface
func NewTmux(exec core.Exec) *Tmux {
	return &Tmux{exec: exec}
}

// NewSession creates a new detached tmux session in the specified directory
func (t *Tmux) NewSession(sessionName, workDir string) error {
	_, err := t.exec.Run("tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
	if err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}

// AttachSession attaches to an existing tmux session
func (t *Tmux) AttachSession(sessionName string) error {
	_, err := t.exec.Run("tmux", "attach-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

