package workos

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

// UserResolver maps a WorkOS user id (`sub`) to the local user. The MCP resource
// server uses it to bind a validated WorkOS token to a local user.
type UserResolver interface {
	GetUserByExternalID(ctx context.Context, externalUserID string) (store.User, error)
}

// NewTokenVerifier returns an auth.TokenVerifier (for the MCP resource server)
// that validates a WorkOS access-token JWT against the WorkOS JWKS — checking
// signature, issuer, expiry, and (if non-empty) audience — then maps its subject
// to a local user id. jwksURL is typically <authkit-domain>/oauth2/jwks.
func NewTokenVerifier(jwksURL, issuer, audience string, resolver UserResolver) (auth.TokenVerifier, error) {
	kf, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("workos: load jwks %q: %w", jwksURL, err)
	}
	opts := []jwt.ParserOption{jwt.WithExpirationRequired()}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		opts = append(opts, jwt.WithAudience(audience))
	}
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		parsed, err := jwt.Parse(token, kf.Keyfunc, opts...)
		if err != nil || !parsed.Valid {
			return nil, fmt.Errorf("%w: %v", auth.ErrInvalidToken, err)
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			return nil, fmt.Errorf("%w: unexpected claims type", auth.ErrInvalidToken)
		}
		return tokenInfoFromClaims(ctx, claims, resolver)
	}, nil
}

// tokenInfoFromClaims maps validated JWT claims to an auth.TokenInfo. Split out
// from the JWKS-bound verifier so the mapping/error logic is unit-testable.
func tokenInfoFromClaims(ctx context.Context, claims jwt.MapClaims, resolver UserResolver) (*auth.TokenInfo, error) {
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("%w: missing sub", auth.ErrInvalidToken)
	}
	user, err := resolver.GetUserByExternalID(ctx, sub)
	if err != nil {
		return nil, fmt.Errorf("%w: no local user for sub %q", auth.ErrInvalidToken, sub)
	}
	ti := &auth.TokenInfo{
		UserID: strconv.FormatInt(user.ID, 10),
		Scopes: scopesFromClaims(claims),
	}
	if exp, err := claims.GetExpirationTime(); err == nil && exp != nil {
		ti.Expiration = exp.Time
	}
	return ti, nil
}

// scopesFromClaims reads scopes from "scp" (array) or "scope" (space-delimited).
func scopesFromClaims(claims jwt.MapClaims) []string {
	if raw, ok := claims["scp"].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, s := range raw {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	if s, ok := claims["scope"].(string); ok && s != "" {
		return strings.Fields(s)
	}
	return nil
}
