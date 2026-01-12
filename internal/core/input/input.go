// Package input provides shared types and utilities for command input handling.
// This package enables consistent input processing across all commands while
// keeping the core handlers agnostic to input mechanism (TUI, JSON, flags).
package input

import (
	"encoding/json"
)

// Field defines a single input field with validation rules.
// Used for schema generation and TUI field rendering.
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	ValidValues []string `json:"valid_values,omitempty"`
}

// GenerateSchema creates a JSON schema from field definitions.
// Returns a map of field names to their default values.
func GenerateSchema(fields []Field) ([]byte, error) {
	schema := make(map[string]any)
	for _, f := range fields {
		schema[f.Name] = f.Default
	}
	return json.MarshalIndent(schema, "", "  ")
}

// GetDefaults returns a map of field names to default values.
func GetDefaults(fields []Field) map[string]string {
	defaults := make(map[string]string)
	for _, f := range fields {
		defaults[f.Name] = f.Default
	}
	return defaults
}

// ParseJSON is a generic JSON parser for input structs.
// Use with type parameter: input.ParseJSON[MyInput](data)
func ParseJSON[T any](data []byte) (T, error) {
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
