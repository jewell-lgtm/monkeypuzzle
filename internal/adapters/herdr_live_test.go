package adapters

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Exercise the installed CLI, not just mocks that can share adapter mistakes.
// The test owns a separate server, config directory and shell workspace.
func TestHerdrLive(t *testing.T) {
	if os.Getenv("MP_TEST_HERDR") != "1" {
		t.Skip("set MP_TEST_HERDR=1 to test the installed herdr")
	}
	if _, err := exec.LookPath("herdr"); err != nil {
		t.Fatal(err)
	}
	// macOS limits Unix socket paths to 104 bytes; testing.T's default
	// temporary path plus Herdr's session suffix can exceed that limit.
	root, err := os.MkdirTemp("/tmp", "mp-herdr-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HERDR_CONFIG_PATH", filepath.Join(root, "config.toml"))
	t.Setenv("HERDR_SESSION", fmt.Sprintf("mp-test-%d", os.Getpid()))
	t.Setenv("HERDR_SOCKET", "")
	t.Setenv("HERDR_ENV", "")
	t.Setenv("HERDR_PANE_ID", "")
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte("onboarding = false\n[terminal]\ndefault_shell = \"/bin/sh\"\n[update]\nversion_check = false\nmanifest_check = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log, err := os.Create(filepath.Join(root, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	server := exec.CommandContext(ctx, "herdr", "server")
	server.Stdout, server.Stderr = log, log
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = exec.CommandContext(stopCtx, "herdr", "server", "stop").Run()
		cancel()
		_ = server.Wait()
	}()
	ready := false
	for i := 0; i < 50; i++ {
		if exec.CommandContext(ctx, "herdr", "workspace", "list").Run() == nil {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		data, _ := os.ReadFile(log.Name())
		t.Fatalf("server not ready: %s", data)
	}
	mux := NewHerdrMultiplexer(NewOSExec())
	name := "mp/test/compat"
	if err := mux.SwitchTo(ctx, name, root); err != nil {
		t.Fatal(err)
	}
	if !mux.Exists(ctx, name) {
		t.Fatal("created workspace not found")
	}
	panes, err := mux.ListPanes(ctx, name)
	if err != nil || len(panes) != 1 || panes[0].ID == "" {
		t.Fatalf("panes: %+v, %v", panes, err)
	}
	if err := mux.SendText(ctx, panes[0].ID, "printf 'mp-herdr-%s\\n' smoke-ok"); err != nil {
		t.Fatal(err)
	}
	found := false
	for i := 0; i < 30; i++ {
		out, err := mux.CapturePane(ctx, panes[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "mp-herdr-smoke-ok") {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		out, _ := mux.CapturePane(ctx, panes[0].ID)
		t.Fatalf("pane output was not captured: %q", out)
	}
	// A custom kind checks provider independence without launching a real agent.
	out, err := exec.CommandContext(ctx, "herdr", "pane", "report-agent", panes[0].ID, "--source", "mp-test", "--agent", "custom-agent", "--state", "blocked").CombinedOutput()
	if err != nil {
		t.Fatalf("report agent: %v: %s", err, out)
	}
	agents, err := mux.ObserveAgents(ctx, name)
	if err != nil || len(agents) != 1 || agents[0].Kind != "custom-agent" || agents[0].Status != "blocked" {
		t.Fatalf("agents: %+v, %v", agents, err)
	}
	if err := mux.FocusPane(ctx, name, panes[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := mux.Kill(ctx, name); err != nil {
		t.Fatal(err)
	}
	if mux.Exists(ctx, name) {
		t.Fatal("closed workspace still exists")
	}
}
