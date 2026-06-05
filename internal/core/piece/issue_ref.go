package piece

// IssueRef is a provider-agnostic issue reference.
// Stored in piece markers to track which issue a piece is working on.
type IssueRef struct {
	Provider string `json:"provider"`         // "markdown", "linear", etc.
	ID       string `json:"id"`               // Provider-specific ID
	Number   string `json:"number,omitempty"` // Human-readable (e.g., "ABC-123")
	Title    string `json:"title"`            // Cached display title
}

// IsEmpty returns true if this ref has no data
func (r IssueRef) IsEmpty() bool {
	return r.Provider == "" && r.ID == ""
}

// DisplayName returns the best available display name for the issue
func (r IssueRef) DisplayName() string {
	if r.Number != "" {
		return r.Number + ": " + r.Title
	}
	return r.Title
}
