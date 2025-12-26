# Contributing

## Development Setup

### Prerequisites

- Go 1.24+
- Git

### Clone and build

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
go build -o mp .
```

### Run tests

```bash
go test ./...
```

### Lint

```bash
go vet ./...
```

## Adding a New Command

### 1. Create input definition

`internal/core/<cmd>/input.go`:

```go
package cmdname

type Input struct {
    Name   string `json:"name"`
    Option string `json:"option"`
}

type Field struct {
    Name        string
    Description string
    Required    bool
    Default     string
    ValidValues []string
}

var fields = []Field{
    {Name: "name", Required: true},
    {Name: "option", Default: "default_value", ValidValues: []string{"a", "b"}},
}

func Validate(input Input) error {
    // Validate using fields
}

func Schema(workDir string) ([]byte, error) {
    // Generate JSON schema from fields
}

func WithDefaults(input Input, workDir string) Input {
    // Apply defaults from fields
}

func ParseJSON(data []byte) (Input, error) {
    // Parse JSON into Input
}
```

### 2. Create handler

`internal/core/<cmd>/handler.go`:

```go
package cmdname

import "monkeypuzzle/internal/core"

type Handler struct {
    deps core.Deps
}

func NewHandler(deps core.Deps) *Handler {
    return &Handler{deps: deps}
}

func (h *Handler) Run(input Input) error {
    // Business logic here
    // Use h.deps.FS, h.deps.Output, h.deps.Exec

    h.deps.Output.Write(core.Message{
        Type:    core.MsgSuccess,
        Content: "Command completed",
    })
    return nil
}
```

### 3. Create tests

`internal/core/<cmd>/handler_test.go`:

```go
package cmdname_test

import (
    "testing"
    "monkeypuzzle/internal/adapters"
    "monkeypuzzle/internal/core"
    cmdname "monkeypuzzle/internal/core/<cmd>"
)

func TestHandlerRun(t *testing.T) {
    deps := core.Deps{
        FS:     adapters.NewMemoryFS(),
        Output: adapters.NewBufferOutput(),
        Exec:   adapters.NewMockExec(),
    }

    handler := cmdname.NewHandler(deps)
    input := cmdname.Input{Name: "test"}

    err := handler.Run(input)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Assert on output
    out := deps.Output.(*adapters.BufferOutput)
    if !out.HasSuccess() {
        t.Error("expected success message")
    }
}
```

### 4. Wire CLI command

`cmd/mp/<cmd>.go`:

```go
package main

import (
    "os"
    "github.com/spf13/cobra"
    "monkeypuzzle/internal/adapters"
    "monkeypuzzle/internal/core"
    cmdname "monkeypuzzle/internal/core/<cmd>"
)

var cmdCmd = &cobra.Command{
    Use:   "cmdname",
    Short: "Description",
    RunE:  runCmd,
}

func init() {
    rootCmd.AddCommand(cmdCmd)
    cmdCmd.Flags().StringVar(&flagName, "name", "", "Name")
}

func runCmd(cmd *cobra.Command, args []string) error {
    deps := core.Deps{
        FS:     adapters.NewOSFS(""),
        Output: adapters.NewTextOutput(os.Stderr),
        Exec:   adapters.NewOSExec(),
    }

    input, err := getInput()
    if err != nil {
        return err
    }

    handler := cmdname.NewHandler(deps)
    return handler.Run(input)
}
```

## Adding a Provider

Providers are defined in field ValidValues:

```go
// internal/core/init/input.go
{
    Name:        "issue_provider",
    ValidValues: []string{"markdown", "linear"},  // Add new provider
}
```

Provider-specific logic goes in the handler.

## Code Style

- Keep functions small and focused
- Use table-driven tests
- No error swallowing - propagate or handle explicitly
- Prefer composition over inheritance
- Use dependency injection for external dependencies

## Testing Guidelines

- All business logic must be testable with mocks
- Use `adapters.MemoryFS` for filesystem tests
- Use `adapters.BufferOutput` for output assertions
- Use `adapters.MockExec` for command execution tests
- Table-driven tests for validation logic

## Pull Request Process

1. Fork the repository
2. Create feature branch
3. Write tests for new functionality
4. Ensure all tests pass: `go test ./...`
5. Ensure linting passes: `go vet ./...`
6. Submit pull request

## Project Structure Reference

See [architecture.md](architecture.md) for detailed architecture documentation.
