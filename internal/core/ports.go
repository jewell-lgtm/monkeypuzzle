package core

import (
	"io/fs"
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
	Run(name string, args ...string) ([]byte, error)
	RunWithDir(dir, name string, args ...string) ([]byte, error)
}

// Deps holds all injectable dependencies for handlers
type Deps struct {
	FS     FS
	Output Output
	Exec   Exec
}
