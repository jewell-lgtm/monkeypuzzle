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
