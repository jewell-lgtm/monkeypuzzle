// Package historytest isolates the history log in test binaries so handler
// tests never append to the developer's real ~/.local/state log.
package historytest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
)

// Main is a TestMain body: it points MP_HISTORY_FILE at a throwaway file for
// the whole test binary (child processes inherit it), runs the tests, then
// removes the file. Individual tests may still t.Setenv their own path.
func Main(m *testing.M) {
	dir, err := os.MkdirTemp("", "mp-history-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv(history.EnvFile, filepath.Join(dir, "history.jsonl")) //nolint:errcheck // test setup
	code := m.Run()
	os.RemoveAll(dir) //nolint:errcheck // best-effort cleanup
	os.Exit(code)
}
