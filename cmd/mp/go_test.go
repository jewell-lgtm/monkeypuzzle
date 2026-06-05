package mp

import "testing"

func TestRemoteBranchShortName(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"simple remote ref", "origin/foo", "foo"},
		{"nested branch keeps slashes", "origin/feature/x", "feature/x"},
		{"other remote", "upstream/bar", "bar"},
		{"no slash", "origin", ""},
		{"trailing slash", "origin/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remoteBranchShortName(tt.ref); got != tt.want {
				t.Errorf("remoteBranchShortName(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}
