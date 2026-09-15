package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

func TestPrivateRegistryWithoutPRMonitoring(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	alice, _ := st.UpsertUser(ctx, store.User{ExternalUserID: "alice", ForgeUserID: 1})
	bob, _ := st.UpsertUser(ctx, store.User{ExternalUserID: "bob", ForgeUserID: 2})
	for i, machine := range []string{"laptop", "desktop"} {
		key := tracking.Key{MachineID: machine, ProjectID: "project", PieceID: machine}
		state := "working"
		if i == 1 {
			state = "blocked"
		}
		_, err := st.PutTrackedItem(ctx, alice, key, tracking.Snapshot{Machine: machine, Project: "app", Piece: "alice-" + machine, State: state, Note: "<script>alert('task')</script>"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = st.PutTrackedItem(ctx, bob, key, tracking.Snapshot{Machine: machine, Project: "secret-project", Piece: "bob-secret", State: "working"})
		if err != nil {
			t.Fatal(err)
		}
	}
	codec := session.NewSecureCookieCodec([]byte("registry-test-secret-32-bytes-long"))
	h := NewHandler(Deps{Store: st, Service: service.New(st, nil, nil, nil, false), Session: codec})
	mux := http.NewServeMux()
	h.Routes(mux)
	request := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		cookie, _ := codec.Encode(alice)
		req.AddCookie(&http.Cookie{Name: session.CookieName, Value: cookie})
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	page := request("/")
	if page.Code != 200 {
		t.Fatalf("registry: %d %s", page.Code, page.Body.String())
	}
	if page.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private registry must not be cached")
	}
	body := page.Body.String()
	for _, want := range []string{"Your pieces", "alice-laptop", "alice-desktop", "2 machines", "2 pieces", "&lt;script&gt;"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"bob-secret", "secret-project", "Sync now", "PR monitoring", "<script>alert"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("registry contains %q", forbidden)
		}
	}
	for _, filter := range []string{"?machine=desktop", "?state=blocked", "?q=alice-desktop"} {
		body := request("/" + filter).Body.String()
		if strings.Contains(body, "alice-laptop") || !strings.Contains(body, "alice-desktop") {
			t.Errorf("filter %s failed", filter)
		}
	}
	if body := request("/?project=not-published").Body.String(); !strings.Contains(body, "No pieces match") {
		t.Fatal("empty filter result missing")
	}
	for _, path := range []string{"/repositories", "/partials/repos", "/sync/status"} {
		if rec := request(path); rec.Code != 404 {
			t.Errorf("disabled PR route %s: %d", path, rec.Code)
		}
	}
}
