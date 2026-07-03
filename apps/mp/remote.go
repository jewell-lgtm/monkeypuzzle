package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Remote execution proxy: `mp --host <ssh-host> [--dir <path>] <cmd> ...`
// forwards the whole invocation over ssh to an `mp` binary on the remote
// host, where the repo and worktrees live. Nothing in core runs locally, so
// worktree paths, hooks, and forge auth all resolve on the remote machine.
//
// The --host/--dir flags (env: MP_HOST/MP_DIR) are extracted from argv
// *before* cobra parses anything. That keeps forwarding lossless under
// version skew: flags that only a newer remote mp understands never fail
// local parsing, because the local binary ships the raw argv through
// untouched.

// remoteTarget is an ssh destination for proxied execution.
type remoteTarget struct {
	host    string
	dir     string // remote working directory; empty = ssh login dir
	fromEnv bool   // target came from MP_HOST, not an explicit flag
}

// extractRemoteTarget strips --host/--dir (and --host=/--dir= forms) from
// argv and returns the remaining args plus the target, if any. Falls back to
// MP_HOST/MP_DIR when the flags are absent. A --dir without a host is an
// error: mp never proxies based on cwd or a bare directory alone.
func extractRemoteTarget(args []string) ([]string, *remoteTarget, error) {
	var host, dir string
	dirFromFlag := false
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		var name, value string
		var hasValue bool
		switch {
		case arg == "--host" || arg == "--dir":
			name = arg
		case strings.HasPrefix(arg, "--host="), strings.HasPrefix(arg, "--dir="):
			name, value, _ = strings.Cut(arg, "=")
			hasValue = true
		default:
			rest = append(rest, arg)
			continue
		}
		if !hasValue {
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag needs an argument: %s", name)
			}
			i++
			value = args[i]
		}
		if name == "--host" {
			host = value
		} else {
			dir = value
			dirFromFlag = true
		}
	}

	fromEnv := false
	if host == "" && os.Getenv("MP_HOST") != "" {
		host = os.Getenv("MP_HOST")
		fromEnv = true
	}
	if dir == "" {
		dir = os.Getenv("MP_DIR")
	}

	if host == "" {
		if dirFromFlag {
			return nil, nil, fmt.Errorf("--dir requires --host (mp never proxies from a directory alone)")
		}
		// A stray MP_DIR without a host is ignored rather than fatal.
		return rest, nil, nil
	}
	return rest, &remoteTarget{host: host, dir: dir, fromEnv: fromEnv}, nil
}

// shQuote quotes s for a POSIX shell: wrap in single quotes, with embedded
// single quotes spliced out as quoted-backslash-quote.
func shQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// remoteCommand builds the shell command executed on the remote host.
// Non-interactive ssh gets a bare sshd PATH that omits ~/.local/bin — mp's own
// install location — so it is prepended before the lookup. MP_REMOTE_BIN
// (local env) overrides the remote binary name/path entirely.
func remoteCommand(target *remoteTarget, args []string) string {
	bin := os.Getenv("MP_REMOTE_BIN")
	if bin == "" {
		bin = "mp"
	}
	var b strings.Builder
	b.WriteString(`export PATH="$HOME/.local/bin:$PATH"; `)
	if target.dir != "" {
		b.WriteString("cd " + shQuote(target.dir) + " && ")
	}
	b.WriteString("exec " + shQuote(bin))
	for _, a := range args {
		b.WriteString(" " + shQuote(a))
	}
	return b.String()
}

// sshArgv builds the full local ssh invocation. BatchMode keeps agents from
// hanging on password prompts; ConnectTimeout bounds unreachable hosts. A
// remote pty (-t) is allocated only when the local invocation is fully
// interactive, so piped/JSON usage stays byte-clean.
func sshArgv(target *remoteTarget, args []string) []string {
	argv := []string{"ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}
	if cli.IsTerminal() && cli.IsStdoutTerminal() {
		argv = append(argv, "-t")
	}
	argv = append(argv, target.host, "--", remoteCommand(target, args))
	return argv
}

// runRemote executes the proxied command and returns its exit code.
func runRemote(target *remoteTarget, args []string) int {
	if target.fromEnv {
		fmt.Fprintf(os.Stderr, "mp: forwarding to %s (MP_HOST is set)\n", target.host)
	}
	argv := sshArgv(target, args)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	// Over ssh the remote mp always sees a pipe for stdin, so it takes the
	// stdin-JSON path even when the local caller supplied nothing and a
	// zero-byte read is "invalid JSON". Substitute the empty JSON object:
	// same meaning ("no input, use defaults"), valid to parse.
	if !cli.IsTerminal() && !cli.HasStdinData() {
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
