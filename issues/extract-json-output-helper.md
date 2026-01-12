---
title: Extract JSON output helper function
status: done
priority: low
---

# Extract JSON output helper function

## Problem

11 instances of nearly identical JSON marshaling code:

```go
jsonData, err := json.MarshalIndent(result, "", "  ")
if err != nil {
    return fmt.Errorf("failed to marshal result: %w", err)
}
fmt.Println(string(jsonData))
```

## Locations

- piece.go: lines 342, 476, 540, 627, 699, 801, 946
- init.go: line 118
- issue.go: lines 117, 220
- pr.go: line 85

## Fix

Extract to helper in `pkg/cli`:

```go
// pkg/cli/output.go
func PrintJSON(data interface{}) error {
    jsonData, err := json.MarshalIndent(data, "", "  ")
    if err != nil {
        return fmt.Errorf("failed to marshal JSON: %w", err)
    }
    fmt.Println(string(jsonData))
    return nil
}
```

Then replace all 11 instances with `cli.PrintJSON(result)`.
