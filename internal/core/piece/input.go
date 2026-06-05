package piece

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PieceInfo contains information about a created piece worktree.
// It includes the piece name, worktree path, and associated tmux session name.
type PieceInfo struct {
	// Name is the unique identifier for this piece (e.g., "piece-20250127-143022")
	Name string `json:"name"`
	// WorktreePath is the absolute path to the git worktree directory
	WorktreePath string `json:"worktree_path"`
	// SessionName is the name of the tmux session created for this piece
	SessionName string `json:"session_name"`
}

// PieceStatus contains information about the current piece status.
// It indicates whether the current directory is in a piece worktree or the main repository.
type PieceStatus struct {
	// InPiece is true if the current directory is within a piece worktree
	InPiece bool `json:"in_piece"`
	// PieceName is the name of the piece, only set when InPiece is true
	PieceName string `json:"piece_name,omitempty"`
	// WorktreePath is the path to the worktree, only set when InPiece is true
	WorktreePath string `json:"worktree_path,omitempty"`
	// RepoRoot is the path to the main repository root
	RepoRoot string `json:"repo_root,omitempty"`
}

// PieceHierarchyStatus extends PieceStatus with hierarchy information.
// It includes parent/child relationships, stack depth, and merge readiness.
type PieceHierarchyStatus struct {
	PieceStatus
	// Parent is the parent piece name, or "main" if this is a root piece
	Parent string `json:"parent,omitempty"`
	// Children is the list of child piece names
	Children []string `json:"children,omitempty"`
	// StackDepth is the depth of this piece in the stack (1 = direct child of main, 2 = grandchild, etc.)
	StackDepth int `json:"stack_depth,omitempty"`
	// CanMerge is true if this piece can be merged (no children, or all children are merged)
	CanMerge bool `json:"can_merge,omitempty"`
}

// PieceListItem represents a piece available for switching.
type PieceListItem struct {
	Name         string    `json:"name"`
	WorktreePath string    `json:"worktree_path"`
	SessionName  string    `json:"session_name"`
	HasSession   bool      `json:"has_session"`
	ModTime      time.Time `json:"mod_time"`
	Parent       string    `json:"parent,omitempty"` // Parent piece name, or "main" for root pieces
}

// SwitchResult contains the result of a switch operation.
type SwitchResult struct {
	Piece  PieceListItem `json:"piece"`
	Method string        `json:"method"` // "tmux-switch", "tmux-attach", "path"
}

// NewPieceInput holds input for the piece create command.
// Must provide one of: Issue, Name, or Prompt.
// Issue and Prompt are mutually exclusive. Name can coexist with Prompt.
type NewPieceInput struct {
	Issue            IssueRef `json:"issue,omitempty"`
	Name             string   `json:"name,omitempty"`
	Prompt           string   `json:"prompt,omitempty"`
	Parent           string   `json:"parent,omitempty"` // Parent piece name, defaults to "main"
	SkipSwitch       bool     `json:"skip_switch,omitempty"`
	OverwriteSession bool     `json:"overwrite_session,omitempty"`
}

// NewPieceSchema returns the JSON schema for piece create input.
func NewPieceSchema() ([]byte, error) {
	schema := map[string]any{
		"issue": map[string]any{
			"provider": "",
			"id":       "",
			"number":   "",
			"title":    "",
		},
		"name":              "",
		"prompt":            "",
		"parent":            "main",
		"skip_switch":       false,
		"overwrite_session": false,
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseNewPieceJSON parses JSON input into NewPieceInput.
func ParseNewPieceJSON(data []byte) (NewPieceInput, error) {
	var input NewPieceInput
	if err := json.Unmarshal(data, &input); err != nil {
		return NewPieceInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidateNewPieceInput validates the input.
// Must provide one of: Issue, Name, or Prompt.
// Issue and Prompt are mutually exclusive. Name can coexist with Prompt.
func ValidateNewPieceInput(input NewPieceInput) error {
	hasIssue := !input.Issue.IsEmpty()
	hasName := strings.TrimSpace(input.Name) != ""
	hasPrompt := strings.TrimSpace(input.Prompt) != ""

	if hasIssue && hasPrompt {
		return fmt.Errorf("cannot specify both issue and prompt")
	}
	if hasIssue && hasName {
		return fmt.Errorf("cannot specify both issue and name")
	}
	if !hasIssue && !hasName && !hasPrompt {
		return fmt.Errorf("must specify issue, name, or prompt")
	}
	return nil
}

// WithNewPieceDefaults returns input with whitespace trimmed and defaults applied.
// When Prompt is set and Name is empty, derives name from prompt.
func WithNewPieceDefaults(input NewPieceInput) NewPieceInput {
	parent := strings.TrimSpace(input.Parent)
	if parent == "" {
		parent = "main"
	}
	prompt := strings.TrimSpace(input.Prompt)
	name := strings.TrimSpace(input.Name)

	// Derive name from prompt if not explicitly set
	if prompt != "" && name == "" {
		name = SanitizePieceName(prompt)
	}

	return NewPieceInput{
		Issue:            input.Issue,
		Name:             name,
		Prompt:           prompt,
		Parent:           parent,
		SkipSwitch:       input.SkipSwitch,
		OverwriteSession: input.OverwriteSession,
	}
}

// AbandonInput holds input for the piece abandon command.
type AbandonInput struct {
	Name         string `json:"name"`
	Force        bool   `json:"force,omitempty"`
	DeleteBranch bool   `json:"delete_branch,omitempty"`
}

// AbandonSchema returns the JSON schema for piece abandon input.
func AbandonSchema() ([]byte, error) {
	schema := map[string]any{
		"name":          "",
		"force":         false,
		"delete_branch": false,
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseAbandonJSON parses JSON input into AbandonInput.
func ParseAbandonJSON(data []byte) (AbandonInput, error) {
	var input AbandonInput
	if err := json.Unmarshal(data, &input); err != nil {
		return AbandonInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidateAbandonInput validates the abandon input.
func ValidateAbandonInput(input AbandonInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// WithAbandonDefaults returns input with whitespace trimmed.
func WithAbandonDefaults(input AbandonInput) AbandonInput {
	return AbandonInput{
		Name:         strings.TrimSpace(input.Name),
		Force:        input.Force,
		DeleteBranch: input.DeleteBranch,
	}
}

// UpdateInput holds input for the piece update command.
type UpdateInput struct {
	MainBranch string `json:"main_branch,omitempty"`
}

// UpdateResult contains the result of an update operation.
type UpdateResult struct {
	PieceName  string `json:"piece_name"`
	MainBranch string `json:"main_branch"`
	Status     string `json:"status"` // "updated", "already-up-to-date"
}

// UpdateSchema returns the JSON schema for piece update input.
func UpdateSchema() ([]byte, error) {
	schema := map[string]any{
		"main_branch": "main",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseUpdateJSON parses JSON input into UpdateInput.
func ParseUpdateJSON(data []byte) (UpdateInput, error) {
	var input UpdateInput
	if err := json.Unmarshal(data, &input); err != nil {
		return UpdateInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// WithUpdateDefaults returns input with defaults applied.
func WithUpdateDefaults(input UpdateInput) UpdateInput {
	mainBranch := strings.TrimSpace(input.MainBranch)
	if mainBranch == "" {
		mainBranch = "main"
	}
	return UpdateInput{MainBranch: mainBranch}
}

// Reparent strategies for merging a piece that has child pieces.
const (
	ReparentRebase = "rebase" // rebase descendants onto the merge target (rewrites history)
	ReparentMerge  = "merge"  // merge the target into direct children (no history rewrite)
)

// MergeInput holds input for the piece merge command.
type MergeInput struct {
	MainBranch string `json:"main_branch,omitempty"`
	Force      bool   `json:"force,omitempty"` // Force merge even if piece has children (children are NOT re-homed)
	// ReparentChildren, when true, allows merging a piece that has child pieces:
	// each descendant is re-homed onto the merge target and its parent metadata
	// re-pointed accordingly.
	ReparentChildren bool `json:"reparent_children,omitempty"`
	// ReparentStrategy is "rebase" (default) or "merge"; only used when
	// ReparentChildren is true.
	ReparentStrategy string `json:"reparent_strategy,omitempty"`
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	PieceName          string   `json:"piece_name"`
	PieceBranch        string   `json:"piece_branch"`
	TargetBranch       string   `json:"target_branch"` // The branch merged into (parent or main)
	Status             string   `json:"status"`        // "merged"
	ReparentedChildren []string `json:"reparented_children,omitempty"`
}

// MergeSchema returns the JSON schema for piece merge input.
func MergeSchema() ([]byte, error) {
	schema := map[string]any{
		"main_branch":       "main",
		"force":             false,
		"reparent_children": false,
		"reparent_strategy": ReparentRebase,
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ValidateMergeInput checks the reparent strategy is recognised.
func ValidateMergeInput(input MergeInput) error {
	switch input.ReparentStrategy {
	case "", ReparentRebase, ReparentMerge:
		return nil
	default:
		return fmt.Errorf("invalid reparent strategy %q (valid: %s, %s)", input.ReparentStrategy, ReparentRebase, ReparentMerge)
	}
}

// ParseMergeJSON parses JSON input into MergeInput.
func ParseMergeJSON(data []byte) (MergeInput, error) {
	var input MergeInput
	if err := json.Unmarshal(data, &input); err != nil {
		return MergeInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// WithMergeDefaults returns input with defaults applied.
func WithMergeDefaults(input MergeInput) MergeInput {
	mainBranch := strings.TrimSpace(input.MainBranch)
	if mainBranch == "" {
		mainBranch = "main"
	}
	strategy := strings.TrimSpace(input.ReparentStrategy)
	if input.ReparentChildren && strategy == "" {
		strategy = ReparentRebase
	}
	return MergeInput{
		MainBranch:       mainBranch,
		Force:            input.Force,
		ReparentChildren: input.ReparentChildren,
		ReparentStrategy: strategy,
	}
}

// CleanupInput holds input for the piece cleanup command.
type CleanupInput struct {
	MainBranch string `json:"main_branch,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

// CleanupSchema returns the JSON schema for piece cleanup input.
func CleanupSchema() ([]byte, error) {
	schema := map[string]any{
		"main_branch": "main",
		"dry_run":     false,
		"force":       false,
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseCleanupJSON parses JSON input into CleanupInput.
func ParseCleanupJSON(data []byte) (CleanupInput, error) {
	var input CleanupInput
	if err := json.Unmarshal(data, &input); err != nil {
		return CleanupInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// WithCleanupDefaults returns input with defaults applied.
func WithCleanupDefaults(input CleanupInput) CleanupInput {
	mainBranch := strings.TrimSpace(input.MainBranch)
	if mainBranch == "" {
		mainBranch = "main"
	}
	return CleanupInput{
		MainBranch: mainBranch,
		DryRun:     input.DryRun,
		Force:      input.Force,
	}
}

// FlattenInput holds input for the flatten command.
type FlattenInput struct {
	Force          bool `json:"force,omitempty"`
	DeleteBranches bool `json:"delete_branches,omitempty"`
	DryRun         bool `json:"dry_run,omitempty"`
}

// FlattenSchema returns the JSON schema for flatten input.
func FlattenSchema() ([]byte, error) {
	schema := map[string]any{
		"force":           false,
		"delete_branches": false,
		"dry_run":         false,
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseFlattenJSON parses JSON input into FlattenInput.
func ParseFlattenJSON(data []byte) (FlattenInput, error) {
	var input FlattenInput
	if err := json.Unmarshal(data, &input); err != nil {
		return FlattenInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// DoneInput holds input for the piece done command.
type DoneInput struct {
	MainBranch string `json:"main_branch,omitempty"`
}

// DoneResult contains the result of a done operation.
type DoneResult struct {
	PieceName    string `json:"piece_name"`
	WorktreePath string `json:"worktree_path"`
	MainPath     string `json:"main_path"`
	Cleaned      bool   `json:"cleaned"`
}

// DoneSchema returns the JSON schema for piece done input.
func DoneSchema() ([]byte, error) {
	schema := map[string]any{
		"main_branch": "main",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseDoneJSON parses JSON input into DoneInput.
func ParseDoneJSON(data []byte) (DoneInput, error) {
	var input DoneInput
	if err := json.Unmarshal(data, &input); err != nil {
		return DoneInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// WithDoneDefaults returns input with defaults applied.
func WithDoneDefaults(input DoneInput) DoneInput {
	mainBranch := strings.TrimSpace(input.MainBranch)
	if mainBranch == "" {
		mainBranch = "main"
	}
	return DoneInput{MainBranch: mainBranch}
}

// AdoptPieceInput holds input for the piece adopt command.
// Converts an existing git branch to a piece.
type AdoptPieceInput struct {
	Branch   string `json:"branch,omitempty"`    // Branch to adopt (defaults to current branch)
	Name     string `json:"name,omitempty"`      // Override piece name (defaults to branch name)
	Parent   string `json:"parent,omitempty"`    // Parent piece name (defaults to "main")
	RepoRoot string `json:"repo_root,omitempty"` // Explicit repo root (defaults to resolving from cwd)
}

// AdoptPieceSchema returns the JSON schema for piece adopt input.
func AdoptPieceSchema() ([]byte, error) {
	schema := map[string]any{
		"branch":    "",
		"name":      "",
		"parent":    "main",
		"repo_root": "",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseAdoptPieceJSON parses JSON input into AdoptPieceInput.
func ParseAdoptPieceJSON(data []byte) (AdoptPieceInput, error) {
	var input AdoptPieceInput
	if err := json.Unmarshal(data, &input); err != nil {
		return AdoptPieceInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// WithAdoptPieceDefaults returns input with defaults applied.
func WithAdoptPieceDefaults(input AdoptPieceInput) AdoptPieceInput {
	parent := strings.TrimSpace(input.Parent)
	if parent == "" {
		parent = "main"
	}
	return AdoptPieceInput{
		Branch:   strings.TrimSpace(input.Branch),
		Name:     strings.TrimSpace(input.Name),
		Parent:   parent,
		RepoRoot: strings.TrimSpace(input.RepoRoot),
	}
}
