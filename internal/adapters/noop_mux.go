package adapters

import (
	"context"
)

// NoopMultiplexer implements core.Multiplexer as a no-op.
// Use when no terminal multiplexer is configured.
type NoopMultiplexer struct{}

// NewNoopMultiplexer creates a NoopMultiplexer.
func NewNoopMultiplexer() *NoopMultiplexer {
	return &NoopMultiplexer{}
}

// SwitchTo is a no-op. Callers should handle the "none" case by printing the path.
func (n *NoopMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	return nil
}

// Kill is a no-op.
func (n *NoopMultiplexer) Kill(ctx context.Context, sessionName string) error {
	return nil
}

// Exists always returns false (no sessions managed).
func (n *NoopMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	return false
}

// InSession always returns false.
func (n *NoopMultiplexer) InSession() bool {
	return false
}

// IsInstalled always returns true (always "available").
func (n *NoopMultiplexer) IsInstalled(ctx context.Context) bool {
	return true
}

// Name returns "none".
func (n *NoopMultiplexer) Name() string {
	return "none"
}
