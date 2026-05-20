package fuzzy

import "testing"

func TestMatch(t *testing.T) {
	cases := []struct {
		name   string
		query  string
		target string
		want   bool
	}{
		{"empty query matches anything", "", "anything", true},
		{"empty query empty target", "", "", true},
		{"exact match", "abc", "abc", true},
		{"subsequence", "abc", "a-b-c", true},
		{"case insensitive", "ABC", "alphabetic", true},
		{"missing char", "abz", "abc", false},
		{"wrong order", "ba", "abcd", false},
		{"prefix", "wir", "wire-the-picker", true},
		{"non-contiguous", "wtp", "wire-the-picker", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Match(tc.query, tc.target); got != tc.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.query, tc.target, got, tc.want)
			}
		})
	}
}
