package issue

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Status values
const (
	StatusTodo       = "todo"
	StatusInProgress = "in-progress"
	StatusDone       = "done"
)

var validStatuses = []string{StatusTodo, StatusInProgress, StatusDone}

var statusRegex = regexp.MustCompile(`(?i)^status:\s*(.+)$`)

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
		// Add status field after first line (which could be title)
		// Insert at position 1 if we have lines, or append
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
