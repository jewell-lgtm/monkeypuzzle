package issue

import (
	"encoding/json"
	"fmt"
	"strings"
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
		Name:        "title",
		Description: "Issue title",
		Required:    true,
	},
	{
		Name:        "description",
		Description: "Issue description",
		Required:    false,
		Default:     "",
	},
}

// Input holds validated input for issue create
type Input struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Schema returns the JSON schema with defaults for issue create
func Schema() ([]byte, error) {
	schema := map[string]any{}
	for _, f := range fields {
		schema[f.Name] = f.Default
	}
	return json.MarshalIndent(schema, "", "  ")
}

// Fields returns field definitions for TUI generation
func Fields() []Field {
	return fields
}

// GetDefaults returns default values for all fields.
func GetDefaults() map[string]string {
	defaults := make(map[string]string)
	for _, f := range fields {
		defaults[f.Name] = f.Default
	}
	return defaults
}

// Validate validates input and returns errors for invalid fields
func Validate(input Input) error {
	var errs []string

	title := strings.TrimSpace(input.Title)
	if title == "" {
		errs = append(errs, "title is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %v", errs)
	}
	return nil
}

// WithDefaults returns input with defaults applied and whitespace trimmed
func WithDefaults(input Input) Input {
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
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

// ListInput holds input for issue list command
type ListInput struct {
	Status []string `json:"status,omitempty"`
}

// ListSchema returns the JSON schema for issue list input
func ListSchema() ([]byte, error) {
	schema := map[string]any{
		"status": []string{},
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseListJSON parses JSON input into ListInput
func ParseListJSON(data []byte) (ListInput, error) {
	var input ListInput
	if err := json.Unmarshal(data, &input); err != nil {
		return ListInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidStatuses returns valid status values for filtering
func ValidStatuses() []string {
	return []string{"todo", "in-progress", "done"}
}

// ValidateListInput validates the list input
func ValidateListInput(input ListInput) error {
	valid := ValidStatuses()
	for _, s := range input.Status {
		found := false
		for _, v := range valid {
			if s == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid status %q (valid: %v)", s, valid)
		}
	}
	return nil
}
