package pr

import (
	"encoding/json"
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
		Description: "Base branch to merge into",
		Required:    false,
		Default:     "main",
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

// WithDefaults returns input with defaults applied and whitespace trimmed
func WithDefaults(input Input) Input {
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.Base = strings.TrimSpace(input.Base)

	if input.Base == "" {
		input.Base = "main"
	}

	return input
}

// ParseJSON parses JSON input into Input struct
func ParseJSON(data []byte) (Input, error) {
	var input Input
	if err := json.Unmarshal(data, &input); err != nil {
		return Input{}, err
	}
	return input, nil
}
