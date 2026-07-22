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

func TestValidSSHDest(t *testing.T) {
	for _, ok := range []string{"wire", "user@wire", "wire.example.com", "10.0.0.7", "box_1"} {
		if err := ValidSSHDest(ok); err != nil {
			t.Errorf("ValidSSHDest(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=x", "host name", "host;rm", "host`x`", "a'b", `a"b`, "a$b", "a|b", "a&b", "a\\b"} {
		if err := ValidSSHDest(bad); err == nil {
			t.Errorf("ValidSSHDest(%q) = nil, want error", bad)
		}
	}
}
