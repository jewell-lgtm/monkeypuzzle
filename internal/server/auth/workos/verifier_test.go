package workos

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

type fakeResolver struct {
	byExternal map[string]store.User
}

func (f fakeResolver) GetUserByExternalID(_ context.Context, id string) (store.User, error) {
	u, ok := f.byExternal[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func TestTokenInfoFromClaims(t *testing.T) {
	resolver := fakeResolver{byExternal: map[string]store.User{"wos_42": {ID: 7}}}

	ti, err := tokenInfoFromClaims(context.Background(),
		jwt.MapClaims{"sub": "wos_42", "scope": "mcp:read mcp:write"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if ti.UserID != "7" {
		t.Fatalf("UserID = %q, want 7", ti.UserID)
	}
	if !reflect.DeepEqual(ti.Scopes, []string{"mcp:read", "mcp:write"}) {
		t.Fatalf("scopes = %v", ti.Scopes)
	}
}

func TestTokenInfoFromClaims_Errors(t *testing.T) {
	resolver := fakeResolver{byExternal: map[string]store.User{"known": {ID: 1}}}

	// missing sub
	if _, err := tokenInfoFromClaims(context.Background(), jwt.MapClaims{}, resolver); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("missing sub: got %v", err)
	}
	// unknown user
	if _, err := tokenInfoFromClaims(context.Background(), jwt.MapClaims{"sub": "unknown"}, resolver); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("unknown sub: got %v", err)
	}
}

func TestScopesFromClaims(t *testing.T) {
	if got := scopesFromClaims(jwt.MapClaims{"scp": []any{"a", "b"}}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("scp array: %v", got)
	}
	if got := scopesFromClaims(jwt.MapClaims{"scope": "x y"}); !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Fatalf("scope string: %v", got)
	}
	if got := scopesFromClaims(jwt.MapClaims{}); got != nil {
		t.Fatalf("no scopes: %v", got)
	}
}
