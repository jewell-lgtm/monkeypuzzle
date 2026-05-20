// Package fuzzy provides a tiny case-insensitive subsequence matcher used by
// monkeypuzzle's pickers and providers.
package fuzzy

import "strings"

// Match returns true if every character of query appears in target in order,
// case-insensitively. An empty query matches everything.
func Match(query, target string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	target = strings.ToLower(target)

	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}
