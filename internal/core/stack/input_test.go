package stack

import (
	"encoding/json"
	"testing"
)

func TestWithSyncDefaults(t *testing.T) {
	got := WithSyncDefaults(SyncInput{})
	if got.MainBranch != "main" {
		t.Errorf("expected main branch default 'main', got %q", got.MainBranch)
	}
	if got.Strategy != StrategyMerge {
		t.Errorf("expected default strategy merge, got %q", got.Strategy)
	}
}

func TestValidateSyncInput(t *testing.T) {
	tests := []struct {
		strategy string
		wantErr  bool
	}{
		{"", false},
		{StrategyMerge, false},
		{StrategyRebase, false},
		{"squash", true},
	}
	for _, tt := range tests {
		err := ValidateSyncInput(SyncInput{Strategy: tt.strategy})
		if (err != nil) != tt.wantErr {
			t.Errorf("strategy %q: wantErr=%v, got %v", tt.strategy, tt.wantErr, err)
		}
	}
}

func TestParseSyncJSON_RoundTrip(t *testing.T) {
	in := SyncInput{MainBranch: "trunk", Strategy: StrategyRebase, Push: true, Stack: true}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSyncJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Errorf("round trip mismatch: got %+v want %+v", got, in)
	}
}

func TestSyncSchema_Valid(t *testing.T) {
	schema, err := SyncSchema()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(schema, &m); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	for _, k := range []string{"main_branch", "strategy", "push", "stack"} {
		if _, ok := m[k]; !ok {
			t.Errorf("schema missing key %q", k)
		}
	}
}

func TestWithStatusDefaults(t *testing.T) {
	got := WithStatusDefaults(StatusInput{})
	if got.MainBranch != "main" {
		t.Errorf("expected main default, got %q", got.MainBranch)
	}
}

func TestValidateAppendInput(t *testing.T) {
	if err := ValidateAppendInput(AppendInput{}); err == nil {
		t.Error("expected error for empty append input")
	}
	if err := ValidateAppendInput(AppendInput{Name: "x"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := ValidateAppendInput(AppendInput{Prompt: "do x"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
