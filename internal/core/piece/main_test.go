package piece_test

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history/historytest"
)

// TestMain keeps every handler test's history writes out of the real
// ~/.local/state log.
func TestMain(m *testing.M) { historytest.Main(m) }
