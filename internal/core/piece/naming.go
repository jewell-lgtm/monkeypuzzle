package piece

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	DefaultFilePerm = 0644

	// maxPieceNameWords and maxPieceNameLen bound the length of a name derived
	// from a free-form prompt. The full prompt is preserved in piece metadata,
	// so the name only needs to be a short, readable identifier rather than
	// carrying the whole sentence into the branch name.
	maxPieceNameWords = 5
	maxPieceNameLen   = 40
)

// hyphenRegex matches one or more consecutive hyphens.
var hyphenRegex = regexp.MustCompile(`-+`)

// SanitizePieceName turns a free-form string (a prompt or branch name) into a
// safe, short piece name: lowercased, with spaces and special characters
// collapsed to hyphens and invalid filesystem characters removed.
func SanitizePieceName(name string) string {
	// Characters that are invalid in filenames on most filesystems
	invalidChars := []rune{'/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00'}

	var result strings.Builder
	prevWasSeparator := false

	for _, r := range strings.ToLower(name) {
		// Check if it's an invalid character
		isInvalid := false
		for _, invalid := range invalidChars {
			if r == invalid {
				isInvalid = true
				break
			}
		}

		if isInvalid || unicode.IsControl(r) {
			// Replace with hyphen if not already one
			if !prevWasSeparator {
				result.WriteRune('-')
				prevWasSeparator = true
			}
			continue
		}

		// Replace spaces and other separators with hyphens
		if unicode.IsSpace(r) || r == '_' || r == '.' {
			if !prevWasSeparator {
				result.WriteRune('-')
				prevWasSeparator = true
			}
			continue
		}

		// Replace other punctuation with hyphens
		if unicode.IsPunct(r) && r != '-' {
			if !prevWasSeparator {
				result.WriteRune('-')
				prevWasSeparator = true
			}
			continue
		}

		// Keep alphanumeric and hyphens
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			result.WriteRune(r)
			prevWasSeparator = false
		}
	}

	// Trim hyphens from start and end
	resultStr := strings.Trim(result.String(), "-")

	// Replace multiple consecutive hyphens with single hyphen
	resultStr = hyphenRegex.ReplaceAllString(resultStr, "-")

	// Ensure it's not empty
	if resultStr == "" {
		return "piece"
	}

	return shortenSlug(resultStr)
}

// shortenSlug trims a hyphen-separated slug to a short identifier, keeping whole
// words up to maxPieceNameWords and maxPieceNameLen. At least the first word is
// always kept (hard-cut if that single word exceeds the length budget).
func shortenSlug(slug string) string {
	words := strings.Split(slug, "-")

	var kept []string
	length := 0
	for _, w := range words {
		if len(kept) >= maxPieceNameWords {
			break
		}
		// +1 accounts for the joining hyphen between words.
		next := length + len(w)
		if len(kept) > 0 {
			next++
		}
		if len(kept) > 0 && next > maxPieceNameLen {
			break
		}
		kept = append(kept, w)
		length = next
	}

	result := strings.Join(kept, "-")
	if len(result) > maxPieceNameLen {
		result = strings.TrimRight(result[:maxPieceNameLen], "-")
	}
	return result
}
