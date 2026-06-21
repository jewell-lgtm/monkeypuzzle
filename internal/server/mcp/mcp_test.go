package mcp

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

// Constructing the server registers all tools, which infers their JSON schemas
// (including the recursive stack tree). This guards against schema-inference
// panics without needing the full integration harness.
func TestNewServer_RegistersToolsWithoutPanic(t *testing.T) {
	svc := service.New(store.NewMemoryStore(), nil)
	if NewServer(svc) == nil {
		t.Fatal("NewServer returned nil")
	}
}
