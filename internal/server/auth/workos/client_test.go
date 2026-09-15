package workos

import (
	"github.com/workos/workos-go/v4/pkg/usermanagement"
	"testing"
)

func TestRegistryIdentityWithoutGitHubToken(t *testing.T) {
	id := registryIdentity(usermanagement.AuthenticateResponse{User: usermanagement.User{ID: "user_123", FirstName: "Registry", LastName: "Developer"}})
	if id.ProviderUserID != "user_123" || id.DisplayName != "Registry Developer" || id.Token != "" {
		t.Fatalf("unexpected identity: %+v", id)
	}
}
