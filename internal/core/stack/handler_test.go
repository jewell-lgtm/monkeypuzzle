package stack

import "testing"

func TestSplitRemoteRef(t *testing.T) {
	tests := []struct {
		from       string
		wantRemote string
		wantRef    string
	}{
		{"origin/main", "origin", "main"},
		{"upstream/dev", "upstream", "dev"},
		{"origin/feature/x", "origin", "feature/x"}, // branch keeps its slashes
		{"main", "", "main"},                        // bare ref, no remote
		{"", "", ""},
	}
	for _, tt := range tests {
		remote, ref := splitRemoteRef(tt.from)
		if remote != tt.wantRemote || ref != tt.wantRef {
			t.Errorf("splitRemoteRef(%q) = (%q, %q), want (%q, %q)", tt.from, remote, ref, tt.wantRemote, tt.wantRef)
		}
	}
}
