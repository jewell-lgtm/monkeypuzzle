package adapters

import (
	"context"
	"errors"
	"testing"
)

func TestGitLab_CreatePR(t *testing.T) {
	tests := []struct {
		name       string
		input      PRCreateInput
		mockOutput []byte
		mockErr    error
		wantNum    int
		wantURL    string
		wantErr    bool
	}{
		{
			name:       "success with base",
			input:      PRCreateInput{Title: "Add feature", Body: "Description", Base: "main"},
			mockOutput: []byte("Creating merge request for feature into main\n\nhttps://gitlab.com/group/project/-/merge_requests/42\n"),
			wantNum:    42,
			wantURL:    "https://gitlab.com/group/project/-/merge_requests/42",
		},
		{
			name:       "success draft without body",
			input:      PRCreateInput{Title: "Add feature", Base: "main", Draft: true},
			mockOutput: []byte("https://gitlab.com/group/project/-/merge_requests/123\n"),
			wantNum:    123,
			wantURL:    "https://gitlab.com/group/project/-/merge_requests/123",
		},
		{
			name:       "glab error",
			input:      PRCreateInput{Title: "Add feature"},
			mockOutput: []byte("error: not authenticated"),
			mockErr:    MockError("exit status 1"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			args := []string{"mr", "create", "--yes", "--title", tt.input.Title}
			if tt.input.Body != "" {
				args = append(args, "--description", tt.input.Body)
			} else {
				args = append(args, "--description", "")
			}
			if tt.input.Base != "" {
				args = append(args, "--target-branch", tt.input.Base)
			}
			if tt.input.Draft {
				args = append(args, "--draft")
			}
			exec.AddResponse("glab", args, tt.mockOutput, tt.mockErr)

			gl := NewGitLab(exec)
			result, err := gl.CreatePR(context.Background(), "/repo", tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreatePR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if result.Number != tt.wantNum {
				t.Errorf("CreatePR() Number = %v, want %v", result.Number, tt.wantNum)
			}
			if result.URL != tt.wantURL {
				t.Errorf("CreatePR() URL = %v, want %v", result.URL, tt.wantURL)
			}
		})
	}
}

func TestGitLab_Push(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{name: "success", mockErr: nil, wantErr: false},
		{name: "failure", mockErr: MockError("authentication failed"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, nil, tt.mockErr)

			gl := NewGitLab(exec)
			err := gl.Push(context.Background(), "/repo")

			if (err != nil) != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitLab_MarkPRReady(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{name: "success", mockErr: nil, wantErr: false},
		{name: "failure", mockErr: MockError("not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("glab", []string{"mr", "update", "42", "--ready"}, nil, tt.mockErr)

			gl := NewGitLab(exec)
			err := gl.MarkPRReady(context.Background(), "/repo", 42)

			if (err != nil) != tt.wantErr {
				t.Errorf("MarkPRReady() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGitLab_GetPRStatus(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput []byte
		mockErr    error
		want       string
		wantErr    bool
	}{
		{name: "opened normalizes to OPEN", mockOutput: []byte(`{"state":"opened"}`), want: "OPEN"},
		{name: "merged", mockOutput: []byte(`{"state":"merged"}`), want: "MERGED"},
		{name: "closed", mockOutput: []byte(`{"state":"closed"}`), want: "CLOSED"},
		{name: "locked normalizes to CLOSED", mockOutput: []byte(`{"state":"locked"}`), want: "CLOSED"},
		{name: "error", mockErr: MockError("not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("glab", []string{"mr", "view", "42", "-F", "json"}, tt.mockOutput, tt.mockErr)

			gl := NewGitLab(exec)
			got, err := gl.GetPRStatus(context.Background(), "/repo", 42)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetPRStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetPRStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitLab_IsPRMerged(t *testing.T) {
	tests := []struct {
		name       string
		mockOutput []byte
		mockErr    error
		want       bool
		wantErr    bool
	}{
		{name: "merged state", mockOutput: []byte(`{"state":"merged"}`), want: true},
		{name: "merged_at set", mockOutput: []byte(`{"state":"opened","merged_at":"2024-01-15T10:30:00Z"}`), want: true},
		{name: "not merged", mockOutput: []byte(`{"state":"opened","merged_at":null}`), want: false},
		{name: "error", mockErr: MockError("not found"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("glab", []string{"mr", "view", "42", "-F", "json"}, tt.mockOutput, tt.mockErr)

			gl := NewGitLab(exec)
			got, err := gl.IsPRMerged(context.Background(), "/repo", 42)

			if (err != nil) != tt.wantErr {
				t.Errorf("IsPRMerged() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("IsPRMerged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitLab_FindMergedPRByBranch(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		mockOutput []byte
		mockErr    error
		wantMerged bool
		wantNum    int
		wantErr    bool
	}{
		{name: "found merged MR", branch: "feature", mockOutput: []byte(`[{"iid":42}]`), wantMerged: true, wantNum: 42},
		{name: "no merged MR", branch: "feature", mockOutput: []byte(`[]`), wantMerged: false, wantNum: 0},
		{name: "error", branch: "feature", mockErr: MockError("network error"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("glab", []string{"mr", "list", "--source-branch", tt.branch, "--state", "merged", "-F", "json"}, tt.mockOutput, tt.mockErr)

			gl := NewGitLab(exec)
			merged, num, err := gl.FindMergedPRByBranch(context.Background(), "/repo", tt.branch)

			if (err != nil) != tt.wantErr {
				t.Errorf("FindMergedPRByBranch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if merged != tt.wantMerged {
				t.Errorf("FindMergedPRByBranch() merged = %v, want %v", merged, tt.wantMerged)
			}
			if num != tt.wantNum {
				t.Errorf("FindMergedPRByBranch() num = %v, want %v", num, tt.wantNum)
			}
		})
	}
}

func TestGitLab_ListPRs(t *testing.T) {
	t.Run("maps iid/branches/state/url", func(t *testing.T) {
		exec := NewMockExec()
		out := []byte(`[
			{"iid":1,"source_branch":"feat-a","target_branch":"main","state":"opened","web_url":"https://gitlab.com/g/p/-/merge_requests/1"},
			{"iid":2,"source_branch":"feat-b","target_branch":"feat-a","state":"merged","web_url":"https://gitlab.com/g/p/-/merge_requests/2"}
		]`)
		exec.AddResponse("glab", []string{"mr", "list", "--all", "--per-page", "100", "-F", "json"}, out, nil)

		gl := NewGitLab(exec)
		prs, err := gl.ListPRs(context.Background(), "/repo")
		if err != nil {
			t.Fatalf("ListPRs() error = %v", err)
		}
		if len(prs) != 2 {
			t.Fatalf("ListPRs() len = %d, want 2", len(prs))
		}
		if prs[0] != (PRInfo{Number: 1, HeadRefName: "feat-a", BaseRefName: "main", State: "OPEN", URL: "https://gitlab.com/g/p/-/merge_requests/1"}) {
			t.Errorf("ListPRs()[0] = %+v", prs[0])
		}
		if prs[1].State != "MERGED" || prs[1].BaseRefName != "feat-a" {
			t.Errorf("ListPRs()[1] = %+v", prs[1])
		}
	})

	t.Run("glab unavailable degrades to ErrGlabUnavailable + empty slice", func(t *testing.T) {
		exec := NewMockExec()
		exec.AddResponse("glab", []string{"mr", "list", "--all", "--per-page", "100", "-F", "json"}, nil, MockError("exit status 1"))

		gl := NewGitLab(exec)
		prs, err := gl.ListPRs(context.Background(), "/repo")
		if !errors.Is(err, ErrGlabUnavailable) {
			t.Errorf("ListPRs() error = %v, want ErrGlabUnavailable", err)
		}
		if len(prs) != 0 {
			t.Errorf("ListPRs() len = %d, want 0", len(prs))
		}
	})
}

func TestGitLab_SetPRBase(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{name: "success", mockErr: nil, wantErr: false},
		{name: "failure", mockErr: MockError("forbidden"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("glab", []string{"mr", "update", "42", "--target-branch", "main"}, nil, tt.mockErr)

			gl := NewGitLab(exec)
			err := gl.SetPRBase(context.Background(), "/repo", 42, "main")

			if (err != nil) != tt.wantErr {
				t.Errorf("SetPRBase() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractGitLabMRURL(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "banner then url",
			out:  "Creating merge request for x into main\n\nhttps://gitlab.com/group/project/-/merge_requests/7\n",
			want: "https://gitlab.com/group/project/-/merge_requests/7",
		},
		{
			name: "no merge_requests url",
			out:  "some unrelated output https://gitlab.com/group/project\n",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractGitLabMRURL(tt.out); got != tt.want {
				t.Errorf("extractGitLabMRURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractMRNumberFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    int
		wantErr bool
	}{
		{name: "valid", url: "https://gitlab.example.com/group/project/-/merge_requests/123", want: 123},
		{name: "trailing slash", url: "https://gitlab.com/g/p/-/merge_requests/9/", want: 9},
		{name: "not a number", url: "https://gitlab.com/g/p/-/merge_requests/abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractMRNumberFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractMRNumberFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractMRNumberFromURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
