package piece

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
)

// Status values for issue tracking
const (
	StatusTodo       = "todo"
	StatusInProgress = "in-progress"
	StatusDone       = "done"
	DefaultStatus    = StatusTodo
	DefaultFilePerm  = 0644
)

var validStatuses = []string{StatusTodo, StatusInProgress, StatusDone}

var (
	// titleRegex matches "title: value" in YAML frontmatter (case-insensitive)
	titleRegex = regexp.MustCompile(`(?i)^title:\s*(.+)$`)
	// statusRegex matches "status: value" in YAML frontmatter (case-insensitive)
	statusRegex = regexp.MustCompile(`(?i)^status:\s*(.+)$`)
	// hyphenRegex matches one or more consecutive hyphens
	hyphenRegex = regexp.MustCompile(`-+`)
)

// ExtractIssueName extracts the issue name from a markdown file.
// Priority: 1) YAML frontmatter title, 2) First H1 heading, 3) Filename
func ExtractIssueName(issuePath string, fs core.FS) (string, error) {
	// Read the issue file
	content, err := fs.ReadFile(issuePath)
	if err != nil {
		return "", fmt.Errorf("failed to read issue file: %w", err)
	}

	text := string(content)

	// Try frontmatter first
	if title := extractFromFrontmatter(text); title != "" {
		return title, nil
	}

	// Try H1 heading
	if title := extractFromH1(text); title != "" {
		return title, nil
	}

	// Fallback to filename
	return extractFromFilename(issuePath), nil
}

// extractFromFrontmatter extracts the title from YAML frontmatter.
// Looks for frontmatter between --- delimiters at the start of the file.
func extractFromFrontmatter(text string) string {
	// Check if file starts with frontmatter delimiter
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return ""
	}

	// Find the end of frontmatter (next ---)
	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return ""
	}

	// Skip the first --- line
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return ""
	}

	// Extract frontmatter content
	frontmatter := strings.Join(lines[1:endIdx], "\n")

	// Look for title: field (simple regex-based parsing)
	// Match "title: value" or "title: 'value'" or "title: \"value\""
	for _, line := range strings.Split(frontmatter, "\n") {
		matches := titleRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) > 1 {
			title := strings.TrimSpace(matches[1])
			// Remove quotes if present
			title = strings.Trim(title, `"'`)
			return title
		}
	}

	return ""
}

// extractFromH1 extracts the first H1 heading from the markdown.
// Note: This does not skip code blocks, so an H1 inside a code block (fenced or indented)
// will be matched. This is acceptable because:
// - Most markdown files don't have code blocks before the first H1
// - The filename fallback provides a reasonable default if extraction fails
// - Adding code block detection would significantly increase complexity
func extractFromH1(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match H1: starts with # followed by space
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimSpace(trimmed[2:])
			if title != "" {
				return title
			}
		}
	}
	return ""
}

// extractFromFilename extracts the name from the filename (without extension).
func extractFromFilename(issuePath string) string {
	base := filepath.Base(issuePath)
	ext := filepath.Ext(base)
	if ext != "" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

// SanitizePieceName sanitizes an issue name for use as a piece name.
// Converts to lowercase, replaces spaces and special chars with hyphens,
// and removes invalid filesystem characters.
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

	return resultStr
}

// ReadConfig reads the monkeypuzzle config from the repository root.
func ReadConfig(repoRoot string, fs core.FS) (*initcmd.Config, error) {
	configPath := filepath.Join(repoRoot, initcmd.DirName, initcmd.ConfigFile)

	data, err := fs.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg initcmd.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// ResolveIssuePath resolves an issue path (absolute or relative) to an absolute path.
// If relative, resolves from repoRoot. Uses fs to verify the file exists.
func ResolveIssuePath(repoRoot, issuePath string, fs core.FS) (string, error) {
	if filepath.IsAbs(issuePath) {
		// Verify the absolute path exists
		if _, err := fs.Stat(issuePath); err != nil {
			return "", fmt.Errorf("issue file not found: %s", issuePath)
		}
		return issuePath, nil
	}

	// Try resolving relative to repo root
	absPath := filepath.Join(repoRoot, issuePath)
	absPath = filepath.Clean(absPath)

	// Verify the path exists
	if _, err := fs.Stat(absPath); err != nil {
		return "", fmt.Errorf("issue file not found: %s", issuePath)
	}

	return absPath, nil
}

// ValidateStatus checks if a status value is valid
func ValidateStatus(status string) bool {
	for _, v := range validStatuses {
		if v == status {
			return true
		}
	}
	return false
}

// ParseStatus reads the status field from an issue file's YAML frontmatter.
// Returns DefaultStatus ("todo") if status field is missing.
func ParseStatus(issuePath string, fs core.FS) (string, error) {
	content, err := fs.ReadFile(issuePath)
	if err != nil {
		return "", fmt.Errorf("failed to read issue file: %w", err)
	}

	status := extractStatusFromFrontmatter(string(content))
	if status == "" {
		return DefaultStatus, nil
	}

	if !ValidateStatus(status) {
		return "", fmt.Errorf("invalid status: %q (valid: %v)", status, validStatuses)
	}

	return status, nil
}

// UpdateStatus updates the status field in an issue file's YAML frontmatter.
// Preserves all other frontmatter fields and file content.
func UpdateStatus(issuePath string, status string, fs core.FS) error {
	if !ValidateStatus(status) {
		return fmt.Errorf("invalid status: %q (valid: %v)", status, validStatuses)
	}

	content, err := fs.ReadFile(issuePath)
	if err != nil {
		return fmt.Errorf("failed to read issue file: %w", err)
	}

	text := string(content)
	updated, err := updateStatusInFrontmatter(text, status)
	if err != nil {
		return err
	}

	if err := fs.WriteFile(issuePath, []byte(updated), DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write issue file: %w", err)
	}

	return nil
}

// extractStatusFromFrontmatter extracts the status from YAML frontmatter.
func extractStatusFromFrontmatter(text string) string {
	frontmatter, _ := splitFrontmatter(text)
	if frontmatter == "" {
		return ""
	}

	for _, line := range strings.Split(frontmatter, "\n") {
		matches := statusRegex.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) > 1 {
			status := strings.TrimSpace(matches[1])
			status = strings.Trim(status, `"'`)
			return status
		}
	}

	return ""
}

// updateStatusInFrontmatter updates or adds status field in frontmatter.
func updateStatusInFrontmatter(text, status string) (string, error) {
	frontmatter, rest := splitFrontmatter(text)

	if frontmatter == "" {
		// No frontmatter - add it
		return fmt.Sprintf("---\nstatus: %s\n---\n%s", status, text), nil
	}

	// Check if status field exists
	lines := strings.Split(frontmatter, "\n")
	found := false
	for i, line := range lines {
		if statusRegex.MatchString(strings.TrimSpace(line)) {
			lines[i] = fmt.Sprintf("status: %s", status)
			found = true
			break
		}
	}

	if !found {
		// Add status field after first line
		if len(lines) > 0 {
			newLines := make([]string, 0, len(lines)+1)
			newLines = append(newLines, lines[0])
			newLines = append(newLines, fmt.Sprintf("status: %s", status))
			newLines = append(newLines, lines[1:]...)
			lines = newLines
		} else {
			lines = append(lines, fmt.Sprintf("status: %s", status))
		}
	}

	return "---\n" + strings.Join(lines, "\n") + "\n---" + rest, nil
}

// splitFrontmatter splits text into frontmatter content and remaining text.
// Returns ("", text) if no frontmatter found.
func splitFrontmatter(text string) (frontmatter, rest string) {
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", text
	}

	lines := strings.Split(text, "\n")
	if len(lines) < 2 {
		return "", text
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return "", text
	}

	frontmatter = strings.Join(lines[1:endIdx], "\n")
	rest = "\n" + strings.Join(lines[endIdx+1:], "\n")
	return frontmatter, rest
}
