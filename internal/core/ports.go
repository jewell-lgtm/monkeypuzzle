package core

import (
	"context"
	"io/fs"
	"net/http"
	"os"
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

// Deps holds all injectable dependencies for handlers
type Deps struct {
	FS     FS
	Output Output
	Exec   Exec
	HTTP   HTTPClient
}

// NewDeps creates Deps with all required dependencies.
// Using this constructor ensures the compiler catches missing dependencies.
func NewDeps(fs FS, output Output, exec Exec, http HTTPClient) Deps {
	return Deps{FS: fs, Output: output, Exec: exec, HTTP: http}
}
