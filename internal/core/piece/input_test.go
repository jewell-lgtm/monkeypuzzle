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

func TestFlattenSchema(t *testing.T) {
	schema, err := piece.FlattenSchema()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"force", "delete_branches", "dry_run"} {
		if _, ok := data[key]; !ok {
			t.Errorf("expected %q in schema", key)
		}
	}
}

func TestParseFlattenJSON(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    piece.FlattenInput
		wantErr bool
	}{
		{
			name: "all flags set",
			json: `{"force":true,"delete_branches":true,"dry_run":true}`,
			want: piece.FlattenInput{Force: true, DeleteBranches: true, DryRun: true},
		},
		{
			name: "empty object defaults to false",
			json: `{}`,
			want: piece.FlattenInput{},
		},
		{
			name: "partial input",
			json: `{"force":true}`,
			want: piece.FlattenInput{Force: true},
		},
		{
			name:    "invalid JSON",
			json:    `{not json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := piece.ParseFlattenJSON([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
