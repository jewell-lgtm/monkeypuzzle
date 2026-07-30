package core

import (
	"context"
	"io/fs"
	"net/http"
	"os"
	"sync"
)

// FS abstracts filesystem operations for testability
type FS interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	ReadFile(name string) ([]byte, error)
	Stat(name string) (fs.FileInfo, error)
	Remove(name string) error
	Symlink(oldname, newname string) error
	ReadDir(name string) ([]fs.DirEntry, error)
	// Rename atomically replaces newpath with oldpath (same-directory renames
	// only, which is all callers need for atomic write-temp-then-rename).
	Rename(oldpath, newpath string) error
}

// FileLocker is an optional FS extension providing advisory exclusive
// locking, used to serialize read-modify-write cycles on shared state files
// (concurrent `mp agent report` hooks). Callers type-assert; absence (e.g.
// MemoryFS in tests) means no locking.
type FileLocker interface {
	// LockFile blocks until the exclusive lock on path is held and returns
	// the unlock function.
	LockFile(path string) (unlock func(), err error)
}

// MessageType categorizes output messages
type MessageType int

const (
	MsgInfo MessageType = iota
	MsgSuccess
	MsgWarning
	MsgError
)

// Message represents a structured output message
type Message struct {
	Type    MessageType
	Content string
	Data    any // optional structured data for JSON output
}

// Output abstracts how messages are presented to the user
type Output interface {
	Write(msg Message)
}

// Exec abstracts command execution for testability
type Exec interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	RunWithDir(ctx context.Context, dir, name string, args ...string) ([]byte, error)
	RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error)
	// StartDetached launches a command in its own process group, redirects its
	// combined stdout/stderr to logPath, and returns once the process has
	// started without waiting for it to finish. The process is deliberately not
	// tied to a context: it is fire-and-forget and survives the caller exiting.
	// An error is returned only if the process fails to start.
	StartDetached(dir string, env []string, logPath, name string, args ...string) error
}

// HTTPClient abstracts HTTP requests for testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Multiplexer abstracts terminal multiplexer operations (currently tmux + a no-op).
// Maybe one day this grows zellij / wezterm / kitty implementations.
type Multiplexer interface {
	// SwitchTo switches to (or creates) a session for the given piece
	SwitchTo(ctx context.Context, sessionName, workDir string) error

	// Kill terminates a session
	Kill(ctx context.Context, sessionName string) error

	// Exists checks if session is running
	Exists(ctx context.Context, sessionName string) bool

	// InSession returns true if inside a managed session
	InSession() bool

	// IsInstalled returns true if multiplexer is available
	IsInstalled(ctx context.Context) bool

	// Name returns the provider name ("tmux", "none", ...) for labels and
	// provider-specific gating.
	Name() string
}

// PaneInfo describes one pane in a multiplexer session.
type PaneInfo struct {
	// ID is the pane target (tmux: "%12").
	ID string
	// Command is the pane's foreground process name (tmux:
	// pane_current_command), how agent processes are recognized.
	Command string
	// PID is the pane's root process id.
	PID int
}

// PaneOps is the optional pane-level extension of Multiplexer. Callers
// type-assert into it and treat a failed assertion as "unsupported by this
// multiplexer" — only tmux implements it today.
type PaneOps interface {
	// SendText types text into a pane (or a session's active pane) followed by
	// Enter, as if the user had typed it — the way to hand a prompt to an
	// agent TUI that owns the pane's stdin.
	SendText(ctx context.Context, target, text string) error

	// CapturePane returns the current visible contents of a pane (or a
	// session's active pane).
	CapturePane(ctx context.Context, target string) ([]byte, error)

	// ListPanes enumerates every pane in a session — the discovery half of
	// zero-install agent detection.
	ListPanes(ctx context.Context, sessionName string) ([]PaneInfo, error)
}

// LoadingSignal provides pub/sub for loading state.
// Handlers publish loading state changes, UI layers subscribe to display feedback.
type LoadingSignal struct {
	mu   sync.RWMutex
	subs []func(active bool, label string)
}

// NewLoadingSignal creates a new loading signal pub/sub.
func NewLoadingSignal() *LoadingSignal {
	return &LoadingSignal{subs: make([]func(active bool, label string), 0)}
}

// Sub registers a subscriber to receive loading state changes.
func (l *LoadingSignal) Sub(fn func(active bool, label string)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.subs = append(l.subs, fn)
}

// Pub publishes a loading state change to all subscribers.
func (l *LoadingSignal) Pub(active bool, label string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, fn := range l.subs {
		fn(active, label)
	}
}

// Deps holds all injectable dependencies for handlers
type Deps struct {
	FS      FS
	Output  Output
	Exec    Exec
	HTTP    HTTPClient
	Loading *LoadingSignal
}

// NewDeps creates Deps with all required dependencies.
// Using this constructor ensures the compiler catches missing dependencies.
func NewDeps(fs FS, output Output, exec Exec, http HTTPClient, loading *LoadingSignal) Deps {
	return Deps{FS: fs, Output: output, Exec: exec, HTTP: http, Loading: loading}
}
