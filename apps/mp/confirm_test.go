package main

import "testing"

// TestResolveApply covers the dry-run-by-default decision logic. The interactive
// branch is unreachable here: the test process has no controlling TTY, so
// cli.IsTerminal() is false and the confirm callback is never invoked.
func TestResolveApply(t *testing.T) {
	confirmCalled := false
	confirm := func() (bool, error) { confirmCalled = true; return true, nil }

	tests := []struct {
		name      string
		apply     bool
		dryRun    bool
		toDo      bool
		wantApply bool
		wantErr   bool
	}{
		{name: "apply flag forces apply", apply: true, wantApply: true},
		{name: "dry-run flag forces preview", dryRun: true, wantApply: false},
		{name: "both flags is contradictory", apply: true, dryRun: true, wantErr: true},
		{name: "non-interactive default previews", toDo: true, wantApply: false},
		{name: "non-interactive default with nothing to do", toDo: false, wantApply: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveApply(tt.apply, tt.dryRun, tt.toDo, confirm)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantApply {
				t.Errorf("resolveApply = %v, want %v", got, tt.wantApply)
			}
		})
	}

	if confirmCalled {
		t.Error("confirm callback should not run without an interactive terminal")
	}
}
