//go:build integration

package main_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestCLI_AtomicNounAliasesAndNoTTYDefaults(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("atoms-test")

	stdout, stderr, err := env.run("piece", "create", "--name", "noun-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("piece create: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	for _, invocation := range [][]string{
		{"pieces", "show", "noun-piece", "--json"},
		{"piece", "list", "--json"},
		{"stacks", "show", "--json"},
		{"inboxes", "list", "--json"},
		{"projects", "--json"},
		{"agents", "--json"},
	} {
		stdout, stderr, err = env.run(invocation...)
		if err != nil {
			t.Errorf("mp %s: %v\nstdout: %s\nstderr: %s", strings.Join(invocation, " "), err, stdout, stderr)
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(stdout), &value); err != nil {
			t.Errorf("mp %s did not reserve stdout for JSON: %v\n%s", strings.Join(invocation, " "), err, stdout)
		}
	}

	stdout, stderr, err = env.run("prs")
	if err != nil || !strings.Contains(stdout, `"prs": []`) {
		t.Fatalf("bare prs should be a non-mutating JSON list: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
}

func TestCLI_PRAtomListsAndShowsRecordedAssociations(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	env.initGitRepo()
	env.initProject("atoms-test")
	if _, stderr, err := env.run("piece", "create", "--name", "with-pr", "--skip-switch"); err != nil {
		t.Fatalf("create: %v\n%s", err, stderr)
	}
	wt := filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", "with-pr")
	fs := adapters.NewOSFS("")
	meta, err := piececmd.ReadPieceMetadata(wt, fs)
	if err != nil {
		t.Fatal(err)
	}
	meta.Stack[0].PRNumber, meta.Stack[0].PRURL, meta.Stack[0].Status = 42, "https://example.test/pr/42", "OPEN"
	if err := piececmd.WritePieceMetadata(wt, *meta, fs); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := env.run("prs")
	if err != nil || !strings.Contains(stdout, `"number": 42`) || !strings.Contains(stdout, `"branch": "with-pr"`) {
		t.Fatalf("list: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
	stdout, stderr, err = env.run("pr", "show", "#42", "--json")
	if err != nil || !strings.Contains(stdout, `"url": "https://example.test/pr/42"`) {
		t.Fatalf("show: %v\nstdout=%s\nstderr=%s", err, stdout, stderr)
	}
}
