package pr

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/input"
)

// fields defines all input fields - single source of truth for validation + schema
var fields = []input.Field{
	{
		Name:        "title",
		Description: "PR title",
		Required:    false, // Can be derived from issue title
	},
	{
		Name:        "body",
		Description: "PR description",
		Required:    false,
		Default:     "",
	},
	{
		Name:        "base",
		Description: "Base branch to merge into (auto-detects from piece parent if not specified)",
		Required:    false,
		Default:     "", // Empty means auto-detect from piece metadata
	},
}

// Input holds input for PR creation
type Input struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Base  string `json:"base"`
}

// Schema returns the JSON schema with defaults for PR create
func Schema() ([]byte, error) {
	return input.GenerateSchema(fields)
}

// Fields returns field definitions for TUI generation
func Fields() []input.Field {
	return fields
}

// GetDefaults returns default values for all fields.
func GetDefaults() map[string]string {
	return input.GetDefaults(fields)
}

// WithDefaults returns input with defaults applied and whitespace trimmed.
// Note: Base is NOT defaulted here - the handler auto-detects from piece parent metadata.
func WithDefaults(in Input) Input {
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.Base = strings.TrimSpace(in.Base)
	// Base left empty intentionally - handler will auto-detect from piece parent
	return in
}

// ParseJSON parses JSON input into Input struct
func ParseJSON(data []byte) (Input, error) {
	return input.ParseJSON[Input](data)
}

// MaxTitleLength is the maximum allowed PR title length
const MaxTitleLength = 256

// branchNameRegex validates git branch names
// Disallows: spaces, ~, ^, :, \, ?, *, [, .., @{, leading/trailing dots or slashes
var branchNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][-a-zA-Z0-9._/]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

// Validate validates the input (title is optional, derived from issue if not provided)
func Validate(in Input) error {
	if in.Title != "" && len(in.Title) > MaxTitleLength {
		return fmt.Errorf("title too long (max %d chars, got %d)", MaxTitleLength, len(in.Title))
	}
	if in.Base != "" && !isValidBranchName(in.Base) {
		return fmt.Errorf("invalid base branch name: %q", in.Base)
	}
	return nil
}

// isValidBranchName checks if a string is a valid git branch name
func isValidBranchName(name string) bool {
	if name == "" {
		return false
	}
	// Check for invalid patterns
	if strings.Contains(name, "..") ||
		strings.Contains(name, "@{") ||
		strings.HasPrefix(name, "/") ||
		strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".lock") {
		return false
	}
	return branchNameRegex.MatchString(name)
}
