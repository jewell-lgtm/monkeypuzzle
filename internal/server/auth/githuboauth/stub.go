package githuboauth

import "context"

// StubClient is a deterministic Client for tests: it returns a fixed token from
// Exchange and a fixed profile from FetchProfile.
type StubClient struct {
	Token   string
	Profile Profile
}

// NewStubClient returns a stub that exchanges any code for token and reports profile.
func NewStubClient(token string, profile Profile) *StubClient {
	return &StubClient{Token: token, Profile: profile}
}

func (s *StubClient) AuthCodeURL(state string) string {
	return "https://github.test/login/oauth/authorize?state=" + state
}

func (s *StubClient) Exchange(context.Context, string) (string, error) {
	return s.Token, nil
}

func (s *StubClient) FetchProfile(context.Context, string) (Profile, error) {
	return s.Profile, nil
}

var _ Client = (*StubClient)(nil)
