package mprunner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// stackGraphTimeout caps a single `mp stack graph` invocation.
const stackGraphTimeout = 30 * time.Second

// ExecRunner runs the real `mp` binary out of process. The binary path is
// resolved once at construction.
type ExecRunner struct {
	mpPath string
}

// NewExecRunner resolves the mp binary once and returns a runner bound to it.
// An explicit override (from MP_BIN / config) wins; otherwise mp is located as
// a sibling of this executable, then on PATH — mirroring apps/mp-mcp's
// findMpBinary so install layout stays consistent.
func NewExecRunner(override string) *ExecRunner {
	return &ExecRunner{mpPath: findMp(override)}
}

func findMp(override string) string {
	if override != "" {
		return override
	}
	if exe, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "mp")
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
	}
	if p, err := exec.LookPath("mp"); err == nil {
		return p
	}
	return "mp"
}

func providerOrDefault(p string) string {
	if p == "" {
		return "github"
	}
	return p
}

// minimalEnvVars are the only host environment variables forwarded to the child
// `mp`/`gh` process: enough to find the binary (PATH), a home for git/gh config
// (HOME), proxy settings, and custom CA bundles. Everything else — notably the
// server's secrets (TOKEN_ENCRYPTION_KEY, SESSION_SECRET, WORKOS_API_KEY,
// DATABASE_URL) — is deliberately withheld.
var minimalEnvVars = []string{
	"PATH", "HOME",
	"HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "ALL_PROXY",
	"https_proxy", "http_proxy", "no_proxy", "all_proxy",
	"SSL_CERT_FILE", "SSL_CERT_DIR",
}

// minimalEnv returns a KEY=VALUE slice carrying only the allow-listed host
// variables that are actually set, so the child inherits none of the server's
// secrets.
func minimalEnv() []string {
	env := make([]string, 0, len(minimalEnvVars))
	for _, k := range minimalEnvVars {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// buildCmd assembles the fully configured `mp stack graph` command: its args,
// and a minimal environment carrying only the forge token (GH_TOKEN, or
// GITLAB_TOKEN for gitlab) plus the allow-listed host vars. The token rides in
// the env only — never in cmd.Args.
func (r *ExecRunner) buildCmd(ctx context.Context, in StackGraphInput) *exec.Cmd {
	provider := providerOrDefault(in.Provider)
	args := []string{"stack", "graph", "--repo", in.RepoSlug, "--provider", provider}
	if in.DefaultBranch != "" {
		args = append(args, "--default-branch", in.DefaultBranch)
	}

	cmd := exec.CommandContext(ctx, r.mpPath, args...)
	tokenEnv := "GH_TOKEN="
	if provider == "gitlab" {
		tokenEnv = "GITLAB_TOKEN="
	}
	// Token rides in the child env only — never logged, never in cmd.Args.
	cmd.Env = append(minimalEnv(), tokenEnv+in.Token)
	return cmd
}

// StackGraph runs `mp stack graph` and parses its {stacks: [...]} output. The
// token is supplied only through the child's environment (GH_TOKEN, or
// GITLAB_TOKEN for gitlab) and is never included in any returned error.
func (r *ExecRunner) StackGraph(ctx context.Context, in StackGraphInput) ([]stackgraph.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, stackGraphTimeout)
	defer cancel()

	cmd := r.buildCmd(ctx, in)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mp stack graph: %w", scrub(err))
	}

	var parsed struct {
		Stacks []stackgraph.Stack `json:"stacks"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("mp stack graph: parse output: %w", err)
	}
	return parsed.Stacks, nil
}

// scrub returns an exec error enriched with the child's stderr (useful for
// diagnosis) but never its environment. The token lives only in cmd.Env, which
// exec errors never expose, and it is never passed as an argument — so neither
// the wrapped error nor the stderr can carry it.
func scrub(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, string(ee.Stderr))
	}
	return err
}

var _ MpRunner = (*ExecRunner)(nil)
