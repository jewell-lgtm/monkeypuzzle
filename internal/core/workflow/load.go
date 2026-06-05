package workflow

import (
	"encoding/json"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// LoadForRepo reads monkeypuzzle.json at repoRoot and returns the resolved
// workflow. Falls back to Default() when the config is unreadable or has no
// workflow block. An invalid workflow block returns the parse error so the
// caller can surface it.
func LoadForRepo(repoRoot string, fs core.FS) (Workflow, error) {
	configPath := projectdir.ConfigFilePath(repoRoot)
	data, err := fs.ReadFile(configPath)
	if err != nil {
		// No config — default workflow. Same fallback as ListIssues etc.
		return Default(), nil
	}
	return ParseConfigBytes(data)
}

// ParseConfigBytes extracts the workflow block from a raw config blob and
// returns the resolved workflow. Tolerates missing/empty workflow keys by
// returning Default().
func ParseConfigBytes(configData []byte) (Workflow, error) {
	var env struct {
		Workflow json.RawMessage `json:"workflow"`
	}
	if err := json.Unmarshal(configData, &env); err != nil {
		return Workflow{}, err
	}
	return ParseJSON(env.Workflow)
}
