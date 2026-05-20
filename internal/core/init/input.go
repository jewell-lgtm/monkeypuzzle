package init

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
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
		Description: "Issue import source (markdown = none; local store is always markdown). Consumed only by `mp issue import`.",
		Required:    true,
		Default:     "markdown",
		ValidValues: []string{"markdown", "linear", "plane", "gitlab"},
	},
	{
		Name:        "pr_provider",
		Description: "How PRs/MRs are managed",
		Required:    true,
		Default:     "github",
		ValidValues: []string{"github", "gitlab"},
	},
	{
		Name:        "dir",
		Description: "Directory (relative to the repo root) for monkeypuzzle state",
		Required:    false,
		Default:     projectdir.DefaultDirName,
	},
}

// Input holds validated input for the init command
type Input struct {
	Name          string            `json:"name"`
	IssueProvider string            `json:"issue_provider"`
	IssueConfig   map[string]string `json:"issue_config,omitempty"` // provider-specific config (e.g., api_key, team for linear)
	PRProvider    string            `json:"pr_provider"`
	Dir           string            `json:"dir,omitempty"`          // monkeypuzzle state dir, relative to repo root; defaults to ".monkeypuzzle"
	CreateSkill   *bool             `json:"create_skill,omitempty"` // nil means default (true)
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
	schema["create_skill"] = true

	return json.MarshalIndent(schema, "", "  ")
}

// Fields returns field definitions for documentation/TUI generation
func Fields() []Field {
	return fields
}

// GetDefaults returns default values for all fields.
// Used by TUI to set initial values.
func GetDefaults(workDir string) map[string]string {
	defaults := make(map[string]string)
	for _, f := range fields {
		def := f.Default
		if f.Name == "name" && def == "" {
			def = filepath.Base(workDir)
		}
		defaults[f.Name] = def
	}
	return defaults
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

	// monkeypuzzle dir must be a safe relative path within the repo
	if input.Dir != "" && !projectdir.ValidRel(filepath.Clean(input.Dir)) {
		errs = append(errs, fmt.Sprintf("dir %q must be a relative path within the repo (no \"..\", no absolute paths)", input.Dir))
	}

	// Linear provider requires team config
	if input.IssueProvider == "linear" {
		team := ""
		if input.IssueConfig != nil {
			team = input.IssueConfig["team"]
		}
		if team == "" {
			errs = append(errs, "linear provider requires 'team' config")
		}
	}

	// Plane provider requires workspace and project config
	if input.IssueProvider == "plane" {
		var workspace, project string
		if input.IssueConfig != nil {
			workspace = input.IssueConfig["workspace"]
			project = input.IssueConfig["project"]
		}
		if workspace == "" {
			errs = append(errs, "plane provider requires 'workspace' config")
		}
		if project == "" {
			errs = append(errs, "plane provider requires 'project' config")
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
	input.Dir = strings.TrimSpace(input.Dir)

	if input.Name == "" {
		input.Name = filepath.Base(workDir)
	}
	if input.IssueProvider == "" {
		input.IssueProvider = "markdown"
	}
	if input.PRProvider == "" {
		input.PRProvider = "github"
	}
	if input.Dir == "" {
		input.Dir = projectdir.DefaultDirName
	} else {
		input.Dir = filepath.Clean(input.Dir)
	}
	if input.CreateSkill == nil {
		t := true
		input.CreateSkill = &t
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
	case "dir":
		return input.Dir
	default:
		return ""
	}
}
