---
title: Extract common input validation code
status: done
priority: low
---

# Extract common input validation code

## Problem

Both `issue/input.go` and `pr/input.go` have nearly identical code:

- Field struct definition
- fields array pattern
- Schema() function
- Fields() function
- GetDefaults() function
- ParseJSON() function (just type changes)
- WithDefaults() function (same pattern)

## Impact

- Code duplication
- Bug fixes need to be applied in multiple places
- Inconsistencies can creep in

## Locations

- `internal/core/issue/input.go:19-91`
- `internal/core/pr/input.go:18-85`

## Fix

Extract to shared package:

```go
// internal/core/input/input.go
package input

type Field struct {
    Name        string
    Description string
    Required    bool
    Default     string
    ValidValues []string
}

func GenerateSchema(fields []Field) ([]byte, error) { ... }
func GetDefaults(fields []Field) map[string]string { ... }
```

Then issue and pr packages use this shared code.
