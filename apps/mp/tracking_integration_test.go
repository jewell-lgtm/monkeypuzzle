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
	first := run("put", "--state", "working", "--note", "test")
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
	run("put", "--state", "done")
	// Explicit selectors still address the same item after the worktree is gone.
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, stderr, err := env.run("tracking", "delete", "--project-root", env.tmpDir, "--piece", "feature")
		if err != nil {
			t.Fatalf("delete: %v %s %s", err, out, stderr)
		}
	}
	stored, err := st.ListTrackedItems(context.Background(), uid)
	if err != nil || len(stored) != 0 {
		t.Fatalf("delete failed: %+v %v", stored, err)
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
