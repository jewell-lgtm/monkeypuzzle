package adapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

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
func (e *OSExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// RunWithDir executes a command in the specified directory and returns its output
func (e *OSExec) RunWithDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// RunWithEnv executes a command in the specified directory with environment variables
func (e *OSExec) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, err
	}
	return output, nil
}

// StartDetached launches a command in its own process group with combined
// output redirected to logPath, returning as soon as the process has started.
// It does not wait for completion, so the hook outlives the mp invocation that
// triggered it. A reaping goroutine collects the child if this process happens
// to stay alive; otherwise init reaps it after we exit.
func (e *OSExec) StartDetached(dir string, env []string, logPath, name string, args ...string) error {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return fmt.Errorf("failed to create log directory for %s: %w", logPath, err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open hook log %s: %w", logPath, err)
	}
	// The child inherits a dup of the file descriptor, so closing our handle
	// after Start does not affect the running process.
	defer func() { _ = logFile.Close() }()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Setpgid detaches the child into its own process group so it is not killed
	// by signals (e.g. Ctrl-C) delivered to mp's process group.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
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
	mu            sync.RWMutex
	calls         []CallRecord
	detachedCalls []DetachedCall
	responses     map[string]map[string]responseEntry
	detachedErr   error
}

type responseEntry struct {
	output []byte
	err    error
}

// DetachedCall records a StartDetached invocation for assertions in tests.
type DetachedCall struct {
	Name    string
	Args    []string
	Dir     string
	Env     []string
	LogPath string
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
func (m *MockExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
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
func (m *MockExec) RunWithDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// filepath.Abs only fails if cwd cannot be determined; in tests, paths are controlled
	dir, _ = filepath.Abs(dir) //nolint:errcheck // test mock, paths are controlled
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
func (m *MockExec) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dir != "" {
		// filepath.Abs only fails if cwd cannot be determined; in tests, paths are controlled
		dir, _ = filepath.Abs(dir) //nolint:errcheck // test mock, paths are controlled
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

// StartDetached records a detached launch and returns the configured error (nil
// by default, simulating a process that started successfully). The mock never
// actually runs anything in the background.
func (m *MockExec) StartDetached(dir string, env []string, logPath, name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dir != "" {
		// filepath.Abs only fails if cwd cannot be determined; in tests, paths are controlled
		dir, _ = filepath.Abs(dir) //nolint:errcheck // test mock, paths are controlled
	}
	m.detachedCalls = append(m.detachedCalls, DetachedCall{
		Name:    name,
		Args:    args,
		Dir:     dir,
		Env:     env,
		LogPath: logPath,
	})
	return m.detachedErr
}

// SetDetachedErr makes subsequent StartDetached calls return err, simulating a
// process that fails to start.
func (m *MockExec) SetDetachedErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detachedErr = err
}

// GetDetachedCalls returns all recorded StartDetached calls.
func (m *MockExec) GetDetachedCalls() []DetachedCall {
	m.mu.RLock()
	defer m.mu.RUnlock()

	calls := make([]DetachedCall, len(m.detachedCalls))
	copy(calls, m.detachedCalls)
	return calls
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
