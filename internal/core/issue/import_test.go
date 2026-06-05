package issue

import (
	"strings"
	"testing"
)

func TestResolveImportSource(t *testing.T) {
	linear := ImportSource{Name: "linear"}
	plane := ImportSource{Name: "plane"}

	tests := []struct {
		name      string
		sources   []ImportSource
		from      string
		wantName  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "single source, no explicit from, proceeds",
			sources: []ImportSource{linear},
			from:    "",

			wantName: "linear",
		},
		{
			name:      "no source configured fails",
			sources:   nil,
			from:      "",
			wantErr:   true,
			errSubstr: "no import source configured",
		},
		{
			name:      "multiple sources, no from, ambiguous",
			sources:   []ImportSource{linear, plane},
			from:      "",
			wantErr:   true,
			errSubstr: "multiple import sources",
		},
		{
			name:     "multiple sources, explicit from selects",
			sources:  []ImportSource{linear, plane},
			from:     "plane",
			wantName: "plane",
		},
		{
			name:      "explicit from not configured fails",
			sources:   []ImportSource{linear},
			from:      "plane",
			wantErr:   true,
			errSubstr: "not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveImportSource(tt.sources, tt.from)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("got source %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestImportBody_AppendsURL(t *testing.T) {
	got := importBody(RemoteIssue{Body: "hello", URL: "https://x/issue/1"})
	if !strings.Contains(got, "hello") || !strings.Contains(got, "Imported from: https://x/issue/1") {
		t.Errorf("importBody = %q", got)
	}

	// No URL: body unchanged.
	if got := importBody(RemoteIssue{Body: "only body"}); got != "only body" {
		t.Errorf("importBody without URL = %q", got)
	}
}
