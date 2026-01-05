---
title: Add Provider interface to PR package
status: todo
priority: high
---

# Add Provider interface to PR package

## Problem

The `pr` package has no Provider interface like the `issue` package. It directly depends on concrete `adapters.GitHub` implementation, making it impossible to swap PR backends.

## Impact

- Cannot support GitLab, Gitea, Bitbucket, etc.
- Architectural inconsistency with issue package
- Harder to test with mocks

## Location

- `internal/core/pr/handler.go`

## Fix

Create a Provider interface similar to issue package:

```go
// provider.go
type Provider interface {
    Create(input CreateInput) (PR, error)
    Get(id string) (PR, error)
    GetMergeStatus(id string) (bool, error)
}

type GitHubProvider struct { ... }
type GitLabProvider struct { ... }
```

## Considerations

- Mirror issue package pattern
- Update handler to use interface
- Add provider selection based on config
