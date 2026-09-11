package trackingclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
)

func TestIdentityPersistsAndSerializesFirstUse(t *testing.T) {
	dir := t.TempDir()
	keys := make(chan tracking.Key, 20)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			key, err := Identity(dir, "/project", "feature")
			if err != nil {
				t.Error(err)
				return
			}
			keys <- key
		})
	}
	wg.Wait()
	close(keys)
	first := <-keys
	for key := range keys {
		if key != first {
			t.Fatalf("racing identities: %+v %+v", first, key)
		}
	}
	second, err := Identity(dir, "/project", "other")
	if err != nil || second.MachineID != first.MachineID || second.ProjectID != first.ProjectID || second.PieceID == first.PieceID {
		t.Fatalf("piece isolation: %+v %v", second, err)
	}
	third, err := Identity(dir, "/another-project", "feature")
	if err != nil || third.MachineID != first.MachineID || third.ProjectID == first.ProjectID || third.PieceID == first.PieceID {
		t.Fatalf("project isolation: %+v %v", third, err)
	}
	lookedUp, err := LookupIdentity(dir, "/project", "feature")
	if err != nil || lookedUp != first {
		t.Fatalf("lookup: %+v %v", lookedUp, err)
	}
	if _, err := LookupIdentity(dir, "/project", "typo"); err == nil {
		t.Fatal("lookup invented an identity for an unpublished piece")
	}
	info, _ := os.Stat(filepath.Join(dir, "identity.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("identity permissions: %v", info.Mode())
	}
	if err := os.WriteFile(filepath.Join(dir, "identity.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Identity(dir, "/project", "feature"); err == nil {
		t.Fatal("corrupt identity must not silently rotate IDs")
	}
}

func TestExplicitConfigurationAndNoRedirects(t *testing.T) {
	for _, endpoint := range []string{"", "file:///tmp/foo", "http://user:password@example.test", "https://example.test?token=secret"} {
		if _, err := New(endpoint, "secret"); err == nil {
			t.Fatalf("accepted %q", endpoint)
		}
	}
	if _, err := New("http://localhost", ""); err == nil {
		t.Fatal("missing token accepted")
	}
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c, _ := New(source.URL, "secret")
	if _, err := c.List(context.Background()); err == nil || !strings.Contains(err.Error(), "307") {
		t.Fatalf("redirect accepted: %v", err)
	}
	if leaked.Load() != 0 {
		t.Fatal("followed redirect")
	}
}
