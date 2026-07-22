package cli

import "testing"

func TestShQuote(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"simple", "'simple'"},
		{"/home/matt/code/api", "'/home/matt/code/api'"},
		{"with space", "'with space'"},
		{"it's", `'it'\''s'`},
		{"'; rm -rf /; '", `''\''; rm -rf /; '\'''`},
		{"a$b`c", "'a$b`c'"},
		{"multi\nline", "'multi\nline'"},
		{"~/code", "'~/code'"}, // tilde does NOT expand inside quotes
	}
	for _, tt := range tests {
		if got := ShQuote(tt.in); got != tt.want {
			t.Errorf("ShQuote(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}
