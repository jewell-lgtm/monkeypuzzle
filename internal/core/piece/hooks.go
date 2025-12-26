package piece

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Hook types for piece operations
const (
	HookOnPieceCreate    = "on-piece-create.sh"
	HookBeforePieceMerge = "before-piece-merge.sh"
	HookAfterPieceMerge  = "after-piece-merge.sh"
	HookBeforePieceUpdate = "before-piece-update.sh"
	HookAfterPieceUpdate  = "after-piece-update.sh"
)

// HooksDir is the directory name for hooks within the project
const HooksDir = ".monkeypuzzle/hooks"

// HookContext contains environment variables to pass to hooks
type HookContext struct {
	PieceName    string // MP_PIECE_NAME
	WorktreePath string // MP_WORKTREE_PATH
	RepoRoot     string // MP_REPO_ROOT
	MainBranch   string // MP_MAIN_BRANCH (for merge/update hooks)
	SessionName  string // MP_SESSION_NAME (for create hooks)
}

// HookRunner executes hook scripts from the .monkeypuzzle/hooks directory
type HookRunner struct {
	exec   core.Exec
	fs     core.FS
	output core.Output
}

// NewHookRunner creates a new HookRunner with the given dependencies
func NewHookRunner(deps core.Deps) *HookRunner {
	return &HookRunner{
		exec:   deps.Exec,
		fs:     deps.FS,
		output: deps.Output,
	}
}

// RunHook executes a hook script if it exists and is executable.
// Returns nil if the hook doesn't exist or the hooks directory doesn't exist.
// Returns an error if the hook exists but fails to execute (non-zero exit code).
func (h *HookRunner) RunHook(repoRoot, hookName string, ctx HookContext) error {
	hookPath := filepath.Join(repoRoot, HooksDir, hookName)

	// Check if the hook file exists
	info, err := h.fs.Stat(hookPath)
	if err != nil {
		// Hook doesn't exist, that's fine
		if os.IsNotExist(err) {
			return nil
		}
		// Other stat error
		return fmt.Errorf("failed to stat hook %s: %w", hookName, err)
	}

	// Check if the file is executable
	if info.Mode()&0111 == 0 {
		// Not executable, skip
		h.output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Hook %s is not executable, skipping", hookName),
		})
		return nil
	}

	// Build environment variables
	env := h.buildEnv(ctx)

	// Execute the hook
	h.output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: fmt.Sprintf("Running hook: %s", hookName),
	})

	output, err := h.execWithEnv(repoRoot, hookPath, env)
	if err != nil {
		// Output hook's stderr/stdout
		if len(output) > 0 {
			h.output.Write(core.Message{
				Type:    core.MsgError,
				Content: string(output),
			})
		}
		return fmt.Errorf("hook %s failed: %w", hookName, err)
	}

	// Output hook's stdout if any
	if len(output) > 0 {
		h.output.Write(core.Message{
			Type:    core.MsgInfo,
			Content: string(output),
		})
	}

	return nil
}

// buildEnv creates environment variable strings for the hook.
// It filters out any existing MP_* variables to ensure our values take precedence.
func (h *HookRunner) buildEnv(ctx HookContext) []string {
	// Filter out existing MP_* variables to avoid duplicates
	env := filterEnv(os.Environ(), "MP_")

	if ctx.PieceName != "" {
		env = append(env, fmt.Sprintf("MP_PIECE_NAME=%s", ctx.PieceName))
	}
	if ctx.WorktreePath != "" {
		env = append(env, fmt.Sprintf("MP_WORKTREE_PATH=%s", ctx.WorktreePath))
	}
	if ctx.RepoRoot != "" {
		env = append(env, fmt.Sprintf("MP_REPO_ROOT=%s", ctx.RepoRoot))
	}
	if ctx.MainBranch != "" {
		env = append(env, fmt.Sprintf("MP_MAIN_BRANCH=%s", ctx.MainBranch))
	}
	if ctx.SessionName != "" {
		env = append(env, fmt.Sprintf("MP_SESSION_NAME=%s", ctx.SessionName))
	}

	return env
}

// filterEnv returns a copy of env with all variables starting with prefix removed.
func filterEnv(env []string, prefix string) []string {
	result := make([]string, 0, len(env))
	for _, e := range env {
		if !hasEnvPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// hasEnvPrefix checks if an environment variable string (KEY=value) starts with the given prefix.
func hasEnvPrefix(envVar, prefix string) bool {
	// Find the = separator
	for i := 0; i < len(envVar); i++ {
		if envVar[i] == '=' {
			// Check if the key part starts with prefix
			return i >= len(prefix) && envVar[:len(prefix)] == prefix
		}
	}
	return false
}

// execWithEnv executes a script with the given environment variables
func (h *HookRunner) execWithEnv(dir, script string, env []string) ([]byte, error) {
	// Use bash to execute the script
	return h.exec.RunWithEnv(dir, env, "bash", script)
}

