package adapters

import (
	"context"
	"testing"
)

func TestGitHub_CreatePR(t *testing.T) {
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
			name: "success with base",
			input: PRCreateInput{
				Title: "Add feature",
				Body:  "Description",
				Base:  "main",
			},
			mockOutput: []byte("https://github.com/owner/repo/pull/42\n"),
			wantNum:    42,
			wantURL:    "https://github.com/owner/repo/pull/42",
		},
		{
			name: "success without body",
			input: PRCreateInput{
				Title: "Add feature",
				Base:  "main",
			},
			mockOutput: []byte("https://github.com/owner/repo/pull/123\n"),
			wantNum:    123,
			wantURL:    "https://github.com/owner/repo/pull/123",
		},
		{
			name: "gh error",
			input: PRCreateInput{
				Title: "Add feature",
			},
			mockOutput: []byte("error: not authenticated"),
			mockErr:    MockError("exit status 1"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			args := []string{"pr", "create", "--title", tt.input.Title}
			if tt.input.Body != "" {
				args = append(args, "--body", tt.input.Body)
			} else {
				args = append(args, "--body", "")
			}
			if tt.input.Base != "" {
				args = append(args, "--base", tt.input.Base)
			}
			exec.AddResponse("gh", args, tt.mockOutput, tt.mockErr)

			gh := NewGitHub(exec)
			result, err := gh.CreatePR(context.Background(), "/repo", tt.input)

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

func TestGitHub_IsPRMerged(t *testing.T) {
	tests := []struct {
		name       string
		prNumber   int
		mockOutput []byte
		mockErr    error
		want       bool
		wantErr    bool
	}{
		{
			name:       "merged",
			prNumber:   42,
			mockOutput: []byte(`{"mergedAt":"2024-01-15T10:30:00Z"}`),
			want:       true,
		},
		{
			name:       "not merged",
			prNumber:   42,
			mockOutput: []byte(`{"mergedAt":null}`),
			want:       false,
		},
		{
			name:       "empty mergedAt",
			prNumber:   42,
			mockOutput: []byte(`{"mergedAt":""}`),
			want:       false,
		},
		{
			name:       "error",
			prNumber:   42,
			mockErr:    MockError("not found"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("gh", []string{"pr", "view", "42", "--json", "mergedAt"}, tt.mockOutput, tt.mockErr)

			gh := NewGitHub(exec)
			got, err := gh.IsPRMerged(context.Background(), "/repo", tt.prNumber)

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

func TestGitHub_FindMergedPRByBranch(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		mockOutput []byte
		mockErr    error
		wantMerged bool
		wantNum    int
		wantErr    bool
	}{
		{
			name:       "found merged PR",
			branch:     "feature",
			mockOutput: []byte(`[{"number":42}]`),
			wantMerged: true,
			wantNum:    42,
		},
		{
			name:       "no merged PR",
			branch:     "feature",
			mockOutput: []byte(`[]`),
			wantMerged: false,
			wantNum:    0,
		},
		{
			name:    "error",
			branch:  "feature",
			mockErr: MockError("network error"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("gh", []string{"pr", "list", "--head", tt.branch, "--state", "merged", "--json", "number", "--limit", "1"}, tt.mockOutput, tt.mockErr)

			gh := NewGitHub(exec)
			merged, num, err := gh.FindMergedPRByBranch(context.Background(), "/repo", tt.branch)

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

func TestGitHub_GetPRStatus(t *testing.T) {
	tests := []struct {
		name       string
		prNumber   int
		mockOutput []byte
		mockErr    error
		want       string
		wantErr    bool
	}{
		{
			name:       "open",
			prNumber:   42,
			mockOutput: []byte("OPEN\n"),
			want:       "OPEN",
		},
		{
			name:       "merged",
			prNumber:   42,
			mockOutput: []byte("MERGED\n"),
			want:       "MERGED",
		},
		{
			name:       "closed",
			prNumber:   42,
			mockOutput: []byte("CLOSED\n"),
			want:       "CLOSED",
		},
		{
			name:     "error",
			prNumber: 42,
			mockErr:  MockError("not found"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("gh", []string{"pr", "view", "42", "--json", "state", "--jq", ".state"}, tt.mockOutput, tt.mockErr)

			gh := NewGitHub(exec)
			got, err := gh.GetPRStatus(context.Background(), "/repo", tt.prNumber)

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

func TestExtractPRNumberFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    int
		wantErr bool
	}{
		{
			name: "valid URL",
			url:  "https://github.com/owner/repo/pull/42",
			want: 42,
		},
		{
			name: "with trailing newline",
			url:  "https://github.com/owner/repo/pull/123",
			want: 123,
		},
		{
			name:    "invalid - not a number",
			url:     "https://github.com/owner/repo/pull/abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractPRNumberFromURL(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("extractPRNumberFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractPRNumberFromURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGitHub_Push(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		wantErr bool
	}{
		{
			name:    "success",
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "failure",
			mockErr: MockError("authentication failed"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, nil, tt.mockErr)

			gh := NewGitHub(exec)
			err := gh.Push(context.Background(), "/repo")

			if (err != nil) != tt.wantErr {
				t.Errorf("Push() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
