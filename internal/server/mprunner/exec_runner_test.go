package mprunner

import (
	"context"
	"errors"
	"os"
	"strings"
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

// TestBuildCmd_TokenInEnvNotArgs pins the security contract: the forge token is
// passed to the child via GH_TOKEN in the env only (never in argv), and the
// server's secrets never leak into the child env.
func TestBuildCmd_TokenInEnvNotArgs(t *testing.T) {
	t.Setenv("TOKEN_ENCRYPTION_KEY", "supersecret")

	r := NewExecRunner("/bin/mp")
	cmd := r.buildCmd(context.Background(), StackGraphInput{
		RepoSlug:      "o/n",
		DefaultBranch: "main",
		Provider:      "github",
		Token:         "tok123",
	})

	for _, a := range cmd.Args {
		if strings.Contains(a, "tok123") {
			t.Fatalf("token leaked into argv: %q", cmd.Args)
		}
	}

	var tokenEntries int
	for _, e := range cmd.Env {
		if e == "GH_TOKEN=tok123" {
			tokenEntries++
		}
		if strings.Contains(e, "TOKEN_ENCRYPTION_KEY") || strings.Contains(e, "supersecret") {
			t.Fatalf("server secret leaked into child env: %q", e)
		}
	}
	if tokenEntries != 1 {
		t.Fatalf("want exactly one GH_TOKEN=tok123 env entry, got %d in %v", tokenEntries, cmd.Env)
	}
}

// TestBuildCmd_GitlabTokenInEnv covers the gitlab provider: the token rides in
// GITLAB_TOKEN, and secrets are still withheld.
func TestBuildCmd_GitlabTokenInEnv(t *testing.T) {
	t.Setenv("TOKEN_ENCRYPTION_KEY", "supersecret")

	r := NewExecRunner("/bin/mp")
	cmd := r.buildCmd(context.Background(), StackGraphInput{
		RepoSlug:      "o/n",
		DefaultBranch: "main",
		Provider:      "gitlab",
		Token:         "tok123",
	})

	for _, a := range cmd.Args {
		if strings.Contains(a, "tok123") {
			t.Fatalf("token leaked into argv: %q", cmd.Args)
		}
	}

	var tokenEntries int
	for _, e := range cmd.Env {
		if e == "GITLAB_TOKEN=tok123" {
			tokenEntries++
		}
		if strings.Contains(e, "TOKEN_ENCRYPTION_KEY") || strings.Contains(e, "supersecret") {
			t.Fatalf("server secret leaked into child env: %q", e)
		}
	}
	if tokenEntries != 1 {
		t.Fatalf("want exactly one GITLAB_TOKEN=tok123 env entry, got %d in %v", tokenEntries, cmd.Env)
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
