package init

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Field defines a single input field with validation rules
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	ValidValues []string `json:"valid_values,omitempty"`
}

// fields defines all input fields - single source of truth for validation + schema
var fields = []Field{
	{
		Name:        "name",
		Description: "Project name",
		Required:    true,
		Default:     "", // set dynamically from directory name
	},
	{
		Name:        "issue_provider",
		Description: "How issues/features are managed",
		Required:    true,
		Default:     "markdown",
		ValidValues: []string{"markdown"},
	},
	{
		Name:        "pr_provider",
		Description: "How PRs are managed",
		Required:    true,
		Default:     "github",
		ValidValues: []string{"github"},
	},
}

// Input holds validated input for the init command
type Input struct {
	Name          string `json:"name"`
	IssueProvider string `json:"issue_provider"`
	PRProvider    string `json:"pr_provider"`
}

// Schema returns the JSON schema with defaults for the init command
func Schema(workDir string) ([]byte, error) {
	defaultName := filepath.Base(workDir)

	schema := map[string]any{}
	for _, f := range fields {
		def := f.Default
		if f.Name == "name" && def == "" {
			def = defaultName
		}
		schema[f.Name] = def
	}

	return json.MarshalIndent(schema, "", "  ")
}

// Fields returns field definitions for documentation/TUI generation
func Fields() []Field {
	return fields
}

// Validate validates input and returns errors for invalid fields
func Validate(input Input) error {
	var errs []string

	for _, f := range fields {
		val := getFieldValue(input, f.Name)
		// Trim whitespace and check for empty strings
		val = strings.TrimSpace(val)

		if f.Required && val == "" {
			errs = append(errs, fmt.Sprintf("%s is required", f.Name))
			continue
		}

		// Special validation for project name - check for filesystem-unsafe characters
		if f.Name == "name" && val != "" {
			if sanitized := SanitizeProjectName(val); sanitized != val {
				errs = append(errs, fmt.Sprintf("%s contains invalid characters", f.Name))
				continue
			}
		}

		if len(f.ValidValues) > 0 && val != "" {
			valid := false
			for _, v := range f.ValidValues {
				if val == v {
					valid = true
					break
				}
			}
			if !valid {
				errs = append(errs, fmt.Sprintf("%s must be one of: %v", f.Name, f.ValidValues))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %v", errs)
	}
	return nil
}

// SanitizeProjectName removes or replaces filesystem-unsafe characters from project names.
// It removes characters that are invalid in filenames on most filesystems.
func SanitizeProjectName(name string) string {
	// Characters that are invalid in filenames on most filesystems
	invalidChars := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00'}
	
	var result strings.Builder
	for _, r := range name {
		isInvalid := false
		for _, invalid := range invalidChars {
			if r == invalid {
				isInvalid = true
				break
			}
		}
		if !isInvalid && !unicode.IsControl(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// WithDefaults returns input with defaults applied for empty fields
// Also trims whitespace from all fields
func WithDefaults(input Input, workDir string) Input {
	input.Name = strings.TrimSpace(input.Name)
	input.IssueProvider = strings.TrimSpace(input.IssueProvider)
	input.PRProvider = strings.TrimSpace(input.PRProvider)
	
	if input.Name == "" {
		input.Name = filepath.Base(workDir)
	}
	if input.IssueProvider == "" {
		input.IssueProvider = "markdown"
	}
	if input.PRProvider == "" {
		input.PRProvider = "github"
	}
	return input
}

// ParseJSON parses JSON input into Input struct
func ParseJSON(data []byte) (Input, error) {
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return Input{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

func getFieldValue(input Input, name string) string {
	switch name {
	case "name":
		return input.Name
	case "issue_provider":
		return input.IssueProvider
	case "pr_provider":
		return input.PRProvider
	default:
		return ""
	}
}
