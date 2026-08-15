package claude

import "encoding/json"

// Input for skill creation (currently empty, for future options)
type Input struct{}

// Schema returns an example input document for the skill command
func Schema() ([]byte, error) {
	schema := map[string]any{}
	return json.MarshalIndent(schema, "", "  ")
}
