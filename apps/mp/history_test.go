package main

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
)

func TestSummarizeData(t *testing.T) {
	cases := []struct {
		name string
		ev   history.Event
		want string
	}{
		{"plain piece", history.Event{Piece: "a", Branch: "a", Parent: "main"}, ""},
		{"adopted branch + stacked", history.Event{Piece: "a", Branch: "feat/a", Parent: "b"}, "branch=feat/a parent=b"},
		{"pr data sorted, ints unfloated", history.Event{Data: map[string]any{"pr_url": "u", "pr_number": float64(7), "base": "main"}}, "base=main pr_number=7 pr_url=u"},
		{"list + empty/nil skipped", history.Event{Data: map[string]any{"pieces": []any{"x", "y"}, "pushed": nil, "note": ""}}, "pieces=x,y"},
	}
	for _, tc := range cases {
		if got := summarizeData(tc.ev); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
