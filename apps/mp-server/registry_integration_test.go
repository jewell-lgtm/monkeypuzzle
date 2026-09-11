//go:build integration

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

// Uses a disposable Postgres database and a local signing-key fixture. Both CLI
// and server are real processes, including production JWT verification/routes.
func TestRegistryServerWithoutTemporal(t *testing.T) {
	dsn := os.Getenv("MP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MP_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	st, err := store.NewPgxStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	sub := fmt.Sprintf("registry-demo-%d", time.Now().UnixNano())
	uid, err := st.UpsertUser(ctx, store.User{ExternalUserID: sub, Provider: "github", ForgeUserID: time.Now().UnixNano(), ForgeLogin: "demo-developer", AccessTokenEnc: []byte{}})
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "demo", "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
	}))
	defer jwks.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"sub": sub, "iss": jwks.URL, "aud": base, "exp": time.Now().Add(time.Hour).Unix()})
	token.Header["kid"] = "demo"
	bearer, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, pkg := range map[string]string{"mp-server": ".", "mp": "../mp"} {
		if out, err := exec.Command("go", "build", "-o", filepath.Join(dir, name), pkg).CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	secret := strings.Repeat("a", 32)
	serverEnv := append(os.Environ(), "PORT="+fmt.Sprint(port), "DATABASE_URL="+dsn, "PUBLIC_BASE_URL="+base,
		"WORKOS_API_KEY=test-only", "WORKOS_CLIENT_ID=test-only", "AUTHKIT_DOMAIN="+jwks.URL, "WORKOS_JWKS_URL="+jwks.URL,
		"SESSION_SECRET="+base64.StdEncoding.EncodeToString([]byte(secret)), "TOKEN_ENCRYPTION_KEY="+base64.StdEncoding.EncodeToString([]byte(secret)),
		"PR_SYNC_ENABLED=false", "TEMPORAL_HOSTPORT=127.0.0.1:1", "SECURE_COOKIES=false")
	start := func() func() {
		logfile, err := os.CreateTemp(dir, "server-*.log")
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(filepath.Join(dir, "mp-server"), "serve")
		cmd.Env = serverEnv
		cmd.Stdout = logfile
		cmd.Stderr = logfile
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
			}
			logfile.Close()
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			res, err := (&http.Client{Timeout: time.Second}).Get(base + "/readyz")
			if err == nil {
				res.Body.Close()
				if res.StatusCode == 200 {
					return stop
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		stop()
		logs, _ := os.ReadFile(logfile.Name())
		t.Fatalf("server did not start without Temporal: %s", logs)
		return stop
	}
	stop := start()
	configDir := filepath.Join(dir, "config")
	os.MkdirAll(configDir, 0700)
	os.WriteFile(filepath.Join(configDir, "config.json"), []byte(`{"multiplexer":"none"}`), 0600)
	cliEnv := append(os.Environ(), "MP_CONFIG_DIR="+configDir, "MP_DATA_DIR="+filepath.Join(dir, "data"), "MP_SERVER_URL="+base, "MP_SERVER_TOKEN="+bearer)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(filepath.Join(dir, "mp"), append([]string{"tracking"}, args...)...)
		cmd.Env = cliEnv
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("mp tracking %v: %v %s", args, err, out)
		}
		return string(out)
	}
	selectors := []string{"--project-root", dir, "--piece", "registry-demo"}
	args := append([]string{"put", "--state", "working", "--note", "Published without PR monitoring"}, selectors...)
	first := run(args...)
	if run(args...) != first {
		t.Fatal("PUT retry changed persisted record")
	}
	t.Log("CLI published a piece through real JWT authentication; retry returned the identical record.")
	codec := session.NewSecureCookieCodec([]byte(secret))
	cookie, _ := codec.Encode(uid)
	req, _ := http.NewRequest("GET", base+"/", nil)
	req.AddCookie(&http.Cookie{Name: session.CookieName, Value: cookie})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(html), "registry-demo") || strings.Contains(string(html), "Sync now") {
		t.Fatalf("registry dashboard: %d %s", res.StatusCode, html)
	}
	if preview := os.Getenv("MP_TEST_REGISTRY_PREVIEW_DIR"); preview != "" {
		if err := os.MkdirAll(preview, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(preview, "index.html"), html, 0600); err != nil {
			t.Fatal(err)
		}
		css, err := os.ReadFile("../../internal/server/web/static/app.css")
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Join(preview, "static"), 0700)
		if err := os.WriteFile(filepath.Join(preview, "static/app.css"), css, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stop()
	// Even explicitly enabled PR monitoring must not take the registry down.
	serverEnv = append(serverEnv, "PR_SYNC_ENABLED=true")
	stop = start()
	defer stop()
	var items tracking.List
	if err := json.Unmarshal([]byte(run("list")), &items); err != nil || len(items.Items) != 1 {
		t.Fatalf("restart persistence: %+v %v", items, err)
	}
	t.Log("Dashboard showed the piece; restarting mp-server preserved it in Postgres, with Temporal absent.")
	for range 2 {
		run(append([]string{"delete"}, selectors...)...)
	}
	if err := json.Unmarshal([]byte(run("list")), &items); err != nil || len(items.Items) != 0 {
		t.Fatalf("delete: %+v %v", items, err)
	}
	t.Log("Repeated DELETE succeeded; the developer's registry is empty again.")
}
