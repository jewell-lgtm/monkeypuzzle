package issue

// Issue represents an issue from any provider
type Issue struct {
	ID          string `json:"id"`          // Provider-specific identifier (path for markdown, UUID for Linear)
	Number      string `json:"number"`      // Human-readable issue number (e.g., "ABC-123" for Linear)
	Title       string `json:"title"`       // Issue title
	Description string `json:"description"` // Issue description/body
}

// CreateInput holds input for creating an issue
type CreateInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// SearchInput holds input for searching issues
type SearchInput struct {
	Query string `json:"query"` // Text search query (empty = all)
	Limit int    `json:"limit"` // Max results (0 = default 100)
}

// DefaultSearchLimit is the default max results for SearchIssues
const DefaultSearchLimit = 100

// Provider abstracts issue storage backends (markdown files, GitHub Issues, Linear, etc.)
type Provider interface {
	// Create creates a new issue and returns it
	Create(input CreateInput) (Issue, error)

	// List returns all issues.
	// Deprecated: Use SearchIssues instead
	List() ([]Issue, error)

	// SearchIssues returns issues matching search criteria, sorted by recency
	SearchIssues(input SearchInput) ([]Issue, error)

	// Get returns an issue by ID
	Get(id string) (Issue, error)
}
