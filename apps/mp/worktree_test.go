package main

import (
	"testing"

	worktreecmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/worktree"
)

func TestWorktreeTUIDefaultIsReadOnly(t *testing.T) {
	options := worktreeActionOptions(worktreecmd.Info{Branch: "feature"})
	if len(options) < 2 {
		t.Fatalf("expected management actions, got %+v", options)
	}
	if options[0].Value != "show" {
		t.Fatalf("default action = %q, want read-only show", options[0].Value)
	}
}

func TestWorktreeTUIOffersNoDeleteForCurrentOrMain(t *testing.T) {
	for _, target := range []worktreecmd.Info{{Current: true}, {Main: true}} {
		options := worktreeActionOptions(target)
		if len(options) != 1 || options[0].Value != "show" {
			t.Errorf("unsafe target %+v offered actions %+v", target, options)
		}
	}
}

func TestWorktreeTUIUsesMPActionsForUnmanagedWorktrees(t *testing.T) {
	options := worktreeActionOptions(worktreecmd.Info{Branch: "agent-work", Path: "/tmp/agent-work"})
	if len(options) != 2 || options[0].Value != "show" || options[1].Value != "adopt" {
		t.Fatalf("unmanaged worktree actions = %+v, want inspect/adopt only", options)
	}
}

func TestWorktreeTUIComposesPieceLifecycle(t *testing.T) {
	active := worktreeActionOptions(worktreecmd.Info{Managed: true, Piece: "active"})
	if len(active) != 4 || active[1].Value != "delete" || active[2].Value != "delete-branch" || active[3].Value != "force" {
		t.Fatalf("active piece actions = %+v", active)
	}
	merged := worktreeActionOptions(worktreecmd.Info{Managed: true, Piece: "merged", Lifecycle: "merged"})
	if len(merged) != 3 || merged[1].Value != "done" || merged[2].Value != "delete" {
		t.Fatalf("merged piece actions = %+v", merged)
	}
}

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(1536); got != "1.5 KiB" {
		t.Fatalf("humanBytes(1536) = %q", got)
	}
}
