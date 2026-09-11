package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/workos"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

func TestRegistryLoginWithoutForgeAccess(t *testing.T) {
	mem := store.NewMemoryStore()
	codec := session.NewSecureCookieCodec([]byte("registry-login-regression-test-secret"))
	login := workos.NewStubClient("user_one", "")
	h := NewHandler(Deps{Store: mem, Service: service.New(mem, nil, nil, nil, false), Session: codec, Logins: map[string]identity.Provider{"github": login}})
	mux := http.NewServeMux()
	h.Routes(mux)
	callback := func(subject string, validState bool) *httptest.ResponseRecorder {
		login.Identity.ProviderUserID = subject
		req := httptest.NewRequest("GET", "/auth/callback?state=test-state&code=verified-code", nil)
		if validState {
			req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: "test-state"})
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := callback("user_one", false); rec.Code != 400 {
		t.Fatalf("CSRF status: %d", rec.Code)
	}
	ids := map[string]int64{}
	for _, subject := range []string{"user_one", "user_two", "user_one"} {
		rec := callback(subject, true)
		if rec.Code != 303 || rec.Header().Get("Location") != "/" {
			t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
		}
		response := rec.Result()
		defer func() { _ = response.Body.Close() }()
		var id int64
		for _, cookie := range response.Cookies() {
			if cookie.Name == session.CookieName {
				var err error
				id, err = codec.Decode(cookie.Value)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if id == 0 {
			t.Fatal("missing valid session")
		}
		if prior, ok := ids[subject]; ok && prior != id {
			t.Fatal("login created a different account")
		}
		ids[subject] = id
		user, err := mem.GetUserByExternalID(context.Background(), subject)
		if err != nil || user.ID != id {
			t.Fatalf("account binding: %v", err)
		}
	}
	if ids["user_one"] == ids["user_two"] {
		t.Fatal("accounts were combined")
	}
}
