package piece_test

import (
	"encoding/json"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestValidateNewPieceInput_BothEmpty_Error(t *testing.T) {
	input := piece.NewPieceInput{}
	err := piece.ValidateNewPieceInput(input)
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestValidateNewPieceInput_BothSet_Error(t *testing.T) {
	input := piece.NewPieceInput{
		Issue: piece.IssueRef{Provider: "markdown", ID: "issues/foo.md", Title: "Test"},
		Name:  "my-piece",
	}
	err := piece.ValidateNewPieceInput(input)
	if err == nil {
		t.Error("expected error when both set")
	}
}

func TestValidateNewPieceInput_OnlyIssue_Valid(t *testing.T) {
	input := piece.NewPieceInput{
		Issue: piece.IssueRef{Provider: "markdown", ID: "issues/foo.md", Title: "Test"},
	}
	err := piece.ValidateNewPieceInput(input)
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidateNewPieceInput_OnlyName_Valid(t *testing.T) {
	input := piece.NewPieceInput{Name: "my-piece"}
	err := piece.ValidateNewPieceInput(input)
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestNewPieceSchema(t *testing.T) {
	schema, err := piece.NewPieceSchema()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := data["issue"]; !ok {
		t.Error("expected 'issue' in schema")
	}
	if _, ok := data["name"]; !ok {
		t.Error("expected 'name' in schema")
	}
	if _, ok := data["skip_switch"]; !ok {
		t.Error("expected 'skip_switch' in schema")
	}
	if _, ok := data["overwrite_session"]; !ok {
		t.Error("expected 'overwrite_session' in schema")
	}
}

func TestParseNewPieceJSON(t *testing.T) {
	jsonData := `{"issue":{"provider":"markdown","id":"issues/foo.md","title":"Test Issue"}}`
	input, err := piece.ParseNewPieceJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Issue.ID != "issues/foo.md" {
		t.Errorf("expected issue ID 'issues/foo.md', got %q", input.Issue.ID)
	}
}

func TestValidateNewPieceInput_OnlyPrompt_Valid(t *testing.T) {
	input := piece.NewPieceInput{Prompt: "add dark mode"}
	err := piece.ValidateNewPieceInput(input)
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidateNewPieceInput_PromptAndName_Valid(t *testing.T) {
	input := piece.NewPieceInput{Prompt: "add dark mode", Name: "dark-mode"}
	err := piece.ValidateNewPieceInput(input)
	if err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
}

func TestValidateNewPieceInput_PromptAndIssue_Error(t *testing.T) {
	input := piece.NewPieceInput{
		Prompt: "add dark mode",
		Issue:  piece.IssueRef{Provider: "markdown", ID: "issues/foo.md", Title: "Test"},
	}
	err := piece.ValidateNewPieceInput(input)
	if err == nil {
		t.Error("expected error when both prompt and issue set")
	}
}

func TestWithNewPieceDefaults(t *testing.T) {
	input := piece.NewPieceInput{
		Name: "  my-piece  ",
	}
	result := piece.WithNewPieceDefaults(input)
	if result.Name != "my-piece" {
		t.Errorf("expected trimmed name 'my-piece', got %q", result.Name)
	}
	if result.Parent != "main" {
		t.Errorf("expected default parent 'main', got %q", result.Parent)
	}
}

func TestWithNewPieceDefaults_PromptDerivesName(t *testing.T) {
	input := piece.NewPieceInput{
		Prompt: "add dark mode support",
	}
	result := piece.WithNewPieceDefaults(input)
	if result.Name == "" {
		t.Error("expected name to be derived from prompt")
	}
	if result.Prompt != "add dark mode support" {
		t.Errorf("expected prompt preserved, got %q", result.Prompt)
	}
}

func TestWithNewPieceDefaults_PromptNameNotOverridden(t *testing.T) {
	input := piece.NewPieceInput{
		Prompt: "add dark mode support",
		Name:   "custom-name",
	}
	result := piece.WithNewPieceDefaults(input)
	if result.Name != "custom-name" {
		t.Errorf("expected explicit name 'custom-name', got %q", result.Name)
	}
}

func TestNewPieceSchema_HasPrompt(t *testing.T) {
	schema, err := piece.NewPieceSchema()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := data["prompt"]; !ok {
		t.Error("expected 'prompt' in schema")
	}
}
