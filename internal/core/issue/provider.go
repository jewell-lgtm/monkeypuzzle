package issue

// Issue represents an issue from any provider
type Issue struct {
	ID          string `json:"id"`          // Provider-specific identifier (path for markdown, number for GitHub)
	Title       string `json:"title"`       // Issue title
	Status      string `json:"status"`      // Status (todo, in-progress, done)
	Description string `json:"description"` // Issue description/body
}

// CreateInput holds input for creating an issue
type CreateInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Provider abstracts issue storage backends (markdown files, GitHub Issues, Linear, etc.)
type Provider interface {
	// Create creates a new issue and returns it
	Create(input CreateInput) (Issue, error)

	// List returns issues, optionally filtered by status
	List(statusFilter []string) ([]Issue, error)

	// Get returns an issue by ID
	Get(id string) (Issue, error)

	// UpdateStatus updates the status of an issue
	UpdateStatus(id string, status string) error
}
