package cli

import (
	"encoding/json"
	"fmt"
)

// PrintJSON marshals data to indented JSON and prints to stdout.
func PrintJSON(data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}
