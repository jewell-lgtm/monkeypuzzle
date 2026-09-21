package main

import "testing"

func TestAtomicNounVocabulary(t *testing.T) {
	registerAtomCommands()

	for _, path := range [][]string{
		{"branch", "list"},
		{"branches", "show"},
		{"worktree", "list"},
		{"worktrees", "show"},
		{"piece", "show"},
		{"pieces", "list"},
		{"stacks", "show"},
		{"stack", "list"},
		{"prs", "create"},
		{"pr", "list"},
		{"prs", "show"},
		{"agents", "show"},
		{"events"},
		{"projects", "create"},
		{"project", "delete"},
		{"project", "show"},
	} {
		cmd, remaining, err := rootCmd.Find(path)
		if err != nil {
			t.Errorf("find %v: %v", path, err)
			continue
		}
		if len(remaining) != 0 {
			t.Errorf("find %v left arguments %v (resolved %q)", path, remaining, cmd.CommandPath())
		}
	}
}

func TestPieceNounCommandsReuseWorkflowFlags(t *testing.T) {
	registerAtomCommands()
	cmd, _, err := rootCmd.Find([]string{"piece", "create"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"name", "parent", "prompt", "schema", "json"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("piece create missing --%s", name)
		}
	}
}

func TestBareCollectionAtomsHaveReadDefaults(t *testing.T) {
	registerAtomCommands()
	for _, name := range []string{"branch", "piece", "stack", "project", "agent", "history", "worktree"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if cmd.RunE == nil {
			t.Errorf("bare mp %s has no read/default behavior", name)
		}
	}
	pr, _, err := rootCmd.Find([]string{"pr"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.RunE == nil {
		t.Error("bare mp pr should list locally recorded PR atoms")
	}
}
