// Package mprunner shells out to the `mp` CLI to reconstruct stacked-PR
// forests, letting the server reuse the CLI's exact stackgraph logic (zero
// drift) instead of rebuilding stacks in-process. It is optional: the server
// falls back to its in-process Go path whenever a run fails, so the page never
// breaks. The forge token is supplied to mp only via the environment and is
// never logged.
package mprunner

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// StackGraphInput is everything `mp stack graph` needs for one repo. Token is
// the forge access token; implementations pass it to mp only via env.
type StackGraphInput struct {
	RepoSlug      string // owner/name
	DefaultBranch string // trunk branch (optional; auto-detected if empty)
	Provider      string // "github" (default) or "gitlab"
	Token         string // forge access token (never logged)
}

// MpRunner runs `mp stack graph` for a repo and returns the reconstructed
// forest. Implementations MUST never log or otherwise expose the token.
type MpRunner interface {
	StackGraph(ctx context.Context, in StackGraphInput) ([]stackgraph.Stack, error)
}
