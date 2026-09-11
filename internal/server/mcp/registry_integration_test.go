//go:build integration

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/tracking"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestMCP_PrivateRegistryWithoutForge(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	alice, _ := st.UpsertUser(ctx, store.User{ExternalUserID: "alice", ForgeUserID: 1})
	bob, _ := st.UpsertUser(ctx, store.User{ExternalUserID: "bob", ForgeUserID: 2})
	key := tracking.Key{MachineID: "machine", ProjectID: "project", PieceID: "piece"}
	for _, uid := range []int64{alice, bob} {
		if _, err := st.PutTrackedItem(ctx, uid, key, tracking.Snapshot{Machine: "laptop", Project: "project", Piece: strconv.FormatInt(uid, 10), State: "working"}); err != nil {
			t.Fatal(err)
		}
	}
	verifier := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		if token != "alice" {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: strconv.FormatInt(alice, 10), Expiration: time.Now().Add(time.Hour)}, nil
	}
	srv := httptest.NewServer(NewHTTPHandler(NewServer(service.New(st, nil, nil, nil, false)), verifier, "https://example.test/metadata"))
	defer srv.Close()
	sess := connect(t, ctx, srv.URL, &http.Client{Transport: bearerRT{token: "alice", base: http.DefaultTransport}})
	defer sess.Close()
	tools, err := sess.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "list_pieces" {
		t.Fatalf("registry tools: %+v", tools.Tools)
	}
	var out tracking.List
	callTool(t, ctx, sess, "list_pieces", map[string]any{}, &out)
	if len(out.Items) != 1 || out.Items[0].Piece != strconv.FormatInt(alice, 10) {
		t.Fatalf("private pieces: %+v", out)
	}
}
