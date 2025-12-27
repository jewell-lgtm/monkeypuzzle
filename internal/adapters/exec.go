package adapters

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Ensure implementations satisfy interface
var (
	_ core.Exec = (*OSExec)(nil)
	_ core.Exec = (*MockExec)(nil)
)

// OSExec implements core.Exec using os/exec for real command execution
type OSExec struct{}

// NewOSExec creates an OSExec instance
func NewOSExec() *OSExec {
	return &OSExec{}
}

// Run executes a command and returns its output
func (e *OSExec) Run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// RunWithDir executes a command in the specified directory and returns its output
func (e *OSExec) RunWithDir(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// RunWithEnv executes a command in the specified directory with environment variables
func (e *OSExec) RunWithEnv(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// CallRecord represents a recorded command call
type CallRecord struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

// MockExec implements core.Exec for testing, recording calls and returning configurable outputs
type MockExec struct {
	mu        sync.RWMutex
	calls     []CallRecord
	responses map[string]map[string]responseEntry
}

type responseEntry struct {
	output []byte
	err    error
}

// NewMockExec creates a MockExec instance for testing
func NewMockExec() *MockExec {
	return &MockExec{
		calls:     make([]CallRecord, 0),
		responses: make(map[string]map[string]responseEntry),
	}
}

// AddResponse configures a mock response for a specific command and arguments
func (m *MockExec) AddResponse(name string, args []string, output []byte, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := strings.Join(args, " ")
	if m.responses[name] == nil {
		m.responses[name] = make(map[string]responseEntry)
	}
	m.responses[name][key] = responseEntry{output: output, err: err}
}

// Run executes a command and returns configured output or an error
func (m *MockExec) Run(name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, CallRecord{
		Name: name,
		Args: args,
		Dir:  "",
	})

	key := strings.Join(args, " ")
	if resp, ok := m.responses[name][key]; ok {
		return resp.output, resp.err
	}

	// Default: return error indicating no response configured
	return nil, fmt.Errorf("no response configured for %s %s", name, key)
}

// RunWithDir executes a command in the specified directory and returns configured output or an error
func (m *MockExec) RunWithDir(dir, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dir, _ = filepath.Abs(dir)
	m.calls = append(m.calls, CallRecord{
		Name: name,
		Args: args,
		Dir:  dir,
	})

	key := strings.Join(args, " ")
	if resp, ok := m.responses[name][key]; ok {
		return resp.output, resp.err
	}

	// Default: return error indicating no response configured
	return nil, fmt.Errorf("no response configured for %s %s (dir: %s)", name, key, dir)
}

// RunWithEnv executes a command with environment variables and returns configured output or an error
func (m *MockExec) RunWithEnv(dir string, env []string, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dir != "" {
		dir, _ = filepath.Abs(dir)
	}
	m.calls = append(m.calls, CallRecord{
		Name: name,
		Args: args,
		Dir:  dir,
		Env:  env,
	})

	key := strings.Join(args, " ")
	if resp, ok := m.responses[name][key]; ok {
		return resp.output, resp.err
	}

	// Default: return error indicating no response configured
	return nil, fmt.Errorf("no response configured for %s %s (dir: %s)", name, key, dir)
}

// WasCalled checks if a command was called with the specified arguments
func (m *MockExec) WasCalled(name string, args ...string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	argKey := strings.Join(args, " ")
	for _, call := range m.calls {
		if call.Name == name && strings.Join(call.Args, " ") == argKey {
			return true
		}
	}
	return false
}

// GetCalls returns all recorded calls
func (m *MockExec) GetCalls() []CallRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]CallRecord, len(m.calls))
	copy(calls, m.calls)
	return calls
}

// ClearCalls clears all recorded calls (useful for test cleanup)
func (m *MockExec) ClearCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = make([]CallRecord, 0)
}

// MockError creates a simple error for use in mock responses
func MockError(msg string) error {
	return errors.New(msg)
}
