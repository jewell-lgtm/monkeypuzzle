package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/workos"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

type noopTrigger struct{}

func (noopTrigger) StartSync(context.Context, int64) (string, error) { return "", nil }
func (noopTrigger) SyncStatus(context.Context, int64) (store.SyncStatus, error) {
	return store.SyncStatus{Status: store.SyncIdle}, nil
}

// Light smoke test of the HTML surface (the MCP suite is the primary one): the
// dashboard shell renders, the repos fragment lists the repo, and unauthenticated
// requests are redirected to login.
func TestWeb_Smoke(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	uid, _ := mem.UpsertUser(ctx, store.User{ExternalUserID: "ext1", GitHubUserID: 1, GitHubLogin: "octo"})
	rid, _ := mem.UpsertRepo(ctx, store.Repo{GitHubRepoID: 7, Owner: "o", Name: "r", DefaultBranch: "main"})
	_ = mem.SetUserRepos(ctx, uid, []int64{rid})

	key := make([]byte, crypto.KeySize)
	cipher, _ := crypto.NewAESGCMCipher(key)
	codec := session.NewSecureCookieCodec([]byte("smoke-secret-smoke-secret-smoke-1"))

	h := NewHandler(Deps{
		Service: service.New(mem, noopTrigger{}),
		Store:   mem,
		Login:   workos.NewStubClient("ext1", "ghtok"),
		GitHub:  forge.NewStubFactory(),
		Session: codec,
		Cipher:  cipher,
	})
	mux := http.NewServeMux()
	h.Routes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Unauthenticated GET / redirects to /login.
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := noRedirect.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Fatalf("unauthenticated /: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	// Authenticated: craft a valid session cookie.
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(ts.URL)
	val, _ := codec.Encode(uid)
	jar.SetCookies(u, []*http.Cookie{{Name: session.CookieName, Value: val}})
	authed := &http.Client{Jar: jar}

	if body := getBody(t, authed, ts.URL+"/"); !strings.Contains(body, "Your repositories") {
		t.Fatalf("dashboard shell missing: %s", body)
	}
	if body := getBody(t, authed, ts.URL+"/partials/repos"); !strings.Contains(body, "o/r") {
		t.Fatalf("repos fragment missing repo: %s", body)
	}
}

func getBody(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}
