# Architecture

Monkeypuzzle uses clean architecture with dependency injection for testability.

## Directory Structure

```
monkeypuzzle/
├── apps/mp/              # CLI wiring (Cobra commands)
│   ├── root.go          # Root command
│   ├── init.go          # mp init command
│   └── piece.go         # piece commands (create, status, list, merge, …)
├── internal/
│   ├── core/            # Business logic + interfaces
│   │   ├── ports.go     # Interface definitions
│   │   ├── init/        # Init command logic
│   │   │   ├── input.go     # Input struct, validation, schema
│   │   │   ├── handler.go   # Business logic
│   │   │   └── handler_test.go
│   │   └── piece/       # Piece command logic
│   │       ├── input.go
│   │       ├── handler.go
│   │       ├── handler_test.go
│   │       ├── hooks.go     # Hook runner for piece operations
│   │       └── placements.go # Controller-side links to pieces placed on boxes
│   ├── adapters/        # Interface implementations
│   │   ├── filesystem.go   # OSFS, MemoryFS
│   │   ├── output.go       # TextOutput, JSONOutput, BufferOutput
│   │   ├── exec.go         # OSExec, MockExec
│   │   ├── git.go          # Git operations
│   │   └── tmux.go         # Tmux operations
│   └── tui/             # Bubble Tea UI
│       └── init/        # Interactive init wizard
└── pkg/styles/          # TUI styling
```

## Project state on disk

Everything mp keeps for a repo lives under its monkeypuzzle directory
(`.monkeypuzzle/` by default; relocatable via `mp config`, see
`internal/projectdir`):

| Path | What |
| --- | --- |
| `monkeypuzzle.json` | project config (name, PR provider, multiplexer, …) |
| `pieces/<name>/` | one git worktree per local piece — **every** directory here is treated as a worktree |
| `hooks/` | lifecycle hook scripts |
| `placements.json` | links to pieces placed on a box with `mp create --remote` — `{ "<piece>": {box, remote_path, remote_project, pending, cached} }`; read by `ListPieces` so placed pieces appear alongside local ones (with `host`/`state`), never as worktrees |
| `logs/` | hook and headless-agent logs |

## Core Concepts

### Ports (Interfaces)

Defined in `internal/core/ports.go`:

```go
type FS interface {
    MkdirAll(path string, perm os.FileMode) error
    WriteFile(name string, data []byte, perm os.FileMode) error
    ReadFile(name string) ([]byte, error)
    Stat(name string) (fs.FileInfo, error)
    Remove(name string) error
    Symlink(oldname, newname string) error
}

type Output interface {
    Write(msg Message)
}

type Exec interface {
    Run(name string, args ...string) ([]byte, error)
    RunWithDir(dir, name string, args ...string) ([]byte, error)
    RunWithEnv(dir string, env []string, name string, args ...string) ([]byte, error)
}
```

### Deps Struct

All dependencies bundled for injection:

```go
type Deps struct {
    FS     FS
    Output Output
    Exec   Exec
}
```

### Handlers

Business logic in handlers that receive Deps:

```go
type Handler struct {
    deps core.Deps
}

func NewHandler(deps core.Deps) *Handler {
    return &Handler{deps: deps}
}

func (h *Handler) Run(input Input) error {
    // Uses h.deps.FS, h.deps.Output, h.deps.Exec
}
```

## Adapters

### Filesystem

**OSFS** - Real filesystem:

```go
fs := adapters.NewOSFS("")  // Empty root = absolute paths
```

**MemoryFS** - In-memory for tests:

```go
fs := adapters.NewMemoryFS()
fs.Files()  // Returns map of all files
fs.Dirs()   // Returns slice of directories
```

### Output

**TextOutput** - Human-readable with prefixes:

```go
out := adapters.NewTextOutput(os.Stderr)
// Adds: ✓ (success), ⚠ (warning), ✗ (error)
```

**JSONOutput** - Machine-readable:

```go
out := adapters.NewJSONOutput(os.Stdout)
// Outputs structured JSON
```

**BufferOutput** - For testing:

```go
out := adapters.NewBufferOutput()
out.HasSuccess()  // Check if success message exists
out.Last()        // Get last message
```

### Exec

**OSExec** - Real command execution:

```go
exec := adapters.NewOSExec()
```

**MockExec** - For testing:

```go
mock := adapters.NewMockExec()
mock.AddResponse("git", []string{"status"}, []byte("output"), nil)
mock.WasCalled("git", "status")
mock.GetCalls()
```

### Composed Adapters

**Git** - Uses Exec internally:

```go
git := adapters.NewGit(deps.Exec)
git.WorktreeAdd(repoRoot, worktreePath)
git.CurrentBranch(workDir)
git.Merge(workDir, branch)
```

**Tmux** - Uses Exec internally:

```go
tmux := adapters.NewTmux(deps.Exec)
tmux.NewSession(sessionName, workDir)
tmux.KillSession(sessionName)
```

**HookRunner** - Executes shell scripts with environment variables. It
strips inherited `MP_*` from the hook env, so the box-side identity of a
placed piece (`MP_PLACEMENT_HOST`, `MP_REMOTE`, exported by the controller's
ssh proxy) is read once in `NewHookRunner` and re-added to every hook. The
one controller-side hook, `on-box-connect.sh`, runs through the same runner
with the local repo as root:

```go
hooks := piece.NewHookRunner(deps)
hooks.RunHook(repoRoot, piece.HookOnPieceCreate, piece.HookContext{
    PieceName:    "my-piece",
    WorktreePath: "/path/to/worktree",
    RepoRoot:     "/path/to/repo",
})
```

## Input Pattern

Single source of truth for validation and schema:

```go
// internal/core/init/input.go
var fields = []Field{
    {
        Name:        "name",
        Description: "Project name",
        Required:    true,
        Default:     "",
    },
    {
        Name:        "pr_provider",
        ValidValues: []string{"github", "gitlab"},
        Default:     "github",
    },
}

// These functions use the same field definitions:
func Validate(input Input) error { ... }
func Schema(workDir string) ([]byte, error) { ... }
func WithDefaults(input Input, workDir string) Input { ... }
```

## Multi-Modal Input

CLI layer (`apps/mp/init.go`) handles mode detection:

```go
func getInput() (Input, error) {
    switch {
    case allFlagsProvided:
        return fromFlags()
    case hasStdinData():
        return initcmd.ParseJSON(stdinData)
    case isTerminal():
        return runInteractiveMode()
    default:
        return Input{}, errors.New("no input provided")
    }
}
```

## Data Flow

```
User Input (flags/JSON/TUI)
         ↓
    apps/mp/*.go (mode detection, adapter creation)
         ↓
    core.Deps{FS, Output, Exec}
         ↓
    core/<cmd>/handler.go (business logic)
         ↓
    Calls port methods (FS.WriteFile, Output.Write, etc.)
         ↓
    Adapter implementations execute
```

## Testing Strategy

All external dependencies mocked:

```go
func TestHandler(t *testing.T) {
    deps := core.Deps{
        FS:     adapters.NewMemoryFS(),
        Output: adapters.NewBufferOutput(),
        Exec:   adapters.NewMockExec(),
    }

    handler := initcmd.NewHandler(deps)
    err := handler.Run(input)

    // Assert on MemoryFS state
    // Assert on BufferOutput messages
    // Assert on MockExec calls
}
```

Benefits:

- No disk I/O
- No external command execution
- Deterministic results
- Fast tests
