package input

import (
	"encoding/json"
	"testing"
)

func TestGenerateSchema(t *testing.T) {
	fields := []Field{
		{Name: "title", Default: ""},
		{Name: "count", Default: "10"},
	}

	data, err := GenerateSchema(fields)
	if err != nil {
		t.Fatalf("GenerateSchema failed: %v", err)
	}

	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("failed to unmarshal schema: %v", err)
	}

	if schema["title"] != "" {
		t.Errorf("expected title='', got %q", schema["title"])
	}
	if schema["count"] != "10" {
		t.Errorf("expected count='10', got %q", schema["count"])
	}
}

func TestGetDefaults(t *testing.T) {
	fields := []Field{
		{Name: "title", Default: ""},
		{Name: "count", Default: "10"},
	}

	defaults := GetDefaults(fields)

	if defaults["title"] != "" {
		t.Errorf("expected title='', got %q", defaults["title"])
	}
	if defaults["count"] != "10" {
		t.Errorf("expected count='10', got %q", defaults["count"])
	}
}

func TestParseJSON(t *testing.T) {
	type TestInput struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	data := []byte(`{"name": "test", "count": 42}`)
	result, err := ParseJSON[TestInput](data)
	if err != nil {
		t.Fatalf("ParseJSON failed: %v", err)
	}

	if result.Name != "test" {
		t.Errorf("expected name='test', got %q", result.Name)
	}
	if result.Count != 42 {
		t.Errorf("expected count=42, got %d", result.Count)
	}
}

func TestParseJSON_Invalid(t *testing.T) {
	type TestInput struct {
		Name string `json:"name"`
	}

	data := []byte(`{invalid json}`)
	_, err := ParseJSON[TestInput](data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
