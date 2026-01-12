package issue

import (
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/input"
)

// fields defines all input fields - single source of truth for validation + schema
var fields = []input.Field{
	{
		Name:        "title",
		Description: "Issue title",
		Required:    true,
	},
	{
		Name:        "description",
		Description: "Issue description",
		Required:    false,
		Default:     "",
	},
}

// Input holds validated input for issue create
type Input struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Schema returns the JSON schema with defaults for issue create
func Schema() ([]byte, error) {
	return input.GenerateSchema(fields)
}

// Fields returns field definitions for TUI generation
func Fields() []input.Field {
	return fields
}

// GetDefaults returns default values for all fields.
func GetDefaults() map[string]string {
	return input.GetDefaults(fields)
}

// Validate validates input and returns errors for invalid fields
func Validate(in Input) error {
	var errs []string

	title := strings.TrimSpace(in.Title)
	if title == "" {
		errs = append(errs, "title is required")
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %v", errs)
	}
	return nil
}

// WithDefaults returns input with defaults applied and whitespace trimmed
func WithDefaults(in Input) Input {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	return in
}

// ParseJSON parses JSON input into Input struct
func ParseJSON(data []byte) (Input, error) {
	return input.ParseJSON[Input](data)
}

// ListInput holds input for issue list command
type ListInput struct {
	Status []string `json:"status,omitempty"`
}

// ListSchema returns the JSON schema for issue list input
func ListSchema() ([]byte, error) {
	return input.GenerateSchema([]input.Field{
		{Name: "status", Description: "Filter by status", Default: ""},
	})
}

// ParseListJSON parses JSON input into ListInput
func ParseListJSON(data []byte) (ListInput, error) {
	return input.ParseJSON[ListInput](data)
}

// ValidStatuses returns valid status values for filtering
func ValidStatuses() []string {
	return []string{"todo", "in-progress", "done"}
}

// ValidateListInput validates the list input
func ValidateListInput(in ListInput) error {
	valid := ValidStatuses()
	for _, s := range in.Status {
		found := false
		for _, v := range valid {
			if s == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid status %q (valid: %v)", s, valid)
		}
	}
	return nil
}

// SearchSchema returns the JSON schema for issue search input
func SearchSchema() ([]byte, error) {
	return input.GenerateSchema([]input.Field{
		{Name: "query", Description: "Search query (fuzzy match)", Default: ""},
		{Name: "status", Description: "Filter by status", Default: ""},
		{Name: "limit", Description: "Max results (default 100)", Default: "100"},
	})
}

// ParseSearchJSON parses JSON input into SearchInput
func ParseSearchJSON(data []byte) (SearchInput, error) {
	return input.ParseJSON[SearchInput](data)
}

// ValidateSearchInput validates the search input
func ValidateSearchInput(in SearchInput) error {
	valid := ValidStatuses()
	for _, s := range in.Status {
		found := false
		for _, v := range valid {
			if s == v {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid status %q (valid: %v)", s, valid)
		}
	}
	return nil
}
