package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Remote execution proxy: `mp --host <ssh-host> [--dir <path>] <cmd> ...`
// forwards the whole invocation over ssh to an `mp` binary on the remote
// host, where the repo and worktrees live. Nothing in core runs locally, so
// worktree paths, hooks, and forge auth all resolve on the remote machine.
// `mp --project <name> <cmd>` addresses a registered project the same way:
// proxied when the registry entry has a host, run from its path when local.
//
// The proxy flags are extracted from argv *before* cobra parses anything,
// and only from the stretch between `mp` and the first non-flag token (the
// verb): `mp --host wire list`, never `mp list --host wire`. That keeps
// forwarding lossless under version skew (flags only a newer remote mp
// understands never fail local parsing) and keeps verbs that own flags with
// the same names (`mp init --dir`, `mp switch --project`) untouched.

// remoteTarget is an ssh destination for proxied execution.
type remoteTarget struct {
	host    string
	dir     string // remote working directory; empty = ssh login dir
	fromEnv bool   // target came from MP_HOST, not an explicit flag
	// placement marks a call made on behalf of a placed piece (create
	// --remote, verbs routed to a placement). The box-side mp is told so via
	// MP_PLACEMENT_HOST / MP_REMOTE (D4); plain `mp --host` proxying is not
	// a placement and exports nothing. MP_HOST is never set: it would make
	// the box re-proxy.
	placement bool
}

// remoteSpec is the raw routing input parsed out of argv/env, before the
// registry is consulted.
type remoteSpec struct {
	host, dir, project string
	hostFromEnv        bool
	dirFromFlag        bool
}

// localOnlyVerbs never proxy, even with MP_HOST exported: completion and help
// are about the local binary, and `mp remote` (doctor etc.) probes hosts
// itself.
var localOnlyVerbs = map[string]bool{
	"__complete": true, "__completeNoDesc": true, "completion": true,
	"help": true, "remote": true,
}

// extractRemoteSpec strips --host/--dir/--project (and = forms) from the
// leading flags of argv — extraction stops at the first token that isn't a
// proxy flag — and returns the remaining args plus the routing spec. Falls
// back to MP_HOST/MP_DIR when the flags are absent.
func extractRemoteSpec(args []string) ([]string, remoteSpec, error) {
	var spec remoteSpec
	i := 0
scan:
	for i < len(args) {
		arg := args[i]
		var name, value string
		var hasValue bool
		switch {
		case arg == "--host" || arg == "--dir" || arg == "--project":
			name = arg
		case strings.HasPrefix(arg, "--host="), strings.HasPrefix(arg, "--dir="), strings.HasPrefix(arg, "--project="):
			name, value, _ = strings.Cut(arg, "=")
			hasValue = true
		default:
			break scan
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, remoteSpec{}, fmt.Errorf("flag needs an argument: %s", name)
			}
			i++
			value = args[i]
		}
		i++
		switch name {
		case "--host":
			spec.host = value
		case "--dir":
			spec.dir = value
			spec.dirFromFlag = true
		case "--project":
			spec.project = value
		}
	}
	rest := args[i:]

	// A --host past the verb would be silently swallowed by the help-only
	// persistent flag; refuse it loudly. (--dir/--project stay verb-owned
	// past the verb, e.g. `mp init --dir`, `mp switch --project`.)
	if spec.host == "" {
		for _, a := range rest {
			if a == "--" {
				break
			}
			if a == "--host" || strings.HasPrefix(a, "--host=") {
				return nil, remoteSpec{}, fmt.Errorf("--host must come before the command (use: mp --host <ssh-host> %s)", rest[0])
			}
		}
	}

	if spec.host == "" && os.Getenv("MP_HOST") != "" {
		spec.host = os.Getenv("MP_HOST")
		spec.hostFromEnv = true
	}
	if spec.dir == "" {
		spec.dir = os.Getenv("MP_DIR")
	}

	if len(rest) > 0 && localOnlyVerbs[rest[0]] {
		return rest, remoteSpec{}, nil
	}
	return rest, spec, nil
}

// resolveTarget turns a routing spec into an ssh target and/or a local chdir.
// Precedence: --host flag (MP_HOST counts) > registry host > local. mp never
// proxies based on cwd or a bare directory alone.
func resolveTarget(spec remoteSpec) (target *remoteTarget, chdir string, err error) {
	var proj registry.Project
	if spec.project != "" {
		reg, err := registry.Load()
		if err != nil {
			return nil, "", err
		}
		proj, err = reg.FindUnique(spec.project)
		if err != nil {
			return nil, "", err
		}
	}

	host, fromEnv := spec.host, spec.hostFromEnv
	if host == "" && proj.Host != "" {
		host, fromEnv = proj.Host, false
	}

	if host == "" {
		if spec.dirFromFlag {
			return nil, "", fmt.Errorf("--dir requires a remote target (--host, or --project with a remote project)")
		}
		return nil, proj.Path, nil // Path is "" without --project: plain local run
	}
	if err := cli.ValidSSHDest(host); err != nil {
		return nil, "", err
	}
	dir := spec.dir
	if dir == "" {
		dir = proj.Path
	}
	return &remoteTarget{host: host, dir: dir, fromEnv: fromEnv}, "", nil
}

// remoteBin is the mp binary to run on the remote host; MP_REMOTE_BIN (local
// env) overrides the default PATH lookup.
func remoteBin() string {
	if bin := os.Getenv("MP_REMOTE_BIN"); bin != "" {
		return bin
	}
	return "mp"
}

// remoteCommand builds the shell command executed on the remote host, always
// wrapped in `sh -c` so it runs under a POSIX shell regardless of the remote
// user's login shell (sshd hands the command string to fish/csh verbatim).
// Non-interactive ssh gets a bare sshd PATH that omits ~/.local/bin — mp's
// own install location — so it is prepended before the lookup. Placement
// calls also export the controller's view of the box for box-side hooks.
func remoteCommand(target *remoteTarget, args []string) string {
	var b strings.Builder
	b.WriteString(`export PATH="$HOME/.local/bin:$PATH"; `)
	if target.placement {
		b.WriteString("export " + piececmd.EnvPlacementHost + "=" + cli.ShQuote(target.host) + " " + piececmd.EnvRemote + "=1; ")
	}
	if target.dir != "" {
		b.WriteString("cd " + cli.ShQuote(target.dir) + " && ")
	}
	b.WriteString("exec " + cli.ShQuote(remoteBin()))
	for _, a := range args {
		b.WriteString(" " + cli.ShQuote(a))
	}
	return "sh -c " + cli.ShQuote(b.String())
}

// sshArgv builds the full local ssh invocation. BatchMode keeps agents from
// hanging on password prompts; ConnectTimeout bounds unreachable hosts. A
// remote pty (-t) is requested only when pty is true — the caller decides
// once, so pty allocation and stdin handling can never disagree.
func sshArgv(target *remoteTarget, args []string, pty bool) []string {
	argv := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}
	if pty {
		argv = append(argv, "-t")
	}
	argv = append(argv, "--", target.host, remoteCommand(target, args))
	return argv
}

// runRemote executes the proxied command and returns its exit code.
func runRemote(target *remoteTarget, args []string) int {
	if target.fromEnv {
		fmt.Fprintf(os.Stderr, "mp: forwarding to %s (MP_HOST is set)\n", target.host)
	}
	// A remote pty is allocated only when the local invocation is fully
	// interactive, so piped/JSON usage stays byte-clean.
	pty := cli.IsTerminal() && cli.IsStdoutTerminal()
	cmdArgv := sshArgv(target, args, pty)
	cmd := exec.Command(cmdArgv[0], cmdArgv[1:]...)
	cmd.Stdin = os.Stdin
	// Without a pty the remote mp always sees a pipe for stdin, so it takes
	// the stdin-JSON path even when the local caller supplied nothing — and a
	// zero-byte read is "invalid JSON", while a local tty as stdin would make
	// the remote block forever waiting for EOF. Substitute the empty JSON
	// object: same meaning ("no input, use defaults"), valid to parse.
	if !pty && !cli.HasStdinData() {
		cmd.Stdin = strings.NewReader("{}")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// ssh itself exits 255 on connection failure; anything else is
			// the remote mp's own exit code, passed through verbatim.
			if exitErr.ExitCode() == 255 {
				fmt.Fprintf(os.Stderr, "mp: ssh to %s failed (check `ssh %s` works and mp is installed there)\n", target.host, target.host)
			}
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "mp: %v\n", err)
		return 1
	}
	return 0
}

// runRemoteCapture runs a proxied command without a pty, capturing stdout
// (JSON for the caller) while stderr streams through. stdin is the empty
// JSON object, as in runRemote. A non-zero timeout bounds the whole call.
// Returns stdout, the exit code (255 = ssh itself failed) and the error.
func runRemoteCapture(target *remoteTarget, args []string, timeout time.Duration) (string, int, error) {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmdArgv := sshArgv(target, args, false)
	cmd := exec.CommandContext(ctx, cmdArgv[0], cmdArgv[1:]...)
	cmd.Stdin = strings.NewReader("{}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		code = 1
	}
	return out.String(), code, err
}
