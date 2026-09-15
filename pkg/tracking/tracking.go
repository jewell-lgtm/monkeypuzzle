// Package tracking defines the opt-in mp-server progress protocol.
package tracking

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const BasePath = "/api/v1/tracking/items"
const MaxBody = 64 << 10

type Key struct {
	MachineID string `json:"machine_id"`
	ProjectID string `json:"project_id"`
	PieceID   string `json:"piece_id"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func (k Key) Validate() error {
	for _, id := range []string{k.MachineID, k.ProjectID, k.PieceID} {
		if !identifier.MatchString(id) {
			return fmt.Errorf("identifiers must be 1–128 ASCII letters, digits, underscores or hyphens")
		}
	}
	return nil
}

func (k Key) Path() string { return BasePath + "/" + k.MachineID + "/" + k.ProjectID + "/" + k.PieceID }

// Snapshot is a full replacement, not a patch or a claim of live agent state.
type Snapshot struct {
	Machine      string `json:"machine"`
	Project      string `json:"project"`
	Piece        string `json:"piece"`
	State        string `json:"state"`
	Note         string `json:"note,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	Parent       string `json:"parent,omitempty"`
}

func (s Snapshot) Validate() error {
	for _, value := range []string{s.Machine, s.Project, s.Piece} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("machine, project and piece labels are required")
		}
	}
	switch s.State {
	case "todo", "working", "blocked", "review", "done":
	default:
		return fmt.Errorf("state must be todo, working, blocked, review or done")
	}
	for _, value := range []string{s.Machine, s.Project, s.Piece, s.State, s.Note, s.WorktreePath, s.Parent} {
		if len(value) > 16000 || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("fields must be at most 16000 bytes and contain no NUL")
		}
	}
	return nil
}

type Item struct {
	Key
	Snapshot
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type List struct {
	Items []Item `json:"items"`
}
