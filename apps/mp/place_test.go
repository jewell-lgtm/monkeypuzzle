package main

import (
	"errors"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// Two `mp create --remote` racing on one name both pass validatePlacement's
// PieceExists (it runs outside the placements lock); claimLink re-checks
// under the lock so only the first writes the pending link.
func TestClaimLink_RefusesTakenName(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repo := "/repo"
	if err := claimLink(repo, "fix-auth", "wire", fs); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	err := claimLink(repo, "fix-auth", "other", fs)
	if !errors.Is(err, piececmd.ErrPieceExists) {
		t.Fatalf("second claim err = %v, want ErrPieceExists", err)
	}
	p, err := piececmd.ReadPlacements(repo, fs)
	if err != nil {
		t.Fatal(err)
	}
	if got := p["fix-auth"]; got.Box != "wire" || !got.Pending {
		t.Errorf("placement = %+v, want the first claim's pending link", got)
	}
}

func TestBoxLockPath_FilenameSafe(t *testing.T) {
	got := piececmd.BoxLockPath("/repo", "u@wire:2222")
	if got != "/repo/.monkeypuzzle/box-u_wire_2222.lock" {
		t.Errorf("BoxLockPath = %q", got)
	}
}
