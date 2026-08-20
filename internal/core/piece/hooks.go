package piece

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// Hook types for piece operations
const (
	HookOnPieceCreate     = "on-piece-create.sh"
	HookBeforePieceMerge  = "before-piece-merge.sh"
	HookAfterPieceMerge   = "after-piece-merge.sh"
	HookBeforePieceUpdate = "before-piece-update.sh"
	HookAfterPieceUpdate  = "after-piece-update.sh"
	HookBeforePRCreate    = "before-pr-create.sh"
	HookAfterPRCreate     = "after-pr-create.sh"
	HookBeforePRReady     = "before-pr-ready.sh"
	HookAfterPRReady      = "after-pr-ready.sh"
	// HookIsPieceDone, if present + executable, is consulted by `mp piece cleanup`
	// and merge-detection paths. Exit 0 means "this piece is merged". Lets users
	// override mp's built-in git-based detection (e.g. to recognise squash-merges
	// via a forge API) without baking forge-specific logic into core.
	HookIsPieceDone = "is-piece-done.sh"
	// HookAgentBlocked / HookAgentDone fire (detached) when a piece's aggregate
	// agent status transitions to blocked / done — the notification points for
	// "an agent needs you" / "an agent finished".
	HookAgentBlocked = "agent-blocked.sh"
	HookAgentDone    = "agent-done.sh"
	// HookOnBoxConnect runs on the controller, blocking, the first time a
	// project places a piece on a box (`mp create --remote`). When present it
	// replaces mp's built-in connect (clone + init + ship hooks) entirely; it
	// must leave an mp project at MP_REMOTE_PATH on MP_BOX. Non-zero aborts
	// the placement. It is the only hook that runs with MP_BOX set.
	HookOnBoxConnect = "on-box-connect.sh"
)

// Env vars a controller exports into a proxied placement call so the box-side
// mp knows it is acting on behalf of a controller. They are read at startup
// (NewHookRunner) and surface in every box-side hook's env. MP_HOST is never
// used for this: it is the proxy reroute var.
const (
	EnvPlacementHost = "MP_PLACEMENT_HOST"
	EnvRemote        = "MP_REMOTE"
)

// HookContext contains environment variables to pass to hooks
type HookContext struct {
	PieceName    string // MP_PIECE_NAME
	WorktreePath string // MP_WORKTREE_PATH
	RepoRoot     string // MP_REPO_ROOT
	MainBranch   string // MP_MAIN_BRANCH (for merge/update hooks)
	SessionName  string // MP_SESSION_NAME (for create hooks)
	PRNumber     int    // MP_PR_NUMBER (for PR hooks)
	PRURL        string // MP_PR_URL (for PR hooks)
	PRBaseBranch string // MP_PR_BASE_BRANCH (for PR hooks)
	AgentID      string // MP_AGENT_ID (for agent hooks)
	AgentKind    string // MP_AGENT_KIND (for agent hooks)
	AgentStatus  string // MP_AGENT_STATUS (for agent hooks; the piece aggregate)
	AgentPane    string // MP_AGENT_PANE (for agent hooks)

	// Placement (box-side hooks): the box this mp is running on, as named
	// by the controller that placed the piece, and whether the call was
	// proxied at all. Zero values fall back to what NewHookRunner read from
	// the process env, so core call sites need not know about placement.
	PlacementHost string // MP_PLACEMENT_HOST
	Remote        bool   // MP_REMOTE=1

	// Box connect (controller-side on-box-connect.sh only).
	Box        string // MP_BOX: ssh destination being connected
	RemotePath string // MP_REMOTE_PATH: intended clone path on the box (pre-readlink)
	RepoURL    string // MP_REPO_URL: origin to clone
	Project    string // MP_PROJECT
	HooksDir   string // MP_HOOKS_DIR: controller-side hooks dir
}

// withPlacementDefaults fills PlacementHost/Remote from the runner when the
// call site left them empty.
func (h *HookRunner) withPlacementDefaults(ctx HookContext) HookContext {
	if ctx.PlacementHost == "" {
		ctx.PlacementHost = h.placementHost
	}
	if !ctx.Remote {
		ctx.Remote = h.remote
	}
	return ctx
}

// HookRunner executes hook scripts from the .monkeypuzzle/hooks directory
type HookRunner struct {
	exec   core.Exec
	fs     core.FS
	output core.Output

	// Box-side placement identity, read once from the env the controller's
	// proxy exported (see EnvPlacementHost). buildEnv strips MP_* from the
	// inherited env, so these must be carried explicitly.
	placementHost string
	remote        bool
}

// NewHookRunner creates a new HookRunner with the given dependencies
func NewHookRunner(deps core.Deps) *HookRunner {
	return &HookRunner{
		exec:          deps.Exec,
		fs:            deps.FS,
		output:        deps.Output,
		placementHost: os.Getenv(EnvPlacementHost),
		remote:        os.Getenv(EnvRemote) == "1",
	}
}

// resolveExecutableHook returns the path to hookName if it exists and is
// executable. ok is false (with a nil error) when the hook is absent or not
// executable — both are no-ops for the caller. A non-existent-file stat is the
// common "no hook configured" case; any other stat error is returned.
func (h *HookRunner) resolveExecutableHook(repoRoot, hookName string) (hookPath string, ok bool, err error) {
	hookPath = filepath.Join(projectdir.HooksDir(repoRoot), hookName)

	info, err := h.fs.Stat(hookPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to stat hook %s: %w", hookName, err)
	}

	if info.Mode()&0111 == 0 {
		h.output.Write(core.Message{
			Type:    core.MsgWarning,
			Content: fmt.Sprintf("Hook %s is not executable, skipping", hookName),
		})
		return "", false, nil
	}

	return hookPath, true, nil
}

// RunHook executes a hook script if it exists and is executable, blocking until
// it completes.
// Returns nil if the hook doesn't exist or the hooks directory doesn't exist.
// Returns an error if the hook exists but fails to execute (non-zero exit code).
func (h *HookRunner) RunHook(ctx context.Context, repoRoot, hookName string, hookCtx HookContext) error {
	_, _, err := h.RunHookOutput(ctx, repoRoot, hookName, hookCtx)
	return err
}

// RunHookOutput is RunHook that also reports whether the hook existed (ran)
// and returns its combined output, so callers can fold a failing hook's
// stderr into their own error.
func (h *HookRunner) RunHookOutput(ctx context.Context, repoRoot, hookName string, hookCtx HookContext) (output []byte, ran bool, err error) {
	hookPath, ok, err := h.resolveExecutableHook(repoRoot, hookName)
	if err != nil || !ok {
		return nil, false, err
	}

	// Build environment variables
	env := h.buildEnv(hookCtx)

	// Execute the hook
	h.output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: fmt.Sprintf("Running hook: %s", hookName),
	})

	output, err = h.execWithEnv(ctx, repoRoot, hookPath, env)
	if err != nil {
		// Output hook's stderr/stdout
		if len(output) > 0 {
			h.output.Write(core.Message{
				Type:    core.MsgError,
				Content: string(output),
			})
		}
		return output, true, fmt.Errorf("hook %s failed: %w", hookName, err)
	}

	// Output hook's stdout if any
	if len(output) > 0 {
		h.output.Write(core.Message{
			Type:    core.MsgInfo,
			Content: string(output),
		})
	}

	return output, true, nil
}

// RunHookDetached starts a hook script in the background (fire-and-forget) and
// returns immediately, without waiting for it to finish. The hook's combined
// output is redirected to a log file under the repo's monkeypuzzle logs
// directory, and the user is told where to find it.
//
// Like RunHook, a missing or non-executable hook is a no-op. Unlike RunHook,
// the hook's exit status is never observed: only a failure to *start* the
// process is returned as an error. This suits setup hooks (dependency installs,
// submodule init) whose work shouldn't block the command that triggered them.
func (h *HookRunner) RunHookDetached(repoRoot, hookName string, hookCtx HookContext) error {
	hookPath, ok, err := h.resolveExecutableHook(repoRoot, hookName)
	if err != nil || !ok {
		return err
	}

	env := h.buildEnv(hookCtx)
	logPath := hookLogPath(repoRoot, hookName, hookCtx.PieceName)

	if err := h.exec.StartDetached(repoRoot, env, logPath, "bash", hookPath); err != nil {
		return fmt.Errorf("hook %s failed to start: %w", hookName, err)
	}

	h.output.Write(core.Message{
		Type:    core.MsgInfo,
		Content: fmt.Sprintf("Running hook %s in background; output: %s", hookName, logPath),
	})
	return nil
}

// hookLogPath returns the log file a detached hook writes to. Including the
// piece name keeps concurrent piece creations from clobbering each other's logs.
func hookLogPath(repoRoot, hookName, pieceName string) string {
	base := strings.TrimSuffix(hookName, ".sh")
	if pieceName != "" {
		base = base + "-" + pieceName
	}
	return filepath.Join(projectdir.LogsDir(repoRoot), base+".log")
}

// buildEnv creates environment variable strings for the hook.
// It filters out any existing MP_* variables to ensure our values take precedence.
func (h *HookRunner) buildEnv(ctx HookContext) []string {
	ctx = h.withPlacementDefaults(ctx)
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
	if ctx.PRNumber != 0 {
		env = append(env, fmt.Sprintf("MP_PR_NUMBER=%d", ctx.PRNumber))
	}
	if ctx.PRURL != "" {
		env = append(env, fmt.Sprintf("MP_PR_URL=%s", ctx.PRURL))
	}
	if ctx.PRBaseBranch != "" {
		env = append(env, fmt.Sprintf("MP_PR_BASE_BRANCH=%s", ctx.PRBaseBranch))
	}
	if ctx.AgentID != "" {
		env = append(env, fmt.Sprintf("MP_AGENT_ID=%s", ctx.AgentID))
	}
	if ctx.AgentKind != "" {
		env = append(env, fmt.Sprintf("MP_AGENT_KIND=%s", ctx.AgentKind))
	}
	if ctx.AgentStatus != "" {
		env = append(env, fmt.Sprintf("MP_AGENT_STATUS=%s", ctx.AgentStatus))
	}
	if ctx.AgentPane != "" {
		env = append(env, fmt.Sprintf("MP_AGENT_PANE=%s", ctx.AgentPane))
	}
	if ctx.PlacementHost != "" {
		env = append(env, fmt.Sprintf("%s=%s", EnvPlacementHost, ctx.PlacementHost))
	}
	if ctx.Remote {
		env = append(env, EnvRemote+"=1")
	}
	if ctx.Box != "" {
		env = append(env, fmt.Sprintf("MP_BOX=%s", ctx.Box))
	}
	if ctx.RemotePath != "" {
		env = append(env, fmt.Sprintf("MP_REMOTE_PATH=%s", ctx.RemotePath))
	}
	if ctx.RepoURL != "" {
		env = append(env, fmt.Sprintf("MP_REPO_URL=%s", ctx.RepoURL))
	}
	if ctx.Project != "" {
		env = append(env, fmt.Sprintf("MP_PROJECT=%s", ctx.Project))
	}
	if ctx.HooksDir != "" {
		env = append(env, fmt.Sprintf("MP_HOOKS_DIR=%s", ctx.HooksDir))
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
func (h *HookRunner) execWithEnv(ctx context.Context, dir, script string, env []string) ([]byte, error) {
	// Use bash to execute the script
	return h.exec.RunWithEnv(ctx, dir, env, "bash", script)
}
