# Contributing

## Development Setup

### Prerequisites

- Go 1.24+ (use [mise](https://mise.jdx.dev/) for version management)
- Git
- tmux (for the piece commands)
- gh CLI (for GitHub PR provider)

### Clone and build

```bash
# Install Go via mise (recommended)
mise install
```

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
go build -o mp .
```

### Run tests

```bash
go test ./...                           # Unit tests only
go test -tags=integration ./...         # All tests including integration
go test ./internal/core/piece/... -v    # Specific package, verbose
```

### Lint

```bash
go vet ./...
```

---

## Testing Philosophy: Outside-In

**This is the most important section of this document.**

Monkeypuzzle uses **outside-in testing** (also called London-school TDD). The core principle:

> **Every new feature starts with an integration test that proves the happy path works. Unit tests fill in edge cases and completeness.**

### Why Outside-In?

1. **Integration tests catch real bugs** - unit tests with mocks can pass while the system is broken
2. **Happy path first** - proves the feature works end-to-end before worrying about edge cases
3. **Unit tests are for completeness** - edge cases, error handling, boundary conditions
4. **Faster feedback on architecture** - integration tests reveal integration problems early

### The Testing Pyramid (Inverted)

Traditional wisdom says "many unit tests, few integration tests." We flip this for *starting* new work:

```
START HERE
    │
    ▼
┌─────────────────────────────────┐
│   Integration Test (Happy Path) │  ← Write this FIRST
└─────────────────────────────────┘
           │
           ▼
┌─────────────────────────────────┐
│   Unit Tests (Edge Cases)       │  ← Fill these in after
└─────────────────────────────────┘
```

### Workflow for New Features

1. **Write one integration test** that exercises the happy path
2. **Make it pass** with the simplest implementation
3. **Add unit tests** for edge cases, error conditions, boundary conditions
4. **Refactor** with confidence (tests catch regressions)

### Example: Adding a New Command

**Step 1: Integration test first** (`cmd/mp/cli_integration_test.go`)

```go
//go:build integration

func TestCLI_NewCommand_HappyPath(t *testing.T) {
    env := setupTestEnv(t)
    defer env.cleanup()

    env.initProject("test")

    // Exercise the REAL binary, REAL filesystem
    stdout, _, err := env.run("newcommand", "--flag", "value")
    if err != nil {
        t.Fatalf("command failed: %v", err)
    }

    // Verify real output
    var result map[string]any
    json.Unmarshal([]byte(stdout), &result)
    if result["expected_field"] != "expected_value" {
        t.Error("happy path broken")
    }
}
```

**Step 2: Make it pass** - implement the feature

**Step 3: Unit tests for edge cases** (`internal/core/newcmd/handler_test.go`)

```go
func TestHandler_InvalidInput(t *testing.T) {
    deps := core.Deps{
        FS:     adapters.NewMemoryFS(),      // Mock filesystem
        Output: adapters.NewBufferOutput(),   // Mock output
        Exec:   adapters.NewMockExec(),       // Mock commands
    }

    handler := newcmd.NewHandler(deps)
    err := handler.Run(newcmd.Input{Name: ""})

    if err == nil {
        t.Error("expected error for empty name")
    }
}

func TestHandler_FileAlreadyExists(t *testing.T) {
    fs := adapters.NewMemoryFS()
    fs.WriteFile("/existing", []byte("content"), 0644)

    deps := core.Deps{FS: fs, Output: adapters.NewBufferOutput()}
    handler := newcmd.NewHandler(deps)

    err := handler.Run(newcmd.Input{Path: "/existing"})
    if err == nil {
        t.Error("expected error for existing file")
    }
}
```

### Test File Organization

```
internal/core/piece/
├── handler.go
├── handler_test.go              # Unit tests (mocked deps)
├── handler_integration_test.go  # Integration tests (real deps)
├── input.go
└── input_test.go                # Unit tests for validation
```

### Integration vs Unit Tests

| Aspect | Integration Test | Unit Test |
|--------|-----------------|-----------|
| **Purpose** | Prove the feature works | Prove edge cases handled |
| **Dependencies** | Real (filesystem, git, etc.) | Mocked |
| **Build tag** | `//go:build integration` | None |
| **Speed** | Slower | Fast |
| **When to write** | FIRST | After happy path works |
| **File suffix** | `_integration_test.go` | `_test.go` |

### Running Tests

```bash
# Fast feedback during development (unit tests only)
go test ./...

# Full test suite (CI, before PR)
go test -tags=integration ./...

# Specific integration tests
go test -tags=integration ./internal/core/piece/... -v -run TestIntegration
```

### What Makes a Good Integration Test?

1. **Tests the real thing** - actual binary, filesystem, git commands
2. **Minimal setup** - use helper functions like `setupTestEnv`, `setupGitRepo`
3. **Focuses on happy path** - save edge cases for unit tests
4. **Self-contained** - creates temp dirs, cleans up after itself
5. **Fast enough** - don't test every combination, just prove it works

### What Makes a Good Unit Test?

1. **Tests one thing** - single behavior or edge case
2. **Uses mocks** - `MemoryFS`, `BufferOutput`, `MockExec`
3. **Table-driven** for multiple cases:

```go
func TestValidate(t *testing.T) {
    tests := []struct {
        name    string
        input   Input
        wantErr bool
    }{
        {"valid", Input{Name: "test"}, false},
        {"empty name", Input{Name: ""}, true},
        {"invalid chars", Input{Name: "a/b"}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := Validate(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
            }
        })
    }
}
```

---

## Adding a New Command

### 1. Write integration test first

`cmd/mp/cli_integration_test.go`:

```go
//go:build integration

func TestCLI_NewCmd_HappyPath(t *testing.T) {
    env := setupTestEnv(t)
    defer env.cleanup()
    env.initProject("test")

    stdout, _, err := env.run("newcmd", "--name", "test")
    if err != nil {
        t.Fatalf("failed: %v", err)
    }
    // Assert on real output
}
```

### 2. Create input definition

`internal/core/newcmd/input.go`:

```go
package newcmd

type Input struct {
    Name   string `json:"name"`
    Option string `json:"option"`
}

var fields = []Field{
    {Name: "name", Required: true},
    {Name: "option", Default: "default_value", ValidValues: []string{"a", "b"}},
}

func Validate(input Input) error { /* use fields */ }
func Schema(workDir string) ([]byte, error) { /* generate from fields */ }
func WithDefaults(input Input, workDir string) Input { /* apply defaults */ }
```

### 3. Create handler

`internal/core/newcmd/handler.go`:

```go
package newcmd

import "github.com/jewell-lgtm/monkeypuzzle/internal/core"

type Handler struct {
    deps core.Deps
}

func NewHandler(deps core.Deps) *Handler {
    return &Handler{deps: deps}
}

func (h *Handler) Run(input Input) error {
    // Business logic using h.deps.FS, h.deps.Output, h.deps.Exec
    return nil
}
```

### 4. Add unit tests for edge cases

`internal/core/newcmd/handler_test.go`:

```go
package newcmd_test

func TestHandler_EmptyName(t *testing.T) {
    deps := core.Deps{
        FS:     adapters.NewMemoryFS(),
        Output: adapters.NewBufferOutput(),
    }
    handler := newcmd.NewHandler(deps)

    err := handler.Run(newcmd.Input{Name: ""})
    if err == nil {
        t.Error("expected error")
    }
}
```

### 5. Wire CLI command

`cmd/mp/newcmd.go`:

```go
package main

var newcmdCmd = &cobra.Command{
    Use:   "newcmd",
    Short: "Description",
    RunE:  runNewCmd,
}

func init() {
    rootCmd.AddCommand(newcmdCmd)
    newcmdCmd.Flags().StringVar(&flagName, "name", "", "Name")
}

func runNewCmd(cmd *cobra.Command, args []string) error {
    deps := core.Deps{
        FS:     adapters.NewOSFS(""),
        Output: adapters.NewTextOutput(os.Stderr),
        Exec:   adapters.NewOSExec(),
    }
    // Handle input modes, run handler
    return nil
}
```

---

## Code Style

- Keep functions small and focused
- Use table-driven tests
- No error swallowing - propagate or handle explicitly
- Prefer composition over inheritance
- Use dependency injection for external dependencies

---

## Pull Request Process

1. Fork the repository
2. Create feature branch
3. **Write integration test for happy path first**
4. Implement the feature
5. Add unit tests for edge cases
6. Ensure all tests pass: `go test -tags=integration ./...`
7. Ensure linting passes: `go vet ./...`
8. Submit pull request

---

## Project Structure Reference

See [architecture.md](architecture.md) for detailed architecture documentation.
