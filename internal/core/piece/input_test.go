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
		IssuePath: "issues/foo.md",
		Name:      "my-piece",
	}
	err := piece.ValidateNewPieceInput(input)
	if err == nil {
		t.Error("expected error when both set")
	}
}

func TestValidateNewPieceInput_OnlyIssuePath_Valid(t *testing.T) {
	input := piece.NewPieceInput{IssuePath: "issues/foo.md"}
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

	var data map[string]string
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := data["issue_path"]; !ok {
		t.Error("expected 'issue_path' in schema")
	}
	if _, ok := data["name"]; !ok {
		t.Error("expected 'name' in schema")
	}
}

func TestParseNewPieceJSON(t *testing.T) {
	jsonData := `{"issue_path":"issues/foo.md"}`
	input, err := piece.ParseNewPieceJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.IssuePath != "issues/foo.md" {
		t.Errorf("expected issue_path 'issues/foo.md', got %q", input.IssuePath)
	}
}

func TestWithNewPieceDefaults(t *testing.T) {
	input := piece.NewPieceInput{
		IssuePath: "  issues/foo.md  ",
		Name:      "  ",
	}
	result := piece.WithNewPieceDefaults(input)
	if result.IssuePath != "issues/foo.md" {
		t.Errorf("expected trimmed issue_path, got %q", result.IssuePath)
	}
	if result.Name != "" {
		t.Errorf("expected empty name after trim, got %q", result.Name)
	}
}
