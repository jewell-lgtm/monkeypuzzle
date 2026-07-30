package agent_test

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
)

func TestBuildLaunch(t *testing.T) {
	cases := []struct {
		kind, prompt string
		wantLine     string
		wantArgv     []string
		wantErr      bool
	}{
		{"claude", "", "claude", nil, false},
		{"claude", "add dark mode", `claude 'add dark mode'`, []string{"claude", "-p", "add dark mode"}, false},
		{"claude", "don't break", `claude 'don'\''t break'`, []string{"claude", "-p", "don't break"}, false},
		{"codex", "fix the bug", `codex 'fix the bug'`, []string{"codex", "exec", "fix the bug"}, false},
		{"cursor", "hi", "", nil, true},
	}
	for _, tc := range cases {
		spec, err := agent.BuildLaunch(tc.kind, tc.prompt)
		if tc.wantErr {
			if err == nil {
				t.Errorf("BuildLaunch(%q): expected error", tc.kind)
			}
			continue
		}
		if err != nil {
			t.Fatalf("BuildLaunch(%q, %q): %v", tc.kind, tc.prompt, err)
		}
		if spec.Line != tc.wantLine {
			t.Errorf("Line = %q, want %q", spec.Line, tc.wantLine)
		}
		if len(spec.Argv) != len(tc.wantArgv) {
			t.Errorf("Argv = %v, want %v", spec.Argv, tc.wantArgv)
			continue
		}
		for i := range spec.Argv {
			if spec.Argv[i] != tc.wantArgv[i] {
				t.Errorf("Argv = %v, want %v", spec.Argv, tc.wantArgv)
				break
			}
		}
	}
}
