package trackingapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/trackingapi"
	"github.com/jewell-lgtm/monkeypuzzle/internal/trackingclient"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func setup(t *testing.T) (*httptest.Server, *trackingclient.Client, *trackingclient.Client) {
	t.Helper()
	st := store.NewMemoryStore()
	users := map[string]string{}
	for i, token := range []string{"alice", "bob"} {
		uid, err := st.UpsertUser(context.Background(), store.User{ExternalUserID: token, Provider: "github", ForgeUserID: int64(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		users[token] = strconv.FormatInt(uid, 10)
	}
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if uid, ok := users[token]; ok {
			return &auth.TokenInfo{UserID: uid, Expiration: time.Now().Add(time.Hour)}, nil
		}
		return nil, auth.ErrInvalidToken
	}
	srv := httptest.NewServer(trackingapi.NewHandler(st, verifier, "https://example.test/.well-known/oauth-protected-resource"))
	t.Cleanup(srv.Close)
	a, _ := trackingclient.New(srv.URL, "alice")
	b, _ := trackingclient.New(srv.URL, "bob")
	return srv, a, b
}

var key = tracking.Key{MachineID: "machine", ProjectID: "project", PieceID: "piece"}
var snapshot = tracking.Snapshot{Machine: "laptop", Project: "mp", Piece: "feature", State: "working", Note: "implementing"}

func TestRoundTripIdempotencyAndUserIsolation(t *testing.T) {
	_, alice, bob := setup(t)
	ctx := context.Background()
	first, err := alice.Put(ctx, key, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := alice.Put(ctx, key, snapshot)
	if err != nil || first != retry {
		t.Fatalf("retry changed item: %+v, %v", retry, err)
	}
	items, err := bob.List(ctx)
	if err != nil || len(items.Items) != 0 {
		t.Fatalf("cross-user list: %+v, %v", items, err)
	}
	if err := bob.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	items, err = alice.List(ctx)
	if err != nil || len(items.Items) != 1 {
		t.Fatalf("cross-user delete: %+v, %v", items, err)
	}
	other := snapshot
	other.State = "blocked"
	if _, err := bob.Put(ctx, key, other); err != nil {
		t.Fatal(err)
	}
	changed := snapshot
	changed.State = "review"
	changed.Note = ""
	updated, err := alice.Put(ctx, key, changed)
	if err != nil || updated.CreatedAt != first.CreatedAt || updated.State != "review" || updated.Note != "" || !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("bad replacement: %+v, %v", updated, err)
	}
	for range 2 {
		if err := alice.Delete(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	items, err = alice.List(ctx)
	if err != nil || len(items.Items) != 0 {
		t.Fatalf("delete: %+v, %v", items, err)
	}
	items, err = bob.List(ctx)
	if err != nil || len(items.Items) != 1 || items.Items[0].State != "blocked" {
		t.Fatalf("other user changed: %+v, %v", items, err)
	}
}

func TestConcurrentRetriesAndIdentityIsolation(t *testing.T) {
	_, client, _ := setup(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := client.Put(context.Background(), key, snapshot); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	for _, k := range []tracking.Key{{MachineID: "other", ProjectID: key.ProjectID, PieceID: key.PieceID}, {MachineID: key.MachineID, ProjectID: "other", PieceID: key.PieceID}} {
		if _, err := client.Put(context.Background(), k, snapshot); err != nil {
			t.Fatal(err)
		}
	}
	items, err := client.List(context.Background())
	if err != nil || len(items.Items) != 3 {
		t.Fatalf("identity collision: %+v, %v", items, err)
	}
}

func TestRejectsUnauthenticatedMalformedAndOversizedRequests(t *testing.T) {
	srv, client, _ := setup(t)
	good, _ := json.Marshal(snapshot)
	tests := []struct {
		method, path, token, contentType, body string
		want                                   int
	}{
		{"GET", tracking.BasePath, "", "", "", 401},
		{"PUT", key.Path(), "bad", "application/json", string(good), 401},
		{"DELETE", key.Path(), "bad", "", "", 401},
		{"PUT", key.Path(), "alice", "text/plain", string(good), 415},
		{"PUT", key.Path(), "alice", "application/json", "null", 400},
		{"PUT", key.Path(), "alice", "application/json", "{", 400},
		{"PUT", key.Path(), "alice", "application/json", string(good) + " {}", 400},
		{"PUT", key.Path(), "alice", "application/json", `{"user_id":2}`, 400},
		{"PUT", key.Path(), "alice", "application/json", strings.Replace(string(good), "working", "invalid", 1), 400},
		{"PUT", key.Path(), "alice", "application/json", string(good) + strings.Repeat(" ", tracking.MaxBody), 413},
		{"GET", tracking.BasePath + "?user_id=2", "alice", "", "", 400},
	}
	for _, tt := range tests {
		t.Run(tt.method+"/"+strconv.Itoa(tt.want)+"/"+tt.body[:min(len(tt.body), 30)], func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, srv.URL+tt.path, strings.NewReader(tt.body))
			if tt.token != "" {
				req.Header.Set("Authorization", "Bearer "+tt.token)
			}
			req.Header.Set("Content-Type", tt.contentType)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != tt.want {
				t.Fatalf("got %d want %d", res.StatusCode, tt.want)
			}
		})
	}
	items, err := client.List(context.Background())
	if err != nil || len(items.Items) != 0 {
		t.Fatalf("invalid requests wrote data: %+v %v", items, err)
	}
}
