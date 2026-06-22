// Package stack implements git-town-style stacked-branch management for
// monkeypuzzle: whole-stack sync, GitHub-as-source-of-truth lineage, and
// stack-native append/prepend — all non-interactive with plain-English fallbacks.
package stack

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Sync strategies.
const (
	StrategyMerge  = "merge"  // merge each parent into its child (safe, no force-push)
	StrategyRebase = "rebase" // rebase each child onto its parent (linear, needs force-push)
)

// StatusInput holds input for `mp stack status`.
type StatusInput struct {
	MainBranch string `json:"main_branch,omitempty"`
	// FromGitHub rewrites local lineage (piece-metadata.json parents) from open PR
	// bases — the multi-machine rebuild path.
	FromGitHub bool `json:"from_github,omitempty"`
	// ApplyBases pushes local lineage to GitHub by editing each open PR's base.
	ApplyBases bool `json:"apply_bases,omitempty"`
}

// StatusSchema returns the JSON schema for stack status input.
func StatusSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"main_branch": "main",
		"from_github": false,
		"apply_bases": false,
	}, "", "  ")
}

// ParseStatusJSON parses JSON input into StatusInput.
func ParseStatusJSON(data []byte) (StatusInput, error) {
	var in StatusInput
	if err := json.Unmarshal(data, &in); err != nil {
		return StatusInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// WithStatusDefaults applies defaults to status input.
func WithStatusDefaults(in StatusInput) StatusInput {
	main := strings.TrimSpace(in.MainBranch)
	if main == "" {
		main = "main"
	}
	return StatusInput{MainBranch: main, FromGitHub: in.FromGitHub, ApplyBases: in.ApplyBases}
}

// SyncInput holds input for `mp stack sync`.
type SyncInput struct {
	MainBranch string `json:"main_branch,omitempty"`
	// From is the upstream ref the local main branch is updated from before the
	// sync propagates down the stack, e.g. "origin/main". Defaults to
	// "origin/<main_branch>". The CLI prompts for it interactively when a human
	// runs sync without supplying it. A ref whose remote isn't configured (e.g.
	// "origin/main" in a local-only repo) makes the main-update a no-op.
	From     string `json:"from,omitempty"`
	Strategy string `json:"strategy,omitempty"` // "merge" (default) | "rebase"
	Push     bool   `json:"push,omitempty"`
	// Stack limits the sync to the current piece's stack (ancestors + self +
	// descendants) instead of every piece in the repo. Requires running from a
	// piece worktree.
	Stack bool `json:"stack,omitempty"`
	// DryRun previews which pieces would be synced without touching any branch.
	// Sync is dry-run by default; Apply opts into mutating the stack.
	DryRun bool `json:"dry_run,omitempty"`
	// Apply performs the sync for real (the opt-in to mutate). Without it a
	// non-interactive invocation only previews.
	Apply bool `json:"apply,omitempty"`
}

// SyncSchema returns the JSON schema for stack sync input.
func SyncSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"main_branch": "main",
		"from":        "origin/main",
		"strategy":    StrategyMerge,
		"push":        false,
		"stack":       false,
		"dry_run":     false,
		"apply":       false,
	}, "", "  ")
}

// ParseSyncJSON parses JSON input into SyncInput.
func ParseSyncJSON(data []byte) (SyncInput, error) {
	var in SyncInput
	if err := json.Unmarshal(data, &in); err != nil {
		return SyncInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// ValidateSyncInput checks the strategy is recognized.
func ValidateSyncInput(in SyncInput) error {
	switch in.Strategy {
	case "", StrategyMerge, StrategyRebase:
		return nil
	default:
		return fmt.Errorf("invalid strategy %q (valid: %s, %s)", in.Strategy, StrategyMerge, StrategyRebase)
	}
}

// WithSyncDefaults applies defaults to sync input.
func WithSyncDefaults(in SyncInput) SyncInput {
	main := strings.TrimSpace(in.MainBranch)
	if main == "" {
		main = "main"
	}
	strategy := strings.TrimSpace(in.Strategy)
	if strategy == "" {
		strategy = StrategyMerge
	}
	from := strings.TrimSpace(in.From)
	if from == "" {
		from = "origin/" + main
	}
	return SyncInput{MainBranch: main, From: from, Strategy: strategy, Push: in.Push, Stack: in.Stack, DryRun: in.DryRun, Apply: in.Apply}
}

// AppendInput holds input for `mp stack append` (create a child of the current piece).
type AppendInput struct {
	Name   string `json:"name,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

// PrependInput holds input for `mp stack prepend` (insert a piece between the
// current piece and its parent).
type PrependInput struct {
	Name   string `json:"name,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

// AppendSchema returns the JSON schema for stack append/prepend input.
func AppendSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"name":   "",
		"prompt": "",
	}, "", "  ")
}

// ParseAppendJSON parses JSON input into AppendInput.
func ParseAppendJSON(data []byte) (AppendInput, error) {
	var in AppendInput
	if err := json.Unmarshal(data, &in); err != nil {
		return AppendInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// ParsePrependJSON parses JSON input into PrependInput.
func ParsePrependJSON(data []byte) (PrependInput, error) {
	var in PrependInput
	if err := json.Unmarshal(data, &in); err != nil {
		return PrependInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// ValidateAppendInput requires a name or prompt.
func ValidateAppendInput(in AppendInput) error {
	if strings.TrimSpace(in.Name) == "" && strings.TrimSpace(in.Prompt) == "" {
		return fmt.Errorf("must specify name or prompt")
	}
	return nil
}

// ValidatePrependInput requires a name or prompt.
func ValidatePrependInput(in PrependInput) error {
	return ValidateAppendInput(AppendInput(in))
}

// SetParentInput holds input for `mp stack set-parent` (re-parent a piece).
type SetParentInput struct {
	Piece  string `json:"piece,omitempty"` // piece to re-parent; defaults to the current piece
	Parent string `json:"parent"`          // new parent piece name, or "main"
}

// SetParentSchema returns the JSON schema for stack set-parent input.
func SetParentSchema() ([]byte, error) {
	return json.MarshalIndent(map[string]any{
		"piece":  "",
		"parent": "",
	}, "", "  ")
}

// ParseSetParentJSON parses JSON input into SetParentInput.
func ParseSetParentJSON(data []byte) (SetParentInput, error) {
	var in SetParentInput
	if err := json.Unmarshal(data, &in); err != nil {
		return SetParentInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return in, nil
}

// ValidateSetParentInput requires a target parent.
func ValidateSetParentInput(in SetParentInput) error {
	if strings.TrimSpace(in.Parent) == "" {
		return fmt.Errorf("must specify parent (a piece name, or \"main\")")
	}
	return nil
}
