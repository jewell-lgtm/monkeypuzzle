package workos

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/modelcontextprotocol/go-sdk/auth"
)

func TestRegistryTokenVerifier(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
	}))
	defer jwks.Close()
	resolver := fakeResolver{byExternal: map[string]store.User{"user_test": {ID: 42}}}
	fallback := func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return nil, fmt.Errorf("%w: rejected", auth.ErrInvalidToken)
	}
	verify, err := NewRegistryTokenVerifier(jwks.URL, "client_test", resolver, fallback)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, claim string
		value       any
		valid       bool
	}{
		{name: "valid", valid: true},
		{name: "wrong issuer", claim: "iss", value: "https://other.example"},
		{name: "wrong application", claim: "client_id", value: "client_other"},
		{name: "missing session", claim: "sid", value: ""},
		{name: "id token audience", claim: "aud", value: "client_test"},
		{name: "unknown user", claim: "sub", value: "user_unknown"},
		{name: "expired", claim: "exp", value: time.Now().Add(-time.Hour).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{"iss": "https://api.workos.com/user_management/client_test", "client_id": "client_test", "sub": "user_test", "sid": "session_test", "exp": time.Now().Add(time.Hour).Unix()}
			if tc.claim != "" {
				claims[tc.claim] = tc.value
			}
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			token.Header["kid"] = "test"
			bearer, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			info, err := verify(context.Background(), bearer, nil)
			if tc.valid {
				if err != nil || info.UserID != "42" {
					t.Fatalf("valid token: %v %v", info, err)
				}
			} else if err == nil {
				t.Fatal("invalid token accepted")
			}
		})
	}
}
