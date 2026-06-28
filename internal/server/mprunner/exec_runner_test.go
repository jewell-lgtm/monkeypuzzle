package mprunner

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestFake_RecordsCallsAndReturns(t *testing.T) {
	f := &Fake{Err: errors.New("boom")}
	in := StackGraphInput{RepoSlug: "o/r", Provider: "github", Token: "tok"}
	if _, err := f.StackGraph(context.Background(), in); err == nil {
		t.Fatal("want error from fake")
	}
	if len(f.Calls) != 1 || f.Calls[0] != in {
		t.Fatalf("call not recorded: %+v", f.Calls)
	}
}

func TestProviderOrDefault(t *testing.T) {
	if got := providerOrDefault(""); got != "github" {
		t.Fatalf("empty should default to github, got %q", got)
	}
	if got := providerOrDefault("gitlab"); got != "gitlab" {
		t.Fatalf("gitlab should pass through, got %q", got)
	}
}

func TestFindMp_OverrideWins(t *testing.T) {
	if got := findMp("/custom/path/mp"); got != "/custom/path/mp" {
		t.Fatalf("override not honored: %q", got)
	}
}

// TestExecRunner_StackGraph_E2E is a real-binary smoke test. It is skipped
// unless MP_E2E is set, so CI and local stay green without a built `mp` + token.
// Provide MP_E2E_REPO (owner/name) and GH_TOKEN; MP_BIN overrides the binary.
func TestExecRunner_StackGraph_E2E(t *testing.T) {
	if os.Getenv("MP_E2E") == "" {
		t.Skip("set MP_E2E to run the real `mp stack graph` smoke test")
	}
	r := NewExecRunner(os.Getenv("MP_BIN"))
	if _, err := r.StackGraph(context.Background(), StackGraphInput{
		RepoSlug:      os.Getenv("MP_E2E_REPO"),
		DefaultBranch: "main",
		Provider:      "github",
		Token:         os.Getenv("GH_TOKEN"),
	}); err != nil {
		t.Fatal(err)
	}
}
