package skill

import "encoding/json"

// Scope says where a skill was installed.
const (
	ScopeProject = "project"
	ScopeUser    = "user"
)

// DefaultSkill is written when no name is given, keeping `mp skill create` and
// the deprecated `mp claude skill` interchangeable.
const DefaultSkill = "managing-monkeypuzzle"

// Input selects a skill and where to write it.
type Input struct {
	Name string `json:"name,omitempty"`
	// User writes under the home directory instead of the repo, for skills
	// that are useful outside any one project.
	User bool `json:"user,omitempty"`
}

// Result reports what was written.
type Result struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Link is the .claude/skills path pointing at Path, empty when something
	// that is not a link already occupies it.
	Link   string `json:"link,omitempty"`
	Scope  string `json:"scope"`
	Status string `json:"status"`
}

// WithDefaults fills the skill name in.
func WithDefaults(in Input) Input {
	if in.Name == "" {
		in.Name = DefaultSkill
	}
	return in
}

// Schema returns an example input document. It is built as a map so the
// omitempty fields still appear in the document a caller is meant to edit.
func Schema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"name": DefaultSkill,
		"user": false,
	}, "", "  ")
}
