# Code Review Guide

## Philosophy

Code review exists to:
1. Catch bugs before they hit main
2. Share knowledge across the team
3. Maintain code quality and consistency

**Reviews should be kind, constructive, and focused on the code, not the person.**

---

## Review Checklist

### 1. Tests First

**This is the most important check.**

- [ ] Does the PR include an **integration test for the happy path**?
- [ ] Are edge cases covered by unit tests?
- [ ] Do tests actually test the thing? (not just "test exists")

```
If no integration test for new functionality → Request changes
```

Integration tests prove the feature works. Unit tests prove edge cases are handled. Both are needed, but **integration tests come first**.

See [contributing.md](contributing.md) for the outside-in testing philosophy.

### 2. Does It Work?

- [ ] Pull the branch and run `go test -tags=integration ./...`
- [ ] Try the feature manually if it has user-facing changes
- [ ] Check for obvious logic errors

### 3. Error Handling

- [ ] Are errors propagated or handled? (never swallowed)
- [ ] Do error messages help debugging?
- [ ] Are edge cases handled gracefully?

**Bad:**
```go
result, _ := someFunction()  // Error ignored
```

**Good:**
```go
result, err := someFunction()
if err != nil {
    return fmt.Errorf("failed to do thing: %w", err)
}
```

### 4. Code Style

- [ ] Functions are small and focused
- [ ] Names are clear and descriptive
- [ ] No dead code or commented-out code
- [ ] Consistent with existing codebase style

### 5. Architecture

- [ ] Does it follow the existing patterns? (ports/adapters, dependency injection)
- [ ] Is business logic in `internal/core/`, not in `cmd/`?
- [ ] Are external dependencies injected, not hardcoded?

### 6. Security

- [ ] No secrets in code
- [ ] User input validated at boundaries
- [ ] File paths sanitized

---

## Integration Test Review

When reviewing integration tests, check:

1. **Uses real dependencies** - `adapters.NewOSFS("")`, `adapters.NewOSExec()`, actual git commands
2. **Tests the actual binary** for CLI integration tests
3. **Creates isolated test environment** - temp directories, cleanup
4. **Has `//go:build integration` tag**
5. **Focuses on happy path** - proves the feature works end-to-end

**Good integration test:**
```go
//go:build integration

func TestIntegration_NewFeature_HappyPath(t *testing.T) {
    // Setup: real filesystem, real git
    tmpDir, _ := os.MkdirTemp("", "test-*")
    defer os.RemoveAll(tmpDir)
    setupGitRepo(t, tmpDir)

    // Execute: real handler with real deps
    deps := core.Deps{
        FS:   adapters.NewOSFS(""),
        Exec: adapters.NewOSExec(),
    }
    handler := feature.NewHandler(deps)
    err := handler.Run(feature.Input{Name: "test"})

    // Assert: real results
    if err != nil {
        t.Fatalf("expected success: %v", err)
    }
    // Check real filesystem state...
}
```

**What to flag:**
- Integration test uses mocks → Should use real deps
- No happy path test → Request one
- Only unit tests for new feature → Request integration test

---

## Unit Test Review

When reviewing unit tests, check:

1. **Uses mocks** - `adapters.NewMemoryFS()`, `adapters.NewBufferOutput()`, `adapters.NewMockExec()`
2. **Tests edge cases** - empty input, invalid input, error conditions
3. **Table-driven** for multiple similar cases
4. **No integration tag** (runs with `go test ./...`)

**Good unit test:**
```go
func TestValidate_EdgeCases(t *testing.T) {
    tests := []struct {
        name    string
        input   Input
        wantErr bool
    }{
        {"empty name", Input{Name: ""}, true},
        {"spaces only", Input{Name: "   "}, true},
        {"valid", Input{Name: "test"}, false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("got err=%v, want err=%v", err, tt.wantErr)
            }
        })
    }
}
```

**What to flag:**
- Unit test uses real filesystem → Should use MemoryFS
- Not table-driven when testing multiple similar cases
- Tests happy path only (that's what integration tests are for)

---

## Common Review Comments

### Missing Integration Test

> This PR adds new functionality but I don't see an integration test for the happy path. Could you add one? See [contributing.md](contributing.md) for examples.

### Error Swallowed

> This error is being ignored. Should it be propagated or logged?

### Business Logic in CLI Layer

> This logic looks like it belongs in `internal/core/` rather than `cmd/`. The CLI layer should just wire things together.

### Hardcoded Dependency

> This uses `os.Open` directly. Should it go through the `FS` interface so it's testable?

### Complex Function

> This function is doing a lot. Could it be broken into smaller functions?

### Missing Edge Case

> What happens if `name` is empty? Could you add a unit test for that?

---

## Approving PRs

Before approving, verify:

1. Integration test exists for happy path
2. `go test -tags=integration ./...` passes
3. `go vet ./...` passes
4. Code follows existing patterns
5. No obvious bugs or security issues

**Don't approve without integration test for new functionality.**

---

## Requesting Changes

Be specific and constructive:

**Bad:** "Tests are wrong"

**Good:** "This test uses mocks but should be an integration test since it's testing the happy path. Could you convert it to use real dependencies? Here's an example: [link]"

---

## Time Expectations

- Small PRs (< 100 lines): Same day
- Medium PRs (100-500 lines): 1-2 days
- Large PRs (> 500 lines): Consider breaking up

If a PR is too large to review effectively, ask for it to be split.
