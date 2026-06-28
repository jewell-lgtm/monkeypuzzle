package workos

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
)

// StubClient is a deterministic identity.Provider for tests.
type StubClient struct {
	Identity identity.Identity
}

// NewStubClient returns a stub that authenticates any code to the given identity
// (provider "github").
func NewStubClient(providerUserID, token string) *StubClient {
	return &StubClient{Identity: identity.Identity{
		ProviderUserID: providerUserID,
		Provider:       "github",
		Token:          token,
	}}
}

func (s *StubClient) AuthorizationURL(state string) string {
	return "https://authkit.test/authorize?state=" + state
}

func (s *StubClient) Authenticate(context.Context, string) (identity.Identity, error) {
	return s.Identity, nil
}

var _ identity.Provider = (*StubClient)(nil)
