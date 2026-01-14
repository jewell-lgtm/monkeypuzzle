package adapters

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// NewMultiplexer creates a Multiplexer based on the provider name.
// Valid providers: "tmux", "zellij", "none"
func NewMultiplexer(provider string, exec core.Exec) (core.Multiplexer, error) {
	switch provider {
	case "tmux":
		return NewTmuxMultiplexer(exec), nil
	case "zellij":
		return NewZellijMultiplexer(exec), nil
	case "none", "":
		return NewNoopMultiplexer(), nil
	default:
		return nil, fmt.Errorf("unknown multiplexer provider: %s", provider)
	}
}

// IsNoopMultiplexer returns true if the multiplexer is a NoopMultiplexer.
// Useful for callers that need to handle the "none" case specially (e.g., print path).
func IsNoopMultiplexer(m core.Multiplexer) bool {
	_, ok := m.(*NoopMultiplexer)
	return ok
}
