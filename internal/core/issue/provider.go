package issue

// Issue is a minimal reference to an issue in an external tracker.
// mp does not model status taxonomies — labels / states / workflows are the
// tracker's concern (and configurable per workflow via hooks). All mp needs
// from a tracker is: title (to derive piece names), an ID/number (to thread
// into hook env), and whether the issue is still open.
type Issue struct {
	ID     string `json:"id"`     // Provider-specific identifier
	Number string `json:"number"` // Human-readable issue number (e.g. "#123" or "ABC-456")
	Title  string `json:"title"`  // Issue title
	URL    string `json:"url"`    // Web URL for humans
	Open   bool   `json:"open"`   // True if still actionable; false if closed/merged/cancelled
}

// Provider resolves an opaque issue identifier to a minimal Issue.
// Listing, searching, and CRUD are deliberately not part of the interface —
// users run their tracker's own CLI (`glab issue`, `linear`, `plane`, etc.) for that.
type Provider interface {
	// Get resolves an opaque identifier (provider-specific format) to an Issue.
	Get(id string) (Issue, error)
}
