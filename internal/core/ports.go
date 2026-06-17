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
