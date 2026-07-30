package integration_test

import (
	"encoding/json"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/integration"
)

func newTestHandler() (*integration.Handler, *adapters.MemoryFS) {
	fs := adapters.NewMemoryFS()
	h := integration.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()})
	return h, fs
}

func readSettings(t *testing.T, fs *adapters.MemoryFS) map[string]any {
	t.Helper()
	data, err := fs.ReadFile("/repo/.claude/settings.json")
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return settings
}

func TestInstallClaude_CreatesSettings(t *testing.T) {
	h, fs := newTestHandler()

	result, err := h.InstallClaude("/repo")
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if !result.Updated {
		t.Error("expected updated=true on fresh install")
	}

	hooks := readSettings(t, fs)["hooks"].(map[string]any)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Notification", "Stop", "SessionEnd"} {
		if _, ok := hooks[event]; !ok {
			t.Errorf("missing hook event %s", event)
		}
	}
}

func TestInstallClaude_Idempotent(t *testing.T) {
	h, _ := newTestHandler()

	if _, err := h.InstallClaude("/repo"); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	result, err := h.InstallClaude("/repo")
	if err != nil {
		t.Fatalf("second install failed: %v", err)
	}
	if result.Updated {
		t.Error("expected updated=false on second install")
	}
}

func TestInstallClaude_PreservesExistingSettings(t *testing.T) {
	h, fs := newTestHandler()

	existing := `{
  "permissions": {"allow": ["Bash(ls:*)"]},
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "say done"}]}]
  }
}`
	_ = fs.MkdirAll("/repo/.claude", 0755)
	_ = fs.WriteFile("/repo/.claude/settings.json", []byte(existing), 0644)

	if _, err := h.InstallClaude("/repo"); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	settings := readSettings(t, fs)
	if _, ok := settings["permissions"]; !ok {
		t.Error("existing permissions were dropped")
	}
	stopGroups := settings["hooks"].(map[string]any)["Stop"].([]any)
	if len(stopGroups) != 2 {
		t.Errorf("expected existing Stop group + mp group, got %d groups", len(stopGroups))
	}
}

func TestInstallClaude_RejectsInvalidExisting(t *testing.T) {
	h, fs := newTestHandler()
	_ = fs.MkdirAll("/repo/.claude", 0755)
	_ = fs.WriteFile("/repo/.claude/settings.json", []byte("{broken"), 0644)

	if _, err := h.InstallClaude("/repo"); err == nil {
		t.Error("expected error for invalid existing settings")
	}
}
