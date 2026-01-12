package pr

import (
	"strings"
	"testing"
)

func TestValidate_TitleLength(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		wantErr bool
	}{
		{"empty title ok", "", false},
		{"short title ok", "Fix bug", false},
		{"max length ok", strings.Repeat("a", MaxTitleLength), false},
		{"too long", strings.Repeat("a", MaxTitleLength+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(Input{Title: tt.title})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_BranchName(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		wantErr bool
	}{
		{"empty ok (auto-detect)", "", false},
		{"main ok", "main", false},
		{"feature branch ok", "feature/add-login", false},
		{"with dashes ok", "fix-bug-123", false},
		{"with underscores ok", "feature_branch", false},
		{"with dots ok", "release.1.0", false},
		{"single char ok", "a", false},

		// Invalid cases
		{"double dots invalid", "feature..branch", true},
		{"leading slash invalid", "/feature", true},
		{"trailing slash invalid", "feature/", true},
		{"space invalid", "feature branch", true},
		{"at-brace invalid", "feature@{branch", true},
		{"lock suffix invalid", "branch.lock", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(Input{Base: tt.base})
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidBranchName(t *testing.T) {
	valid := []string{
		"main",
		"develop",
		"feature/login",
		"fix-123",
		"release/v1.0.0",
		"user/john/feature",
		"a",
		"ab",
	}

	for _, name := range valid {
		if !isValidBranchName(name) {
			t.Errorf("isValidBranchName(%q) = false, want true", name)
		}
	}

	invalid := []string{
		"",
		".hidden",
		"trailing.",
		"double..dot",
		"/leading",
		"trailing/",
		"has space",
		"has@{brace",
		"file.lock",
	}

	for _, name := range invalid {
		if isValidBranchName(name) {
			t.Errorf("isValidBranchName(%q) = true, want false", name)
		}
	}
}
