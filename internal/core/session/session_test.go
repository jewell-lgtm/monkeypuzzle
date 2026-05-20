package session

import "testing"

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"plain":          "plain",
		"with.dots":      "with-dots",
		"a:b:c":          "a-b-c",
		"has space":      "has-space",
		"tab\tsep":       "tab-sep",
		".leading":       "leading",
		"trailing.":      "trailing",
		"...":            "unnamed",
		"":               "unnamed",
		"café":           "café",
		"mixed. :stuff ": "mixed---stuff",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestName(t *testing.T) {
	if got := MainName("monkeypuzzle"); got != "mp/monkeypuzzle" {
		t.Errorf("MainName = %q", got)
	}
	if got := Name("monkeypuzzle", "fix-login"); got != "mp/monkeypuzzle/fix-login" {
		t.Errorf("Name = %q", got)
	}
	if got := Name("My App", "feat.x"); got != "mp/My-App/feat-x" {
		t.Errorf("Name = %q", got)
	}
}
