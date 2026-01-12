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

// NewPieceInput holds input for the piece new command.
// Either IssuePath or Name must be provided (mutually exclusive).
type NewPieceInput struct {
	IssuePath        string `json:"issue_path,omitempty"`
	Name             string `json:"name,omitempty"`
	Parent           string `json:"parent,omitempty"` // Parent piece name, defaults to "main"
	SkipSwitch       bool   `json:"skip_switch,omitempty"`
	OverwriteSession bool   `json:"overwrite_session,omitempty"`
}

// NewPieceSchema returns the JSON schema for piece new input.
func NewPieceSchema() ([]byte, error) {
	schema := map[string]any{
		"issue_path":        "",
		"name":              "",
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
// One of IssuePath or Name must be provided, but not both.
func ValidateNewPieceInput(input NewPieceInput) error {
	hasIssue := strings.TrimSpace(input.IssuePath) != ""
	hasName := strings.TrimSpace(input.Name) != ""

	if hasIssue && hasName {
		return fmt.Errorf("cannot specify both issue_path and name")
	}
	if !hasIssue && !hasName {
		return fmt.Errorf("must specify either issue_path or name")
	}
	return nil
}

// WithNewPieceDefaults returns input with whitespace trimmed and defaults applied.
func WithNewPieceDefaults(input NewPieceInput) NewPieceInput {
	parent := strings.TrimSpace(input.Parent)
	if parent == "" {
		parent = "main"
	}
	return NewPieceInput{
		IssuePath:        strings.TrimSpace(input.IssuePath),
		Name:             strings.TrimSpace(input.Name),
		Parent:           parent,
		SkipSwitch:       input.SkipSwitch,
		OverwriteSession: input.OverwriteSession,
	}
}

// SwitchInput holds input for the piece switch command.
type SwitchInput struct {
	Name string `json:"name"`
}

// SwitchSchema returns the JSON schema for piece switch input.
func SwitchSchema() ([]byte, error) {
	schema := map[string]any{
		"name": "",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseSwitchJSON parses JSON input into SwitchInput.
func ParseSwitchJSON(data []byte) (SwitchInput, error) {
	var input SwitchInput
	if err := json.Unmarshal(data, &input); err != nil {
		return SwitchInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidateSwitchInput validates the switch input.
func ValidateSwitchInput(input SwitchInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// WithSwitchDefaults returns input with whitespace trimmed.
func WithSwitchDefaults(input SwitchInput) SwitchInput {
	return SwitchInput{
		Name: strings.TrimSpace(input.Name),
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

// MergeInput holds input for the piece merge command.
type MergeInput struct {
	MainBranch string `json:"main_branch,omitempty"`
	Force      bool   `json:"force,omitempty"` // Force merge even if piece has children
}

// MergeResult contains the result of a merge operation.
type MergeResult struct {
	PieceName    string `json:"piece_name"`
	PieceBranch  string `json:"piece_branch"`
	TargetBranch string `json:"target_branch"` // The branch merged into (parent or main)
	Status       string `json:"status"`        // "merged"
}

// MergeSchema returns the JSON schema for piece merge input.
func MergeSchema() ([]byte, error) {
	schema := map[string]any{
		"main_branch": "main",
		"force":       false,
	}
	return json.MarshalIndent(schema, "", "  ")
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
	return MergeInput{MainBranch: mainBranch, Force: input.Force}
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
