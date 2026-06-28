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

// StackGraph runs `mp stack graph` and parses its {stacks: [...]} output. The
// token is supplied only through the child's environment (GH_TOKEN, or
// GITLAB_TOKEN for gitlab) and is never included in any returned error.
func (r *ExecRunner) StackGraph(ctx context.Context, in StackGraphInput) ([]stackgraph.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, stackGraphTimeout)
	defer cancel()

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
	cmd.Env = append(os.Environ(), tokenEnv+in.Token)

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
