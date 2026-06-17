package piece

import (
	"encoding/json"
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// ReadConfig reads the monkeypuzzle config from the repository root.
func ReadConfig(repoRoot string, fs core.FS) (*initcmd.Config, error) {
	configPath := projectdir.ConfigFilePath(repoRoot)

	data, err := fs.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg initcmd.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}
