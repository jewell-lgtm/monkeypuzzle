//go:build integration

package main_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/trackingapi"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestCLI_TrackingOptInAndRoundTrip(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	st := store.NewMemoryStore()
	uid, err := st.UpsertUser(context.Background(), store.User{ExternalUserID: "cli-user", Provider: "github", ForgeUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "test-token" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: strconv.FormatInt(uid, 10), Expiration: time.Now().Add(time.Hour)}, nil
	}
	handler := trackingapi.NewHandler(st, verifier, "https://example.test/metadata")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); handler.ServeHTTP(w, r) }))
	defer srv.Close()
	t.Setenv("MP_SERVER_URL", srv.URL)
	t.Setenv("MP_SERVER_TOKEN", "test-token")
	env.initGitRepo()
	env.initProject("tracking")
	help, stderr, err := env.run("--help")
	if err != nil {
		t.Fatalf("root help: %v\n%s", err, stderr)
	}
	for _, want := range []string{"mp create -> mp sync -> mp pr create", "Piece workflow:", "Collaboration:", "settle"} {
		if !strings.Contains(help, want) {
			t.Errorf("root help missing %q:\n%s", want, help)
		}
	}
	trackingHelp, _, err := env.run("tracking", "--help")
	if err != nil || !strings.Contains(trackingHelp, "report") || strings.Contains(trackingHelp, "delete") {
		t.Fatalf("tracking help should teach report + settle, not delete: %v\n%s", err, trackingHelp)
	}
	var settleSchema map[string]any
	settleSchemaOut, _, err := env.run("settle", "--schema")
	if err != nil || json.Unmarshal([]byte(settleSchemaOut), &settleSchema) != nil || settleSchema["piece"] == "" {
		t.Fatalf("settle schema: %v\n%s", err, settleSchemaOut)
	}
	for _, args := range [][]string{{"create", "--name", "feature", "--skip-switch"}, {"list", "--json"}, {"status", "--json"}, {"inbox", "--json"}, {"tracking", "--help"}} {
		if out, stderr, err := env.run(args...); err != nil {
			t.Fatalf("%v: %v\n%s\n%s", args, err, out, stderr)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("ordinary commands contacted mp-server")
	}
	identityFile := filepath.Join(env.configDir, "tracking", "identity.json")
	if _, err := os.Stat(identityFile); !os.IsNotExist(err) {
		t.Fatal("ordinary commands initialized tracking identity")
	}
	worktree := filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", "feature")
	run := func(args ...string) string {
		t.Helper()
		out, stderr, err := env.runInDir(worktree, append([]string{"tracking"}, args...)...)
		if err != nil {
			t.Fatalf("%v: %v\n%s\n%s", args, err, out, stderr)
		}
		return out
	}
	identity := run("identity")
	if requests.Load() != 0 {
		t.Fatal("identity contacted mp-server")
	}
	if identity != run("identity") {
		t.Fatal("identity changed across CLI processes")
	}
	first := run("report", "--state", "working", "--note", "test")
	// The old API-shaped spelling remains an alias, but is no longer the
	// discoverable command.
	if first != run("put", "--state", "working", "--note", "test") {
		t.Fatal("repeated PUT changed snapshot")
	}
	var item tracking.Item
	if err := json.Unmarshal([]byte(first), &item); err != nil {
		t.Fatal(err)
	}
	if item.Piece != "feature" || item.WorktreePath == "" || item.Parent != "main" {
		t.Fatalf("context missing: %+v", item)
	}
	var items tracking.List
	if err := json.Unmarshal([]byte(run("list")), &items); err != nil || len(items.Items) != 1 {
		t.Fatalf("list: %+v %v", items, err)
	}
	run("report", "--state", "done")
	// Explicit selectors still address the same item after the worktree is gone.
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, stderr, err := env.run("settle", "feature", "--json")
		if err != nil {
			t.Fatalf("settle: %v %s %s", err, out, stderr)
		}
		var settled map[string]any
		if err := json.Unmarshal([]byte(out), &settled); err != nil || settled["settled"] != true || settled["piece"] != "feature" {
			t.Fatalf("settle result: %+v %v", settled, err)
		}
	}
	stored, err := st.ListTrackedItems(context.Background(), uid)
	if err != nil || len(stored) != 0 {
		t.Fatalf("settle failed: %+v %v", stored, err)
	}
	t.Setenv("MP_SERVER_URL", "")
	if _, stderr, err := env.run("tracking", "list"); err == nil || !strings.Contains(stderr, "opt-in") {
		t.Fatalf("missing opt-in accepted: %v %s", err, stderr)
	}
	// Ordinary commands continue succeeding with absent and invalid server config.
	for _, server := range []string{"", "not-a-url"} {
		t.Setenv("MP_SERVER_URL", server)
		before := requests.Load()
		if _, stderr, err := env.run("status", "--json"); err != nil {
			t.Fatalf("server config affected status: %v %s", err, stderr)
		}
		if requests.Load() != before {
			t.Fatal("status contacted server")
		}
	}
}
