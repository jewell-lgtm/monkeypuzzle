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
}

// HTTPClient abstracts HTTP requests for testability
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Multiplexer abstracts terminal multiplexer operations (tmux, zellij, etc.)
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

// IssueSyncEvent represents a status change that should be synced to a provider.
type IssueSyncEvent struct {
	Provider     string // Issue provider type (markdown, linear, etc.)
	IssueID      string // Provider-specific issue ID
	NewStatus    string // New status to sync (in-progress, done, etc.)
	PieceName    string // Name of the piece that triggered the sync
	WorktreePath string // Path to the piece worktree (for updating marker dirty flag)
}

// IssueSyncSignal provides pub/sub for issue status sync events.
// Piece operations publish events, sync subscribers persist changes to providers.
type IssueSyncSignal struct {
	mu   sync.RWMutex
	subs []func(event IssueSyncEvent)
}

// NewIssueSyncSignal creates a new issue sync signal pub/sub.
func NewIssueSyncSignal() *IssueSyncSignal {
	return &IssueSyncSignal{subs: make([]func(event IssueSyncEvent), 0)}
}

// Sub registers a subscriber to receive issue sync events.
func (s *IssueSyncSignal) Sub(fn func(event IssueSyncEvent)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, fn)
}

// Pub publishes an issue sync event to all subscribers.
func (s *IssueSyncSignal) Pub(event IssueSyncEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, fn := range s.subs {
		fn(event)
	}
}

// Deps holds all injectable dependencies for handlers
type Deps struct {
	FS        FS
	Output    Output
	Exec      Exec
	HTTP      HTTPClient
	Loading   *LoadingSignal
	IssueSync *IssueSyncSignal
}

// NewDeps creates Deps with all required dependencies.
// Using this constructor ensures the compiler catches missing dependencies.
func NewDeps(fs FS, output Output, exec Exec, http HTTPClient, loading *LoadingSignal) Deps {
	return Deps{FS: fs, Output: output, Exec: exec, HTTP: http, Loading: loading}
}

// NewDepsWithSync creates Deps with all dependencies including issue sync.
func NewDepsWithSync(fs FS, output Output, exec Exec, http HTTPClient, loading *LoadingSignal, issueSync *IssueSyncSignal) Deps {
	return Deps{FS: fs, Output: output, Exec: exec, HTTP: http, Loading: loading, IssueSync: issueSync}
}
