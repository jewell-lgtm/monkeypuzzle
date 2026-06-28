// Package input provides shared types and utilities for command input handling.
// This package enables consistent input processing across all commands while
// keeping the core handlers agnostic to input mechanism (TUI, JSON, flags).
package input

import (
	"encoding/json"
	"strconv"
)

// Field defines a single input field with validation rules.
// Used for schema generation and TUI field rendering.
type Field struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	ValidValues []string `json:"valid_values,omitempty"`
	// Type is the JSON type of the field's value: "" (string, the default),
	// "bool", or "int". It controls how Default is emitted in GenerateSchema so
	// the schema round-trips back through the input struct (e.g. a bool default
	// is emitted as JSON `false`, not the string "false").
	Type string `json:"type,omitempty"`
}

// GenerateSchema creates a JSON schema from field definitions.
// Returns a map of field names to their (typed) default values.
func GenerateSchema(fields []Field) ([]byte, error) {
	schema := make(map[string]any)
	for _, f := range fields {
		schema[f.Name] = typedDefault(f)
	}
	return json.MarshalIndent(schema, "", "  ")
}

// typedDefault converts a field's string Default into a value of its declared
// Type so the generated schema marshals to the correct JSON type.
func typedDefault(f Field) any {
	switch f.Type {
	case "bool":
		return f.Default == "true"
	case "int":
		n, _ := strconv.Atoi(f.Default)
		return n
	default:
		return f.Default
	}
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
