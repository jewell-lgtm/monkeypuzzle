package open

import (
	"strings"
	"testing"
)

func target() Target {
	return Target{Path: "/tmp/pieces/fix x", Piece: "fix-x", Project: "alpha", Branch: "feat/fix-x"}
}

func TestCommand_AppendsPathWhenNoPlaceholder(t *testing.T) {
	got, err := Command("code", target())
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if got != `code '/tmp/pieces/fix x'` {
		t.Errorf("bare command should get the path appended, got %q", got)
	}
}

func TestCommand_SubstitutesEveryPlaceholder(t *testing.T) {
	got, err := Command("edit {path} --title {piece} --repo {project} --branch {branch}", target())
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	for _, want := range []string{`'/tmp/pieces/fix x'`, `'fix-x'`, `'alpha'`, `'feat/fix-x'`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in %q", want, got)
		}
	}
}

// A path is user data (branch names become directory names), so it must not be
// able to run a second command.
func TestCommand_QuotesShellMetacharacters(t *testing.T) {
	got, err := Command("code {path}", Target{Path: `/tmp/a'; rm -rf /; '`})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if !strings.Contains(got, `'\''`) {
		t.Errorf("path was not quoted: %q", got)
	}
}

func TestCommand_ErrorsWithoutTemplateOrPath(t *testing.T) {
	if _, err := Command("  ", target()); err == nil {
		t.Error("empty template should error")
	}
	if _, err := Command("code", Target{}); err == nil {
		t.Error("empty path should error")
	}
}
